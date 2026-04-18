package server

import (
	"net/http"
	"strings"
)

// reassignPageData is the template data for /reassign.
type reassignPageData struct {
	baseData
	FromEmail  string
	ToEmail    string
	// Done is true when a reassignment was successfully attempted.
	Done       bool
	Reassigned int64
	Error      string
}

// handleReassignPage serves GET /reassign (admin only).
func (s *Server) handleReassignPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logr(r.Context()).Error("reassign: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	s.renderer.Render(w, "reassign", reassignPageData{baseData: base})
}

// handleReassignLinks serves POST /reassign (admin only).
func (s *Server) handleReassignLinks(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logr(r.Context()).Error("reassign: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if !requireCSRF(w, r) {
		return
	}

	fromEmail := strings.TrimSpace(r.FormValue("from_email"))
	toEmail := strings.TrimSpace(r.FormValue("to_email"))

	renderErr := func(errMsg string) {
		s.renderer.Render(w, "reassign", reassignPageData{
			baseData:  base,
			FromEmail: fromEmail,
			ToEmail:   toEmail,
			Error:     errMsg,
		})
	}

	if fromEmail == "" || toEmail == "" {
		renderErr("Both 'from' and 'to' email addresses are required.")
		return
	}
	if strings.EqualFold(fromEmail, toEmail) {
		renderErr("'From' and 'to' addresses must be different.")
		return
	}

	n, err := s.links.ReassignLinks(r.Context(), fromEmail, toEmail)
	if err != nil {
		s.logr(r.Context()).Error("reassign: db update", "from", fromEmail, "to", toEmail, "error", err)
		renderErr("Failed to reassign links. Please try again.")
		return
	}

	s.renderer.Render(w, "reassign", reassignPageData{
		baseData:   base,
		FromEmail:  fromEmail,
		ToEmail:    toEmail,
		Done:       true,
		Reassigned: n,
	})
}
