// Package server wires up the HTTP router and all middleware.
package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mkende/golink-url-shortener/internal/auth"
	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/mkende/golink-url-shortener/internal/db"
	serverMiddleware "github.com/mkende/golink-url-shortener/internal/server/middleware"
	"github.com/mkende/golink-url-shortener/internal/templates"
	"github.com/mkende/golink-url-shortener/web/static"
)

// Server is the root HTTP handler. It implements http.Handler via ServeHTTP.
type Server struct {
	router      chi.Router
	cfg         *config.Config
	links       db.LinkRepo
	users       db.UserRepo
	apiKeys     db.APIKeyRepo
	logger      *slog.Logger
	oidcH       *auth.OIDCHandler
	renderer    *templates.Renderer
	useCounter  *db.UseCounter
	trustedNets []*net.IPNet // parsed from cfg.TrustedProxy at construction time
}

// New creates a new Server, wires up all routes, and starts background
// goroutines. Call Shutdown to drain them gracefully. The oidcHandler may be
// nil when OIDC is disabled.
func New(cfg *config.Config, sqlDB *db.DB, logger *slog.Logger, oidcHandler *auth.OIDCHandler) *Server {
	renderer, err := templates.New()
	if err != nil {
		// Template parse errors are programmer errors; panic early so they are
		// caught during development rather than silently serving broken pages.
		panic("failed to parse templates: " + err.Error())
	}

	cacheSize := cfg.CacheSize
	if cacheSize <= 0 {
		cacheSize = 1000
	}
	baseRepo := db.NewLinkRepo(sqlDB)
	cachingRepo, err := db.NewCachingLinkRepo(baseRepo, cacheSize)
	if err != nil {
		panic("failed to create link cache: " + err.Error())
	}

	var trustedNets []*net.IPNet
	if len(cfg.TrustedProxy) > 0 {
		nets, err := auth.ParseCIDRs(cfg.TrustedProxy)
		if err != nil {
			panic("server: invalid trusted_proxy in config: " + err.Error())
		}
		trustedNets = nets
	}

	s := &Server{
		cfg:         cfg,
		links:       cachingRepo,
		users:       db.NewUserRepo(sqlDB),
		apiKeys:     db.NewAPIKeyRepo(sqlDB),
		logger:      logger,
		oidcH:       oidcHandler,
		renderer:    renderer,
		useCounter:  db.NewUseCounter(sqlDB, 2*time.Second),
		trustedNets: trustedNets,
	}
	s.router = s.buildRouter()
	return s
}

// ServeHTTP implements http.Handler, delegating to the underlying chi router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// Shutdown drains the use-count buffer and stops background goroutines. It
// should be called after the HTTP server has stopped accepting new requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.useCounter.Shutdown(ctx)
}

// logr returns the request-scoped logger if the LogEnricher middleware has
// populated one in ctx, otherwise falls back to the server-level logger.
// All handler-level log calls should use this instead of s.logger directly so
// that auth_source and domain are included automatically.
func (s *Server) logr(ctx context.Context) *slog.Logger {
	return serverMiddleware.LoggerFromContext(ctx, s.logger)
}

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	// Standard middleware
	// PreserveRemoteAddr must run before RealIP so that CIDR-based auth
	// middlewares can inspect the actual TCP connection address.
	r.Use(serverMiddleware.PreserveRemoteAddr)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(serverMiddleware.RequestLogger(s.logger))
	r.Use(serverMiddleware.SecurityHeaders)

	// Auth middleware: populate identity context from Tailscale headers,
	// reverse-proxy forward-auth headers, JWT cookie, or anonymous fallback
	// (in that priority order).
	r.Use(auth.TailscaleMiddleware(s.cfg, s.users, s.logger))
	r.Use(auth.ProxyAuthMiddleware(s.cfg, s.users, s.logger))
	r.Use(auth.OIDCMiddleware(s.cfg, s.logger))
	r.Use(auth.AnonymousMiddleware(s.cfg, s.logger))

	// Log enrichment: runs after all auth middleware so that the auth source
	// and request domain are available for both the request log line and any
	// handler-level log calls retrieved via s.logr(r.Context()).
	r.Use(serverMiddleware.LogEnricher(s.logger))

	// Favicon — served before domain redirect so browsers always get it.
	r.Get("/favicon.ico", s.handleFavicon)

	// Health check (no domain redirect)
	r.Get("/healthz", s.handleHealthz)

	// Auth routes (no domain redirect; OIDC callback must be reachable on the
	// registered redirect URL regardless of the current hostname)
	if s.oidcH != nil {
		r.Get("/auth/login", s.oidcH.HandleLogin)
		r.Get("/auth/callback", s.oidcH.HandleCallback)
		r.Post("/auth/logout", s.oidcH.HandleLogout)
		r.Get("/auth/logout", s.oidcH.HandleLogout)
	} else {
		// Fallback logout: redirect to home for non-OIDC modes (anonymous,
		// Tailscale) so the logout link never 404s.
		logoutFallback := func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/", http.StatusFound)
		}
		r.Get("/auth/logout", logoutFallback)
		r.Post("/auth/logout", logoutFallback)
	}

	// Redirect routes bypass domain redirect middleware
	r.Get("/{name}", s.handleRedirect)
	r.Get("/{name}/*", s.handleRedirect)

	// All other routes: apply domain redirect middleware
	r.Group(func(r chi.Router) {
		r.Use(serverMiddleware.DomainRedirect(s.cfg))
		r.Use(serverMiddleware.RequireUIAccess(s.cfg, http.HandlerFunc(s.handleUIAccessDenied)))

		// Landing page
		r.Get("/", s.handleIndex)

		// Link creation
		r.Get("/new", s.handleNew)
		r.Post("/new", s.handleNew)

		// Link details (view + edit); /edit/{name} redirects for backward compat
		r.Get("/details/{name}", s.handleDetails)
		r.Post("/details/{name}", s.handleDetails)
		r.Post("/details/{name}/share", s.handleDetailsShare)
		r.Post("/details/{name}/unshare", s.handleDetailsUnshare)
		r.Post("/details/{name}/delete", s.handleDetailsDelete)
		r.Post("/details/{name}/alias", s.handleCreateAlias)
		r.Get("/edit/{name}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/details/"+chi.URLParam(r, "name"), http.StatusMovedPermanently)
		})

		// Link browsing
		r.Get("/links", s.handleLinks)
		r.Get("/mylinks", s.handleMyLinks)

		// Help pages
		r.Get("/help", s.handleHelp)
		r.Get("/help/advanced", s.handleHelpAdvanced)
		r.Get("/help/search", s.handleHelpSearch)

		// Admin: API key management UI
		r.Route("/apikeys", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg))
			r.Use(auth.RequireAdmin(http.HandlerFunc(s.handleForbidden)))
			r.Get("/", s.handleAPIKeysPage)
			r.Post("/", s.handleCreateAPIKey)
			r.Post("/{id}/delete", s.handleDeleteAPIKey)
		})

		// Admin: import / export UI
		r.Route("/importexport", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg))
			r.Use(auth.RequireAdmin(http.HandlerFunc(s.handleForbidden)))
			r.Get("/", s.handleImportExportPage)
			r.Post("/", s.handleImportUpload)
		})
	})

	// API key middleware applied before the API sub-router so that API key
	// bearers get an Identity before RequireAuth runs.
	r.Group(func(r chi.Router) {
		r.Use(serverMiddleware.DomainRedirect(s.cfg))
		r.Use(s.APIKeyMiddleware)

		// REST API — all routes require authentication (session or API key).
		r.Route("/api", func(r chi.Router) {
			r.Use(auth.RequireAuth(s.cfg))

			// Link CRUD: reads are open to any authenticated caller;
			// writes additionally require write scope (blocks read-only API keys).
			r.Get("/links", s.handleAPIListLinks)
			r.With(auth.RequireWriteScope()).Post("/links", s.handleAPICreateLink)
			r.Get("/links/{name}", s.handleAPIGetLink)
			r.With(auth.RequireWriteScope()).Patch("/links/{name}", s.handleAPIUpdateLink)
			r.With(auth.RequireWriteScope()).Delete("/links/{name}", s.handleAPIDeleteLink)

			// Quick-name generator (also reachable by HTMX from the group above)
			r.Get("/quickname", s.handleQuickName)

			// API key management — admin only, always requires write scope.
			r.Route("/apikeys", func(r chi.Router) {
				r.Use(auth.RequireAdmin(http.HandlerFunc(s.handleForbidden)))
				r.Use(auth.RequireWriteScope())
				r.Get("/", s.handleAPIListAPIKeys)
				r.Post("/", s.handleAPICreateAPIKey)
				r.Delete("/{id}", s.handleAPIDeleteAPIKey)
			})

			// Export (read): admin only; read-only API keys may export.
			// Import (write): admin only, additionally requires write scope.
			r.With(auth.RequireAdmin(http.HandlerFunc(s.handleForbidden))).Get("/export", s.handleExport)
			r.With(auth.RequireAdmin(http.HandlerFunc(s.handleForbidden)), auth.RequireWriteScope()).Post("/import", s.handleImport)
		})
	})

	return r
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok")) //nolint:errcheck
}

// handleFavicon serves the favicon. When the operator has configured a custom
// favicon_path the file at that path is read from disk; otherwise the embedded
// default SVG favicon is served.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var contentType string

	if s.cfg.FaviconPath != "" {
		fileData, err := os.ReadFile(s.cfg.FaviconPath)
		if err != nil {
			s.logr(r.Context()).Error("favicon: read custom file", "path", s.cfg.FaviconPath, "error", err)
			http.NotFound(w, r)
			return
		}
		data = fileData
		// Determine content type from extension.
		switch {
		case strings.HasSuffix(s.cfg.FaviconPath, ".svg"):
			contentType = "image/svg+xml"
		case strings.HasSuffix(s.cfg.FaviconPath, ".png"):
			contentType = "image/png"
		default:
			contentType = "image/x-icon"
		}
	} else {
		fileData, err := static.FS.ReadFile("favicon.svg")
		if err != nil {
			s.logr(r.Context()).Error("favicon: read embedded file", "error", err)
			http.NotFound(w, r)
			return
		}
		data = fileData
		contentType = "image/svg+xml"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data) //nolint:errcheck
}
