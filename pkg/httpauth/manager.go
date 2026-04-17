package httpauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// AuthManager is the central entry point for authentication. Create one with
// [New] at startup, then wire it into the HTTP router using [AuthManager.Middleware]
// and [AuthManager.Mount].
type AuthManager struct {
	cfg             AuthConfig
	canonicalAddr   string
	canonicalScheme string
	canonicalHost   string
	trustedProxies  []string
	trustedNets     []*net.IPNet
	jwtSecret       string
	logger          *slog.Logger
	onAuth          func(email, name, avatarURL string)
	oidcH           *oidcHandler // nil when OIDC is disabled
}

// Option is a functional option for [New].
type Option func(*AuthManager) error

// WithTrustedProxy sets the list of trusted proxy CIDR ranges. Headers from
// Tailscale, proxy auth, and X-Forwarded-Proto are only honoured when the
// connecting IP falls within these ranges. Required when Tailscale or proxy
// auth is enabled in production; may be omitted in tests.
func WithTrustedProxy(cidrs []string) Option {
	return func(m *AuthManager) error {
		m.trustedProxies = cidrs
		return nil
	}
}

// WithCanonicalAddress sets the public base URL for the server (e.g.
// "https://go.example.com"). Required when OIDC is enabled; also used by
// [AuthManager.DomainRedirect] and [AuthManager.SecurityHeaders].
func WithCanonicalAddress(addr string) Option {
	return func(m *AuthManager) error {
		m.canonicalAddr = addr
		return nil
	}
}

// WithJWTSecret sets the HMAC secret used to sign and verify session JWT
// cookies. Required when OIDC is enabled. Use a long random string (>= 32
// bytes recommended).
func WithJWTSecret(secret string) Option {
	return func(m *AuthManager) error {
		m.jwtSecret = secret
		return nil
	}
}

// WithOnAuthenticated registers a callback that is invoked asynchronously
// (in a background goroutine) each time a user is authenticated by any
// provider that supplies user details (OIDC, Tailscale, proxy auth). Use this
// to persist user records to a database without adding latency to the request
// path. Errors should be logged inside the callback; they are not propagated.
// If not set, authenticated users are not persisted anywhere by this package.
func WithOnAuthenticated(fn func(email, name, avatarURL string)) Option {
	return func(m *AuthManager) error {
		m.onAuth = fn
		return nil
	}
}

// WithLogger sets the structured logger used by all middleware. Defaults to
// [slog.Default] when not provided.
func WithLogger(logger *slog.Logger) Option {
	return func(m *AuthManager) error {
		m.logger = logger
		return nil
	}
}

// New creates an [AuthManager] from cfg and the given options.
//
// It applies provider defaults, resolves any env-var-based secrets, validates
// the configuration, and (when OIDC is enabled) contacts the OIDC provider to
// fetch its discovery document. Returns an error if the configuration is
// invalid or the OIDC provider is unreachable.
func New(ctx context.Context, cfg AuthConfig, opts ...Option) (*AuthManager, error) {
	m := &AuthManager{
		cfg:    cfg,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		if err := opt(m); err != nil {
			return nil, err
		}
	}

	applyAuthDefaults(&m.cfg)

	if err := resolveOIDCClientSecret(&m.cfg.OIDC); err != nil {
		return nil, err
	}

	if err := m.validate(); err != nil {
		return nil, err
	}

	if m.canonicalAddr != "" {
		u, err := url.Parse(m.canonicalAddr)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("canonical_address must be a valid URL with scheme and host (got %q)", m.canonicalAddr)
		}
		m.canonicalScheme = u.Scheme
		m.canonicalHost = u.Host
	}

	if len(m.trustedProxies) > 0 {
		nets, err := parseCIDRs(m.trustedProxies)
		if err != nil {
			return nil, fmt.Errorf("trusted_proxy: %w", err)
		}
		m.trustedNets = nets
	}

	if m.cfg.OIDC.Enabled {
		h, err := newOIDCHandler(ctx, m)
		if err != nil {
			return nil, err
		}
		m.oidcH = h
	}

	return m, nil
}

// validate checks that the AuthConfig is consistent given the runtime options.
func (m *AuthManager) validate() error {
	if !m.cfg.Anonymous.Enabled && !m.cfg.Tailscale.Enabled &&
		!m.cfg.ProxyAuth.Enabled && !m.cfg.OIDC.Enabled {
		return errors.New("at least one authentication provider must be enabled (anonymous, tailscale, proxy_auth, or oidc)")
	}
	if m.cfg.OIDC.Enabled && m.canonicalAddr == "" {
		return errors.New("canonical address is required when OIDC is enabled (use WithCanonicalAddress)")
	}
	if m.cfg.OIDC.Enabled && m.jwtSecret == "" {
		return errors.New("JWT secret is required when OIDC is enabled (use WithJWTSecret)")
	}
	return nil
}

// notifyAuth invokes the onAuth callback asynchronously if one is registered.
func (m *AuthManager) notifyAuth(email, name, avatarURL string) {
	if m.onAuth == nil {
		return
	}
	go m.onAuth(email, name, avatarURL)
}

// Middleware returns a single middleware that must be applied globally on the
// router. It composes, in order:
//
//  1. Preserve original TCP remote address (for CIDR checks)
//  2. Real-IP extraction (X-Forwarded-For / X-Real-IP)
//  3. Panic recovery
//  4. Request logging
//  5. Security headers (CSP, HSTS when HTTPS, X-Frame-Options, …)
//  6. Auth provider chain: Tailscale → proxy auth → OIDC → anonymous
//  7. Log enrichment (adds auth_source and domain to log context)
func (m *AuthManager) Middleware() func(http.Handler) http.Handler {
	httpsOnly := m.canonicalScheme == "https"
	return func(next http.Handler) http.Handler {
		h := next
		h = logEnricher(m.logger)(h)
		h = m.anonymousMiddleware()(h)
		h = m.oidcMiddleware()(h)
		h = m.proxyAuthMiddleware()(h)
		h = m.tailscaleMiddleware()(h)
		h = securityHeaders(httpsOnly)(h)
		h = requestLogger(m.logger)(h)
		h = recoverer(m.logger)(h)
		h = realIPMiddleware(h)
		h = preserveRemoteAddr(h)
		return h
	}
}

// Mount registers the authentication HTTP routes on r. The routes registered
// depend on the enabled providers:
//
//   - OIDC enabled: GET /auth/login, GET /auth/callback, POST+GET /auth/logout
//   - Otherwise: GET+POST /auth/logout redirects to "/" (so the logout link
//     never 404s regardless of which provider is active)
func (m *AuthManager) Mount(r chi.Router) {
	if m.oidcH != nil {
		r.Get(loginPath, m.oidcH.handleLogin)
		r.Get(callbackPath, m.oidcH.handleCallback)
		r.Get(logoutPath, m.oidcH.handleLogout)
		r.Post(logoutPath, m.oidcH.handleLogout)
	} else {
		fallback := func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/", http.StatusFound)
		}
		r.Get(logoutPath, fallback)
		r.Post(logoutPath, fallback)
	}
}

// DomainRedirect returns middleware that redirects requests to the canonical
// address configured via [WithCanonicalAddress] when they arrive on a
// different scheme or host. Apply this to route groups that require the
// canonical domain (UI and API routes); do NOT apply it to short-link redirect
// routes so that unauthenticated public links are served from any hostname.
//
// When no canonical address is set, this middleware is a no-op.
func (m *AuthManager) DomainRedirect() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !redirectToCanonical(m.canonicalScheme, m.canonicalHost, m.trustedNets, w, r) {
				next.ServeHTTP(w, r)
			}
		})
	}
}

// APIKeyMiddleware returns a middleware that authenticates requests using an
// API key supplied via the "X-API-Key" header or "Authorization: Bearer
// <token>". When a valid key is found, the request context is populated with a
// synthetic [Identity]. Unknown keys are passed through so that downstream
// enforcement middleware can reject the request with an appropriate error.
//
// The lookup function is responsible for retrieving the key by its SHA-256
// hash (use [HashAPIKey] to compute it) and for any post-lookup side effects
// such as updating last_used_at.
func (m *AuthManager) APIKeyMiddleware(lookup APIKeyLookup) func(http.Handler) http.Handler {
	return apiKeyMiddlewareWith(lookup)
}

// SupportsLogout reports whether the active authentication configuration
// includes an application-level logout flow. Returns true only when OIDC is
// enabled (which issues a session cookie that can be cleared on logout).
// Use this in UI templates to decide whether to render a logout button.
func (m *AuthManager) SupportsLogout() bool {
	return m.oidcH != nil
}

// LoginPath returns the path of the login page, or "" when no login flow is
// available. Currently returns "/auth/login" when OIDC is enabled.
func (m *AuthManager) LoginPath() string {
	if m.oidcH != nil {
		return loginPath
	}
	return ""
}

// RedirectToCanonical checks whether r is already on the canonical address.
// If not, it writes a 301 redirect to w and returns true. Returns false when
// no canonical address is configured or the request already matches it.
//
// This is the same check performed by [AuthManager.DomainRedirect] but
// exposed as a method for inline use in handlers (e.g. to redirect a
// not-found page to the canonical domain before showing the error).
func (m *AuthManager) RedirectToCanonical(w http.ResponseWriter, r *http.Request) bool {
	return redirectToCanonical(m.canonicalScheme, m.canonicalHost, m.trustedNets, w, r)
}

// LoginRedirect redirects the browser to the login page, encoding the current
// request URI as the ?rd= post-login destination. It is a no-op (returns
// false) when no login flow is available.
func (m *AuthManager) LoginRedirect(w http.ResponseWriter, r *http.Request) bool {
	if m.oidcH == nil {
		return false
	}
	loginRedirect(w, r)
	return true
}

// recoverer returns a middleware that recovers from panics, logs them, and
// returns a 500 response.
func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered", "error", rec)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
