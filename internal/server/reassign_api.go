package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

type reassignRequest struct {
	FromEmail string `json:"from_email"`
	ToEmail   string `json:"to_email"`
}

type reassignResponse struct {
	Reassigned int64 `json:"reassigned"`
}

// handleAPIReassignLinks serves POST /api/admin/reassign-links (admin only).
// It transfers ownership of all links from one user to another.
func (s *Server) handleAPIReassignLinks(w http.ResponseWriter, r *http.Request) {
	var req reassignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	fromEmail := strings.TrimSpace(req.FromEmail)
	toEmail := strings.TrimSpace(req.ToEmail)

	if fromEmail == "" || toEmail == "" {
		writeJSONError(w, http.StatusBadRequest, "from_email and to_email are required")
		return
	}
	if strings.EqualFold(fromEmail, toEmail) {
		writeJSONError(w, http.StatusBadRequest, "from_email and to_email must be different")
		return
	}

	n, err := s.links.ReassignLinks(r.Context(), fromEmail, toEmail)
	if err != nil {
		s.apiError(r.Context(), w, "reassign links", "from", fromEmail, "to", toEmail, "error", err)
		return
	}

	writeJSON(w, http.StatusOK, reassignResponse{Reassigned: n})
}
