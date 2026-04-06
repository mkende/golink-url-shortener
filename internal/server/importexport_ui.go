package server

import (
	"encoding/json"
	"net/http"
)

// importExportPageData is the template data for /importexport.
type importExportPageData struct {
	baseData
	Result *importResult
	Error  string
}

// handleImportExportPage serves GET /importexport (admin only).
func (s *Server) handleImportExportPage(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logr(r.Context()).Error("importexport: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	s.renderer.Render(w, "importexport", importExportPageData{baseData: base})
}

// handleImportUpload serves POST /importexport (admin only).
// It accepts a JSON file upload and imports all links from it, then renders
// the page with a result summary.
func (s *Server) handleImportUpload(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logr(r.Context()).Error("importexport: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	renderError := func(msg string) {
		s.renderer.Render(w, "importexport", importExportPageData{
			baseData: base,
			Error:    msg,
		})
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		renderError("Could not parse form: " + err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		renderError("No file uploaded.")
		return
	}
	defer file.Close()

	var data ExportData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		renderError("Invalid JSON file: " + err.Error())
		return
	}

	result := s.doImport(r.Context(), data)

	s.renderer.Render(w, "importexport", importExportPageData{
		baseData: base,
		Result:   &result,
	})
}
