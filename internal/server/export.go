package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mkende/golink-url-shortener/internal/db"
)

// ExportLink is the JSON representation of a link for export/import.
type ExportLink struct {
	Name        string    `json:"name"`
	Target      string    `json:"target"`
	OwnerEmail  string    `json:"owner_email"`
	// LinkType is one of "simple", "advanced", or "alias".
	LinkType    string    `json:"link_type"`
	// AliasTarget is the canonical link name; only present for alias links.
	AliasTarget string    `json:"alias_target,omitempty"`
	RequireAuth bool      `json:"require_auth"`
	CreatedAt   time.Time `json:"created_at"`
	UseCount    int64     `json:"use_count"`
	Shares      []string  `json:"shares,omitempty"`
}

// ExportData is the top-level export JSON structure.
type ExportData struct {
	Version    int          `json:"version"`
	ExportedAt time.Time    `json:"exported_at"`
	Links      []ExportLink `json:"links"`
}

// handleExport serves GET /api/export (admin only).
// It streams the full link database as JSON without loading all records into
// memory at once; links are fetched 500 at a time.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="golink-export.json"`)

	// Write the JSON prefix manually so we can stream individual link objects
	// without buffering the whole dataset.
	exportedAt := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(w, `{"version":1,"exported_at":%q,"links":[`, exportedAt) //nolint:errcheck

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	const pageSize = 500
	offset := 0
	first := true

	for {
		linkPage, _, err := s.links.List(ctx, pageSize, offset, db.SortByName, db.SortAsc, false)
		if err != nil {
			// Headers and partial body are already sent; we can only log and stop.
			s.logr(r.Context()).Error("export: list links", "offset", offset, "error", err)
			return
		}
		if len(linkPage) == 0 {
			break
		}

		for _, link := range linkPage {
			shares, err := s.links.GetShares(ctx, link.ID)
			if err != nil {
				s.logr(r.Context()).Error("export: get shares", "link_id", link.ID, "error", err)
				shares = nil
			}

			el := ExportLink{
				Name:        link.Name,
				Target:      link.Target,
				OwnerEmail:  link.OwnerEmail,
				LinkType:    linkTypeToString(link.LinkType),
				AliasTarget: link.AliasTarget,
				RequireAuth: link.RequireAuth,
				CreatedAt:   link.CreatedAt,
				UseCount:    link.UseCount,
				Shares:      shares,
			}

			if !first {
				fmt.Fprint(w, ",") //nolint:errcheck
			}
			first = false

			enc.Encode(el) //nolint:errcheck
		}

		offset += len(linkPage)
		if len(linkPage) < pageSize {
			break
		}
	}

	fmt.Fprint(w, "]}") //nolint:errcheck
}
