package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mkende/golink-redirector/internal/auth"
	"github.com/mkende/golink-redirector/internal/db"
	"github.com/mkende/golink-redirector/internal/links"
)

// importResult is the JSON summary returned after a POST /api/import request.
type importResult struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// handleImport serves POST /api/import (admin only).
// It reads an ExportData JSON body and upserts every link and its shares,
// returning a summary of created, updated, and skipped links.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var data ExportData
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	result := importResult{}

	for _, el := range data.Links {
		el.Name = strings.TrimSpace(el.Name)
		el.Target = strings.TrimSpace(el.Target)
		el.AliasTarget = strings.ToLower(strings.TrimSpace(el.AliasTarget))

		// Validate name.
		if err := links.ValidateName(el.Name); err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", el.Name, err))
			continue
		}

		// Parse the link type.
		linkType, err := linkTypeFromString(el.LinkType)
		if err != nil {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", el.Name, err))
			continue
		}

		// Validate target or alias_target depending on type.
		if linkType == db.LinkTypeAlias {
			if el.AliasTarget == "" {
				result.Skipped++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: alias_target is required for alias links", el.Name))
				continue
			}
		} else {
			if err := links.ValidateTarget(el.Target); err != nil {
				result.Skipped++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", el.Name, err))
				continue
			}
		}

		// Check whether the link already exists.
		existing, err := s.links.GetByName(ctx, strings.ToLower(el.Name))
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: db error looking up existing link", el.Name))
			continue
		}

		if existing != nil {
			// Update the existing link's mutable fields.
			if linkType == db.LinkTypeAlias {
				if _, err := s.links.SetAlias(ctx, existing.ID, el.Name, el.AliasTarget, el.RequireAuth, s.cfg.MaxAliasesPerLink); err != nil {
					result.Skipped++
					result.Errors = append(result.Errors, fmt.Sprintf("%s: update failed: %v", el.Name, err))
					continue
				}
			} else {
				if _, err := s.links.Update(ctx, existing.ID, el.Name, el.Target, linkType, el.RequireAuth); err != nil {
					result.Skipped++
					result.Errors = append(result.Errors, fmt.Sprintf("%s: update failed: %v", el.Name, err))
					continue
				}
			}
			result.Updated++
		} else {
			// Resolve the owner email; fall back to the importing admin.
			ownerEmail := el.OwnerEmail
			if ownerEmail == "" {
				if id := auth.FromContext(ctx); id != nil {
					ownerEmail = id.Email
				}
			}

			newLink, err := s.links.Create(ctx, el.Name, el.Target, ownerEmail, linkType, el.AliasTarget, el.RequireAuth)
			if err != nil {
				result.Skipped++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: create failed: %v", el.Name, err))
				continue
			}

			// Restore shares.
			for _, email := range el.Shares {
				if shareErr := s.links.AddShare(ctx, newLink.ID, email); shareErr != nil {
					s.logger.Error("import: add share", "link", el.Name, "email", email, "error", shareErr)
				}
			}

			result.Created++
		}
	}

	writeJSON(w, http.StatusOK, result)
}
