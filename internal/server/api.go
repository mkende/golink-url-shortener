package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mkende/golink-redirector/internal/auth"
	"github.com/mkende/golink-redirector/internal/db"
	"github.com/mkende/golink-redirector/internal/links"
)

// generateAPIKey generates a cryptographically random 32-byte URL-safe base64
// encoded string suitable for use as a raw API key.
func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// createLinkRequest is the JSON body expected by POST /api/links.
// link_type must be "simple" (default), "advanced", or "alias".
// For alias links provide alias_target (the canonical link name) instead of target.
type createLinkRequest struct {
	Name        string `json:"name"`
	Target      string `json:"target"`
	LinkType    string `json:"link_type"`
	AliasTarget string `json:"alias_target"`
	RequireAuth bool   `json:"require_auth"`
}

// updateLinkRequest is the JSON body expected by PATCH /api/links/:name.
// Only fields present in the JSON document are applied (field-mask semantics
// are approximated via pointers).
type updateLinkRequest struct {
	Target      *string `json:"target"`
	LinkType    *string `json:"link_type"`
	AliasTarget *string `json:"alias_target"`
	RequireAuth *bool   `json:"require_auth"`
}

// listLinksResponse is the JSON envelope returned by GET /api/links.
type listLinksResponse struct {
	Links      []LinkResponse `json:"links"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	TotalPages int            `json:"total_pages"`
}

// handleAPIListLinks serves GET /api/links.
// Query params: page (1-based), limit (max 100), q (search), sort, dir.
func (s *Server) handleAPIListLinks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := 1
	if p, err := strconv.Atoi(q.Get("page")); err == nil && p > 0 {
		page = p
	}
	limit := 100
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	offset := (page - 1) * limit
	query := q.Get("q")

	sortField := db.SortByName
	if sf := q.Get("sort"); sf != "" {
		switch sf {
		case "name":
			sortField = db.SortByName
		case "created":
			sortField = db.SortByCreated
		case "last_used":
			sortField = db.SortByLastUsed
		case "use_count":
			sortField = db.SortByUseCount
		default:
			writeJSONError(w, http.StatusBadRequest, "invalid sort field: "+sf)
			return
		}
	}
	sortDir := db.SortAsc
	if dir := q.Get("dir"); strings.EqualFold(dir, "desc") {
		sortDir = db.SortDesc
	}

	var (
		linkList []*db.Link
		total    int
		err      error
	)
	if query != "" {
		linkList, total, err = s.links.Search(r.Context(), query, limit, offset)
	} else {
		linkList, total, err = s.links.List(r.Context(), limit, offset, sortField, sortDir)
	}
	if err != nil {
		s.logger.Error("api: list links", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	resp := listLinksResponse{
		Links:      make([]LinkResponse, len(linkList)),
		Total:      total,
		Page:       page,
		TotalPages: totalPages(total, limit),
	}
	for i, l := range linkList {
		resp.Links[i] = linkToResponse(l)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAPICreateLink serves POST /api/links.
func (s *Server) handleAPICreateLink(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	if id == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Target = strings.TrimSpace(req.Target)
	req.AliasTarget = strings.ToLower(strings.TrimSpace(req.AliasTarget))

	if err := links.ValidateName(req.Name); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	linkType, err := linkTypeFromString(req.LinkType)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if linkType == db.LinkTypeAlias {
		if req.AliasTarget == "" {
			writeJSONError(w, http.StatusBadRequest, "alias_target is required for alias links")
			return
		}
		// Resolve alias target to its canonical (non-alias) form.
		req.AliasTarget, err = s.resolveAliasTarget(r, req.AliasTarget, "")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		// Check alias limit.
		count, countErr := s.links.CountAliases(r.Context(), req.AliasTarget)
		if countErr != nil {
			s.logger.Error("api: count aliases", "target", req.AliasTarget, "error", countErr)
			writeJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if count >= s.cfg.MaxAliasesPerLink {
			writeJSONError(w, http.StatusUnprocessableEntity, "alias limit reached for this link")
			return
		}
	} else {
		if err := links.ValidateTarget(req.Target); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	link, err := s.links.Create(r.Context(), req.Name, req.Target, id.Email, linkType, req.AliasTarget, req.RequireAuth)
	if err != nil {
		// Treat unique-constraint violations as conflict.
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			writeJSONError(w, http.StatusConflict, "link name already exists")
			return
		}
		s.logger.Error("api: create link", "name", req.Name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, linkToResponse(link))
}

// handleAPIGetLink serves GET /api/links/{name}.
func (s *Server) handleAPIGetLink(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))
	link, err := s.links.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "link not found")
			return
		}
		s.logger.Error("api: get link", "name", name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, linkToResponse(link))
}

// handleAPIUpdateLink serves PATCH /api/links/{name}.
// Only supplied fields are updated (field-mask semantics via JSON null vs omit).
func (s *Server) handleAPIUpdateLink(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))

	id := auth.FromContext(r.Context())
	if id == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	link, err := s.links.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "link not found")
			return
		}
		s.logger.Error("api: update link get", "name", name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if !canEdit(id, link) {
		// Check the share list before denying access.
		ok, checkErr := s.isSharedWith(r.Context(), link.ID, id)
		if checkErr != nil {
			s.logger.Error("api: update link check shares", "name", name, "error", checkErr)
			writeJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if !ok {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	var req updateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Apply field mask: only overwrite fields explicitly present in the request.
	newLinkType := link.LinkType
	if req.LinkType != nil {
		lt, err := linkTypeFromString(*req.LinkType)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		newLinkType = lt
	}

	newRequireAuth := link.RequireAuth
	if req.RequireAuth != nil {
		newRequireAuth = *req.RequireAuth
	}

	if newLinkType == db.LinkTypeAlias {
		newAliasTarget := link.AliasTarget
		if req.AliasTarget != nil {
			newAliasTarget = strings.ToLower(strings.TrimSpace(*req.AliasTarget))
		}
		if newAliasTarget == "" {
			writeJSONError(w, http.StatusBadRequest, "alias_target is required for alias links")
			return
		}
		// Resolve to canonical, preventing loops.
		newAliasTarget, err = s.resolveAliasTarget(r, newAliasTarget, link.NameLower)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		updated, err := s.links.SetAlias(r.Context(), link.ID, link.Name, newAliasTarget, newRequireAuth, s.cfg.MaxAliasesPerLink)
		if err != nil {
			if errors.Is(err, db.ErrAliasLimitExceeded) {
				writeJSONError(w, http.StatusUnprocessableEntity, "alias limit reached for this link")
				return
			}
			s.logger.Error("api: set alias", "id", link.ID, "error", err)
			writeJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		writeJSON(w, http.StatusOK, linkToResponse(updated))
		return
	}

	// Simple or advanced link.
	newTarget := link.Target
	if req.Target != nil {
		newTarget = strings.TrimSpace(*req.Target)
		if err := links.ValidateTarget(newTarget); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	updated, err := s.links.Update(r.Context(), link.ID, link.Name, newTarget, newLinkType, newRequireAuth)
	if err != nil {
		s.logger.Error("api: update link", "id", link.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, linkToResponse(updated))
}

// handleAPIDeleteLink serves DELETE /api/links/{name}.
func (s *Server) handleAPIDeleteLink(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))

	id := auth.FromContext(r.Context())
	if id == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	link, err := s.links.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "link not found")
			return
		}
		s.logger.Error("api: delete link get", "name", name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Only the owner or an admin may delete a link.
	if !id.IsAdmin && !strings.EqualFold(id.Email, link.OwnerEmail) {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := s.links.Delete(r.Context(), link.ID); err != nil {
		s.logger.Error("api: delete link", "id", link.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveAliasTarget follows at most one level of alias indirection to find
// the canonical (non-alias) link name.  Returns an error if the target does
// not exist, is itself an alias that resolves to selfNameLower (a loop), or
// resolves to a link that is also an alias (should not happen after a single
// resolution step but is checked for safety).
// selfNameLower is the name_lower of the link being edited; pass "" when
// creating a new alias.
func (s *Server) resolveAliasTarget(r *http.Request, aliasTargetLower, selfNameLower string) (string, error) {
	if aliasTargetLower == selfNameLower && selfNameLower != "" {
		return "", errors.New("a link cannot be an alias of itself")
	}
	targetLink, err := s.links.GetByName(r.Context(), aliasTargetLower)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", errors.New("alias target link does not exist: " + aliasTargetLower)
		}
		return "", errors.New("error looking up alias target")
	}
	if targetLink.IsAlias() {
		// Resolve one level: use its canonical target.
		aliasTargetLower = targetLink.AliasTarget
		if aliasTargetLower == selfNameLower && selfNameLower != "" {
			return "", errors.New("alias target would create a circular reference")
		}
		// Verify the resolved canonical exists.
		if _, err := s.links.GetByName(r.Context(), aliasTargetLower); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return "", errors.New("alias target's canonical link does not exist: " + aliasTargetLower)
			}
			return "", errors.New("error looking up canonical link")
		}
	}
	return aliasTargetLower, nil
}

// --- API key management handlers ---

// handleAPIListAPIKeys serves GET /api/apikeys (admin only).
func (s *Server) handleAPIListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.apiKeys.List(r.Context())
	if err != nil {
		s.logger.Error("api: list api keys", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	resp := make([]APIKeyResponse, len(keys))
	for i, k := range keys {
		resp[i] = apiKeyToResponse(k)
	}
	writeJSON(w, http.StatusOK, resp)
}

// createAPIKeyResponse is the JSON response for POST /api/apikeys; includes the
// raw key exactly once.
type createAPIKeyResponse struct {
	APIKeyResponse
	RawKey string `json:"raw_key"`
}

// handleAPICreateAPIKey serves POST /api/apikeys (admin only).
// Returns the raw key once; subsequent lookups will not reveal it.
func (s *Server) handleAPICreateAPIKey(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	if id == nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	rawKey, err := generateAPIKey()
	if err != nil {
		s.logger.Error("api: generate api key", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	key, err := s.apiKeys.Create(r.Context(), body.Name, HashAPIKey(rawKey), id.Email)
	if err != nil {
		s.logger.Error("api: create api key", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusCreated, createAPIKeyResponse{
		APIKeyResponse: apiKeyToResponse(key),
		RawKey:         rawKey,
	})
}

// handleAPIDeleteAPIKey serves DELETE /api/apikeys/{id} (admin only).
func (s *Server) handleAPIDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	keyID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.apiKeys.Delete(r.Context(), keyID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "api key not found")
			return
		}
		s.logger.Error("api: delete api key", "id", keyID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
