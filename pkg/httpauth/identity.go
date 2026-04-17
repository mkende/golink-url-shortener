package httpauth

import "context"

// AuthSource identifies how the current request was authenticated.
type AuthSource string

const (
	// AuthSourceOIDC indicates authentication via OpenID Connect.
	AuthSourceOIDC AuthSource = "oidc"
	// AuthSourceTailscale indicates authentication via Tailscale headers.
	AuthSourceTailscale AuthSource = "tailscale"
	// AuthSourceProxy indicates authentication via reverse-proxy forward-auth headers.
	AuthSourceProxy AuthSource = "proxy"
	// AuthSourceAnonymous indicates the anonymous fallback identity.
	AuthSourceAnonymous AuthSource = "anonymous"
	// AuthSourceAPIKey indicates authentication via a bearer API key.
	AuthSourceAPIKey AuthSource = "apikey"
)

// Identity holds the authenticated user's information for a single request.
// It is stored in the request context by the auth provider middlewares and
// read by enforcement middleware and application handlers.
type Identity struct {
	// Email is the primary user identifier used throughout the system.
	Email string
	// DisplayName is a human-readable name (may be empty).
	DisplayName string
	// AvatarURL is a URL to the user's profile picture (may be empty).
	AvatarURL string
	// Groups contains the user's group memberships (OIDC groups claim or
	// proxy GroupsHeader). May be nil when the provider does not supply groups.
	Groups []string
	// IsAdmin is true when the identity matches an admin email or admin group
	// configured in [AuthConfig].
	IsAdmin bool
	// Source identifies which authentication mechanism produced this identity.
	Source AuthSource
	// APIKeyReadOnly is true when the identity was established by a read-only
	// API key. Such identities may not perform write or mutating operations.
	APIKeyReadOnly bool
}

type identityContextKey struct{}

// WithIdentity returns a new context that carries id.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, id)
}

// IdentityFromContext returns the [Identity] stored in ctx, or nil when the
// request has not been authenticated by any provider.
func IdentityFromContext(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityContextKey{}).(*Identity)
	return id
}

// isAdmin reports whether id has admin privileges according to cfg.
func isAdmin(cfg AuthConfig, id *Identity) bool {
	for _, email := range cfg.AdminEmails {
		if email == id.Email {
			return true
		}
	}
	for _, adminGroup := range cfg.AdminGroups {
		for _, g := range id.Groups {
			if g == adminGroup {
				return true
			}
		}
	}
	return false
}
