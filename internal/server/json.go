package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mkende/golink-url-shortener/internal/db"
)

// Request-body size limits, guarding against memory/disk exhaustion. Ordinary
// API calls carry a single small record; imports may legitimately contain a
// full backup, so they get a much larger cap.
const (
	maxAPIBodyBytes    = 1 << 20   // 1 MiB
	maxImportBodyBytes = 256 << 20 // 256 MiB
)

// decodeJSONBody decodes the request body into v, enforcing maxBytes as an
// upper bound. It writes a 413 response when the body exceeds the limit, a 400
// when it is not valid JSON, and returns false in either case. On success it
// returns true and v is populated.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

// LinkResponse is the JSON representation of a short link returned by the API.
type LinkResponse struct {
	Name       string `json:"name"`
	Target     string `json:"target"`
	OwnerEmail string `json:"owner_email"`
	// LinkType is one of "simple", "advanced", or "alias".
	LinkType string `json:"link_type"`
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
	ReadOnly  bool    `json:"read_only"`
}

// writeJSON writes v as a JSON response with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeJSONError writes a JSON object {"error": message} with the given status.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// apiError logs the error at Error level and writes a JSON 500 response. It is
// a convenience wrapper for the common pattern in API handlers where an
// unexpected internal error should be logged and surfaced as a generic 500.
func (s *Server) apiError(ctx context.Context, w http.ResponseWriter, logMsg string, args ...any) {
	s.logr(ctx).Error(logMsg, args...)
	writeJSONError(w, http.StatusInternalServerError, "internal server error")
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
		ReadOnly:  k.ReadOnly,
	}
	if k.LastUsedAt.Valid {
		s := k.LastUsedAt.Time.Format(time.RFC3339)
		r.LastUsed = &s
	}
	return r
}
