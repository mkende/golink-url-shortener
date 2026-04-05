package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/mkende/golink-url-shortener/internal/auth"
)

// HashAPIKey hashes a raw API key using SHA-256 for storage and lookup.
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// APIKeyMiddleware authenticates requests using an API key supplied via the
// "Authorization: Bearer <key>" header or the "X-API-Key" header. When a
// valid key is found the request context is populated with a synthetic
// Identity (IsAdmin=true) so that downstream handlers treat the caller as
// fully authorised.
func (s *Server) APIKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := extractAPIKey(r)
		if raw == "" {
			next.ServeHTTP(w, r)
			return
		}
		hash := HashAPIKey(raw)
		key, err := s.apiKeys.GetByHash(r.Context(), hash)
		if err != nil {
			// Unrecognised key — pass through without identity so downstream
			// auth middleware will reject the request with 401/403.
			next.ServeHTTP(w, r)
			return
		}
		// Update last_used_at asynchronously to avoid adding latency.
		// Use a detached context so cancellation of the request context does not
		// abort the update.
		keyID := key.ID
		go func() {
			s.apiKeys.UpdateLastUsed(context.Background(), keyID) //nolint:errcheck
		}()

		id := &auth.Identity{
			Email:          "apikey:" + key.Name,
			DisplayName:    key.Name,
			IsAdmin:        true,
			APIKeyReadOnly: key.ReadOnly,
		}
		next.ServeHTTP(w, r.WithContext(auth.WithIdentity(r.Context(), id)))
	})
}

// extractAPIKey returns the raw API key from the request, preferring the
// X-API-Key header and falling back to "Authorization: Bearer <token>".
func extractAPIKey(r *http.Request) string {
	if v := r.Header.Get("X-API-Key"); v != "" {
		return v
	}
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, "Bearer ") {
		return strings.TrimPrefix(v, "Bearer ")
	}
	return ""
}
