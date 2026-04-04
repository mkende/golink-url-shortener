// Package auth provides authentication middleware and helpers.
package auth

import "context"

// AuthSource identifies how the user was authenticated.
type AuthSource string

const (
	// AuthSourceOIDC indicates authentication via OpenID Connect.
	AuthSourceOIDC AuthSource = "oidc"
	// AuthSourceTailscale indicates authentication via Tailscale headers.
	AuthSourceTailscale AuthSource = "tailscale"
	// AuthSourceAnonymous indicates the anonymous fallback identity.
	AuthSourceAnonymous AuthSource = "anonymous"
)

// Identity holds the authenticated user's information.
type Identity struct {
	Email       string
	DisplayName string
	AvatarURL   string
	Groups      []string
	IsAdmin     bool
	// Source identifies which authentication mechanism produced this identity.
	Source AuthSource
}

type contextKey int

const identityKey contextKey = iota

// WithIdentity returns a new context with the given identity.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// FromContext returns the identity from the context, or nil if not authenticated.
func FromContext(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityKey).(*Identity)
	return id
}
