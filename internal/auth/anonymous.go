package auth

import (
	"net/http"

	"github.com/mkende/golink-redirector/internal/config"
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
func AnonymousMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
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
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}
