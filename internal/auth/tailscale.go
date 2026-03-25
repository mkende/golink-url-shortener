package auth

import (
	"context"
	"net/http"

	"github.com/mkende/golink-redirector/internal/config"
	"github.com/mkende/golink-redirector/internal/db"
)

// TailscaleMiddleware reads Tailscale-User-* headers and populates the identity
// context. If the header is absent or Tailscale auth is disabled, it passes
// through unchanged. When a UserRepo is provided, the user record is upserted
// asynchronously on each authenticated request.
func TailscaleMiddleware(cfg *config.Config, users db.UserRepo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Tailscale.Enabled {
				next.ServeHTTP(w, r)
				return
			}
			login := r.Header.Get("Tailscale-User-Login")
			if login == "" {
				next.ServeHTTP(w, r)
				return
			}
			id := &Identity{
				Email:       login,
				DisplayName: r.Header.Get("Tailscale-User-Name"),
				AvatarURL:   r.Header.Get("Tailscale-User-Profile-Pic"),
			}
			id.IsAdmin = isAdmin(cfg, id)

			if users != nil {
				go func() {
					if _, err := users.Upsert(context.Background(), id.Email, id.DisplayName, id.AvatarURL); err != nil {
						// Best-effort; errors are not surfaced to the caller.
						_ = err
					}
				}()
			}

			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

// isAdmin reports whether the given identity has admin privileges according to
// the config's admin_emails list and admin_group setting.
func isAdmin(cfg *config.Config, id *Identity) bool {
	for _, email := range cfg.AdminEmails {
		if email == id.Email {
			return true
		}
	}
	if cfg.AdminGroup != "" {
		for _, g := range id.Groups {
			if g == cfg.AdminGroup {
				return true
			}
		}
	}
	return false
}
