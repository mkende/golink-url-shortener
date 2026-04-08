package auth

import (
	"log/slog"
	"net/http"

	"github.com/mkende/golink-url-shortener/internal/config"
)

// anonymousIdentity is the fixed identity used when anonymous auth is enabled.
// All requests share this single user, regardless of who is making them.
var anonymousIdentity = &Identity{
	Email:       "anonymous@localhost",
	DisplayName: "Anonymous",
}

// AnonymousMiddleware populates the identity context with a single shared
// anonymous user when [config.AnonymousConfig.Enabled] is true. It only acts
// when no identity has already been set by a prior middleware (Tailscale or
// OIDC), so the anonymous fallback never overrides a real authentication.
//
// This mode is intended for local development, testing, or isolated private
// instances where user management is unnecessary. Do not enable it on a
// publicly reachable server.
//
// If logger is nil, slog.Default() is used.
func AnonymousMiddleware(cfg *config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Anonymous.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			// Only apply if no real auth has already identified the user.
			if FromContext(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			id := &Identity{
				Email:       anonymousIdentity.Email,
				DisplayName: anonymousIdentity.DisplayName,
				IsAdmin:     cfg.Anonymous.IsAdmin,
				Source:      AuthSourceAnonymous,
			}
			logger.DebugContext(r.Context(), "anonymous: no prior auth; using anonymous identity",
				"is_admin", id.IsAdmin,
			)
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}
