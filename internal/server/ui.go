package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mkende/golink-redirector/internal/auth"
	"github.com/mkende/golink-redirector/internal/db"
	"github.com/mkende/golink-redirector/internal/links"
)

const linksPerPage = 100

// baseData holds the template data common to all pages.
type baseData struct {
	// Title is the site name shown in the navigation bar and browser tab.
	Title string
	// Identity is the currently authenticated user, or nil when anonymous.
	Identity *auth.Identity
	// CSRFToken is the per-request CSRF protection value injected into forms.
	CSRFToken string
	// FaviconPath is non-empty when a custom favicon is configured.
	FaviconPath string
}

// newBaseData populates baseData from the current request and writes a fresh
// CSRF cookie to w.
func (s *Server) newBaseData(w http.ResponseWriter, r *http.Request) (baseData, error) {
	token, err := generateCSRFToken()
	if err != nil {
		return baseData{}, fmt.Errorf("generate CSRF token: %w", err)
	}
	setCSRFCookie(w, token)
	return baseData{
		Title:       s.cfg.Title,
		Identity:    auth.FromContext(r.Context()),
		CSRFToken:   token,
		FaviconPath: s.cfg.FaviconPath,
	}, nil
}

// indexData is the template data for the landing page.
type indexData struct {
	baseData
	RecentLinks  []*db.Link
	PopularLinks []*db.Link
}

// handleIndex serves GET /.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logger.Error("index: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	data := indexData{baseData: base}

	// Show the authenticated user's recent links.
	if base.Identity != nil {
		recent, _, err := s.links.ListByOwner(r.Context(), base.Identity.Email, 5, 0)
		if err != nil {
			s.logger.Error("index: list by owner", "error", err)
		} else {
			data.RecentLinks = recent
		}
	}

	// Show popular links by use count.
	popular, _, err := s.links.List(r.Context(), 5, 0, db.SortByUseCount, db.SortDesc)
	if err != nil {
		s.logger.Error("index: list popular", "error", err)
	} else {
		data.PopularLinks = popular
	}

	s.renderer.Render(w, "index", data)
}

// newLinkForm holds form field values for the create-link page.
type newLinkForm struct {
	Name        string
	Target      string
	IsAdvanced  bool
	RequireAuth bool
}

// newPageData is the template data for /new.
type newPageData struct {
	baseData
	Error string
	Form  newLinkForm
}

// handleNew serves GET and POST /new.
func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logger.Error("new: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodGet {
		s.renderer.Render(w, "new", newPageData{baseData: base})
		return
	}

	// POST — create the link.
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	id := auth.FromContext(r.Context())
	if id == nil {
		http.Redirect(w, r, "/auth/login?rd=/new", http.StatusFound)
		return
	}

	form := newLinkForm{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Target:      strings.TrimSpace(r.FormValue("target")),
		IsAdvanced:  r.FormValue("is_advanced") == "on",
		RequireAuth: r.FormValue("require_auth") == "on",
	}

	renderError := func(msg string) {
		s.renderer.Render(w, "new", newPageData{baseData: base, Error: msg, Form: form})
	}

	if err := links.ValidateName(form.Name); err != nil {
		renderError(err.Error())
		return
	}
	if err := links.ValidateTarget(form.Target); err != nil {
		renderError(err.Error())
		return
	}

	_, err = s.links.Create(r.Context(), form.Name, form.Target, id.Email, form.IsAdvanced, form.RequireAuth)
	if err != nil {
		s.logger.Error("new: create link", "name", form.Name, "error", err)
		renderError("Could not create link: " + err.Error())
		return
	}

	http.Redirect(w, r, "/edit/"+form.Name, http.StatusFound)
}

// editPageData is the template data for /edit/{name}.
type editPageData struct {
	baseData
	Link       *db.Link
	Shares     []string
	KnownUsers []*db.User
	Error      string
	Success    string
}

// handleEdit serves GET /edit/{name} and POST /edit/{name}.
func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))

	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logger.Error("edit: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	link, err := s.links.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.logger.Error("edit: get link", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	id := auth.FromContext(r.Context())
	if !canEdit(id, link) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodGet {
		data, err := s.buildEditPageData(r, base, link)
		if err != nil {
			s.logger.Error("edit: build page data", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		s.renderer.Render(w, "edit", data)
		return
	}

	// POST — save changes.
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	target := strings.TrimSpace(r.FormValue("target"))
	isAdvanced := r.FormValue("is_advanced") == "on"
	requireAuth := r.FormValue("require_auth") == "on"

	renderError := func(msg string) {
		data, buildErr := s.buildEditPageData(r, base, link)
		if buildErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		data.Error = msg
		s.renderer.Render(w, "edit", data)
	}

	if err := links.ValidateTarget(target); err != nil {
		renderError(err.Error())
		return
	}

	updated, err := s.links.Update(r.Context(), link.ID, link.Name, target, isAdvanced, requireAuth)
	if err != nil {
		s.logger.Error("edit: update link", "id", link.ID, "error", err)
		renderError("Could not save changes: " + err.Error())
		return
	}

	data, err := s.buildEditPageData(r, base, updated)
	if err != nil {
		s.logger.Error("edit: build page data after update", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	data.Success = "Link saved."
	s.renderer.Render(w, "edit", data)
}

// buildEditPageData assembles editPageData for the edit page.
func (s *Server) buildEditPageData(r *http.Request, base baseData, link *db.Link) (editPageData, error) {
	shares, err := s.links.GetShares(r.Context(), link.ID)
	if err != nil {
		return editPageData{}, fmt.Errorf("get shares: %w", err)
	}
	knownUsers, err := s.users.List(r.Context(), 200, 0)
	if err != nil {
		// Non-fatal: autocomplete just won't be populated.
		s.logger.Error("edit: list users for autocomplete", "error", err)
		knownUsers = nil
	}
	return editPageData{
		baseData:   base,
		Link:       link,
		Shares:     shares,
		KnownUsers: knownUsers,
	}, nil
}

// handleEditShare serves POST /edit/{name}/share.
func (s *Server) handleEditShare(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	link, id, ok := s.requireEditAccess(w, r, name)
	if !ok {
		return
	}
	_ = id

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		http.Redirect(w, r, "/edit/"+name, http.StatusFound)
		return
	}

	// Append default domain if bare name given (no @).
	if !strings.Contains(email, "@") && s.cfg.DefaultDomain != "" {
		email = email + "@" + s.cfg.DefaultDomain
	}

	// Enforce required domain restriction.
	if s.cfg.RequiredDomain != "" && !strings.HasSuffix(email, "@"+s.cfg.RequiredDomain) {
		http.Error(w, "sharing restricted to domain "+s.cfg.RequiredDomain, http.StatusBadRequest)
		return
	}

	if err := s.links.AddShare(r.Context(), link.ID, email); err != nil {
		s.logger.Error("share: add share", "link", name, "email", email, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/edit/"+name, http.StatusFound)
}

// handleEditUnshare serves POST /edit/{name}/unshare.
func (s *Server) handleEditUnshare(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	link, id, ok := s.requireEditAccess(w, r, name)
	if !ok {
		return
	}
	_ = id

	email := strings.TrimSpace(r.FormValue("email"))
	if err := s.links.RemoveShare(r.Context(), link.ID, email); err != nil {
		s.logger.Error("unshare: remove share", "link", name, "email", email, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/edit/"+name, http.StatusFound)
}

// handleEditDelete serves POST /edit/{name}/delete.
func (s *Server) handleEditDelete(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	link, id, ok := s.requireEditAccess(w, r, name)
	if !ok {
		return
	}
	_ = id

	if err := s.links.Delete(r.Context(), link.ID); err != nil {
		s.logger.Error("delete: delete link", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/links", http.StatusFound)
}

// linksPageData is the template data for /links and /mylinks.
type linksPageData struct {
	baseData
	Links      []*db.Link
	Query      string
	Page       int
	TotalPages int
}

// handleLinks serves GET /links.
func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logger.Error("links: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	query := r.URL.Query().Get("q")
	page := pageParam(r)

	var linkList []*db.Link
	var total int

	if query != "" {
		linkList, total, err = s.links.Search(r.Context(), query, linksPerPage, (page-1)*linksPerPage)
	} else {
		linkList, total, err = s.links.List(r.Context(), linksPerPage, (page-1)*linksPerPage, db.SortByName, db.SortAsc)
	}
	if err != nil {
		s.logger.Error("links: list", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	s.renderer.Render(w, "links", linksPageData{
		baseData:   base,
		Links:      linkList,
		Query:      query,
		Page:       page,
		TotalPages: totalPages(total, linksPerPage),
	})
}

// handleMyLinks serves GET /mylinks.
func (s *Server) handleMyLinks(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logger.Error("mylinks: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	id := auth.FromContext(r.Context())
	if id == nil {
		http.Redirect(w, r, "/auth/login?rd=/mylinks", http.StatusFound)
		return
	}

	page := pageParam(r)
	linkList, total, err := s.links.ListByOwner(r.Context(), id.Email, linksPerPage, (page-1)*linksPerPage)
	if err != nil {
		s.logger.Error("mylinks: list by owner", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	s.renderer.Render(w, "mylinks", linksPageData{
		baseData:   base,
		Links:      linkList,
		Page:       page,
		TotalPages: totalPages(total, linksPerPage),
	})
}

// handleHelp serves GET /help.
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logger.Error("help: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	s.renderer.Render(w, "help", base)
}

// handleQuickName serves GET /api/quickname and returns an HTML <input> element
// with a freshly generated random name so HTMX can swap it into the form.
func (s *Server) handleQuickName(w http.ResponseWriter, r *http.Request) {
	name, err := links.GenerateQuickName(s.cfg.QuickLinkLength)
	if err != nil {
		s.logger.Error("quickname: generate", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<input class="input" type="text" name="name" value="%s" placeholder="my-link" required>`, name)
}

// requireEditAccess looks up the named link and checks that the current user
// may edit it (owner or shared).  Returns false and writes an HTTP error when
// access is denied.
func (s *Server) requireEditAccess(w http.ResponseWriter, r *http.Request, nameLower string) (*db.Link, *auth.Identity, bool) {
	link, err := s.links.GetByName(r.Context(), nameLower)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return nil, nil, false
		}
		s.logger.Error("edit access: get link", "name", nameLower, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return nil, nil, false
	}
	id := auth.FromContext(r.Context())
	if !canEdit(id, link) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, nil, false
	}
	return link, id, true
}

// canEdit returns true when the identity may edit the given link.
// Admins may edit any link; the owner may always edit their own link.
// Shared users can also edit.
func canEdit(id *auth.Identity, link *db.Link) bool {
	if id == nil {
		return false
	}
	if id.IsAdmin {
		return true
	}
	return strings.EqualFold(id.Email, link.OwnerEmail)
}

// pageParam parses the ?page= query parameter, defaulting to 1.
func pageParam(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		return 1
	}
	return p
}

// totalPages computes the total number of pages for the given item count and
// page size.
func totalPages(total, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	if pages == 0 {
		return 1
	}
	return pages
}
