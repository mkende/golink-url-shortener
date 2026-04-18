package httpauth

import "net/http"

// anonymousMiddleware populates the identity context with a shared anonymous
// user when [AnonymousConfig.Enabled] is true. It only acts when no prior
// provider has already set an identity, so it never overrides real auth.
//
// This mode is intended for local development, testing, or isolated private
// instances where user management is unnecessary. Do not enable it on a
// publicly reachable server.
func (m *AuthManager) anonymousMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !m.cfg.Anonymous.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			if IdentityFromContext(r.Context()) != nil {
				next.ServeHTTP(w, r)
				return
			}
			id := &Identity{
				Email:       "anonymous@localhost",
				DisplayName: "Anonymous",
				IsAdmin:     m.cfg.Anonymous.IsAdmin,
				Source:      AuthSourceAnonymous,
			}
			m.logger.DebugContext(r.Context(), "anonymous: no prior auth; using anonymous identity",
				"is_admin", id.IsAdmin,
			)
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}
