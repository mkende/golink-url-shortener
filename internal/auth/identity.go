// Package auth provides authentication middleware and helpers.
package auth

import (
	"context"
	"log/slog"

	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/mkende/golink-url-shortener/internal/db"
)

// AuthSource identifies how the user was authenticated.
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
	// APIKeyReadOnly is true when the identity was established by a read-only API
	// key. Such identities may not perform write or mutating operations.
	APIKeyReadOnly bool
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

// isAdmin reports whether the given identity has admin privileges according to
// the config's admin_emails list and admin_groups setting.
func isAdmin(cfg *config.Config, id *Identity) bool {
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

// upsertUserAsync fires off a background goroutine that upserts the user
// record. If users is nil the call is a no-op. Errors are logged at Warn level
// rather than silently discarded.
func upsertUserAsync(logger *slog.Logger, users db.UserRepo, email, name, avatar string) {
	if users == nil {
		return
	}
	go func() {
		if _, err := users.Upsert(context.Background(), email, name, avatar); err != nil {
			logger.Warn("async user upsert failed", "email", email, "error", err)
		}
	}()
}
