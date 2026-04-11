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

// renderImportError re-renders the importexport page with an error message.
func (s *Server) renderImportError(w http.ResponseWriter, base baseData, msg string) {
	s.renderer.Render(w, "importexport", importExportPageData{baseData: base, Error: msg})
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

	if !requireCSRF(w, r) {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		s.renderImportError(w, base, "Could not parse form: "+err.Error())
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		s.renderImportError(w, base, "No file uploaded.")
		return
	}
	defer file.Close()

	var data ExportData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		s.renderImportError(w, base, "Invalid JSON file: "+err.Error())
		return
	}

	result := s.doImport(r.Context(), data)

	s.renderer.Render(w, "importexport", importExportPageData{
		baseData: base,
		Result:   &result,
	})
}
