package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mkende/golink-redirector/internal/db"
)

// LinkResponse is the JSON representation of a short link returned by the API.
type LinkResponse struct {
	Name        string    `json:"name"`
	Target      string    `json:"target"`
	OwnerEmail  string    `json:"owner_email"`
	IsAdvanced  bool      `json:"is_advanced"`
	RequireAuth bool      `json:"require_auth"`
	CreatedAt   time.Time `json:"created_at"`
	UseCount    int64     `json:"use_count"`
}

// APIKeyResponse is the JSON representation of an API key record. The raw key
// value is never included except on creation.
type APIKeyResponse struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	CreatedBy string  `json:"created_by"`
	CreatedAt string  `json:"created_at"`
	LastUsed  *string `json:"last_used,omitempty"`
}

// isJSONRequest returns true when the caller prefers a JSON response, i.e.
// when the Accept or Content-Type header contains "application/json".
func isJSONRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	ct := r.Header.Get("Content-Type")
	return strings.Contains(accept, "application/json") ||
		strings.Contains(ct, "application/json")
}

// writeJSON writes v as a JSON response with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeJSONError writes a JSON object {"error": message} with the given status.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// linkToResponse converts a db.Link to a LinkResponse suitable for JSON output.
func linkToResponse(l *db.Link) LinkResponse {
	return LinkResponse{
		Name:        l.Name,
		Target:      l.Target,
		OwnerEmail:  l.OwnerEmail,
		IsAdvanced:  l.IsAdvanced,
		RequireAuth: l.RequireAuth,
		CreatedAt:   l.CreatedAt,
		UseCount:    l.UseCount,
	}
}

// apiKeyToResponse converts a db.APIKey to an APIKeyResponse. The raw key is
// never present on this type.
func apiKeyToResponse(k *db.APIKey) APIKeyResponse {
	r := APIKeyResponse{
		ID:        k.ID,
		Name:      k.Name,
		CreatedBy: k.CreatedBy,
		CreatedAt: k.CreatedAt.Format(time.RFC3339),
	}
	if k.LastUsedAt.Valid {
		s := k.LastUsedAt.Time.Format(time.RFC3339)
		r.LastUsed = &s
	}
	return r
}
