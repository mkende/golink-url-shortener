package httpauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// APIKeyInfo is the information returned by an [APIKeyLookup] function for a
// validated API key.
type APIKeyInfo struct {
	// ID is the key's database identifier; passed back to the lookup function
	// for any post-lookup side effects (e.g. updating last_used_at).
	ID int64
	// Name is a human-readable label for the key (used as Identity.DisplayName).
	Name string
	// ReadOnly indicates that this key may only perform read operations.
	ReadOnly bool
}

// APIKeyLookup looks up an API key by its SHA-256 hex hash. A nil return with
// nil error means the key was not found. The implementation is responsible for
// any post-lookup side effects such as asynchronously updating last_used_at.
type APIKeyLookup func(ctx context.Context, hash string) (*APIKeyInfo, error)

// HashAPIKey returns the SHA-256 hex hash of a raw API key. Use this when
// creating keys for storage and when building an [APIKeyLookup] function.
func HashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// apiKeyMiddlewareWith returns a middleware that authenticates requests using
// an API key supplied via "Authorization: Bearer <key>" or "X-API-Key" header.
// When a valid key is found, the request context is populated with a synthetic
// [Identity]. Unknown keys are passed through without an identity so that
// downstream auth enforcement can reject the request.
func apiKeyMiddlewareWith(lookup APIKeyLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractAPIKey(r)
			if raw == "" {
				next.ServeHTTP(w, r)
				return
			}
			hash := HashAPIKey(raw)
			key, err := lookup(r.Context(), hash)
			if err != nil || key == nil {
				next.ServeHTTP(w, r)
				return
			}
			id := &Identity{
				Email:          "apikey:" + key.Name,
				DisplayName:    key.Name,
				IsAdmin:        true,
				APIKeyReadOnly: key.ReadOnly,
				Source:         AuthSourceAPIKey,
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
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
