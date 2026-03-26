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
	// LinkType is one of "simple", "advanced", or "alias".
	LinkType    string    `json:"link_type"`
	// AliasTarget is the lower-cased canonical link name; only present for
	// alias links.
	AliasTarget string    `json:"alias_target,omitempty"`
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

// linkTypeToString converts a db.LinkType to its API string representation.
func linkTypeToString(lt db.LinkType) string {
	switch lt {
	case db.LinkTypeAdvanced:
		return "advanced"
	case db.LinkTypeAlias:
		return "alias"
	default:
		return "simple"
	}
}

// linkTypeFromString parses an API link_type string. Returns LinkTypeSimple
// for unknown values and a non-nil error.
func linkTypeFromString(s string) (db.LinkType, error) {
	switch strings.ToLower(s) {
	case "advanced":
		return db.LinkTypeAdvanced, nil
	case "alias":
		return db.LinkTypeAlias, nil
	case "simple", "":
		return db.LinkTypeSimple, nil
	default:
		return db.LinkTypeSimple, &invalidLinkTypeError{s}
	}
}

type invalidLinkTypeError struct{ val string }

func (e *invalidLinkTypeError) Error() string {
	return "invalid link_type: " + e.val + "; must be \"simple\", \"advanced\", or \"alias\""
}

// linkToResponse converts a db.Link to a LinkResponse suitable for JSON output.
func linkToResponse(l *db.Link) LinkResponse {
	return LinkResponse{
		Name:        l.Name,
		Target:      l.Target,
		OwnerEmail:  l.OwnerEmail,
		LinkType:    linkTypeToString(l.LinkType),
		AliasTarget: l.AliasTarget,
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
