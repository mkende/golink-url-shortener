package server

import (
	"context"
	"errors"
	"fmt"
	"html"
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

	linkType := db.LinkTypeSimple
	if form.IsAdvanced {
		linkType = db.LinkTypeAdvanced
	}

	_, err = s.links.Create(r.Context(), form.Name, form.Target, id.Email, linkType, "", form.RequireAuth)
	if err != nil {
		s.logger.Error("new: create link", "name", form.Name, "error", err)
		renderError("Could not create link. A link with that name may already exist.")
		return
	}

	http.Redirect(w, r, "/details/"+form.Name, http.StatusFound)
}

// detailsPageData is the template data for /details/{name}.
type detailsPageData struct {
	baseData
	Link          *db.Link
	CanEdit       bool
	Aliases       []*db.Link
	CanonicalLink *db.Link // non-nil only for alias links
	Shares        []string
	KnownUsers    []*db.User
	Error         string
	Success       string
}

// handleDetails serves GET /details/{name} and POST /details/{name}.
// Any authenticated user may view; only owners, shared users, and admins may edit.
func (s *Server) handleDetails(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))

	base, err := s.newBaseData(w, r)
	if err != nil {
		s.logger.Error("details: baseData", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Authentication required to view the details page.
	if base.Identity == nil {
		http.Redirect(w, r, "/auth/login?rd=/details/"+name, http.StatusFound)
		return
	}

	link, err := s.links.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.logger.Error("details: get link", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Determine whether the current user may edit this link.
	canEdit := canEditBasic(base.Identity, link)
	if !canEdit {
		sharedWith, checkErr := s.isSharedWith(r.Context(), link.ID, base.Identity)
		if checkErr != nil {
			s.logger.Error("details: check shares", "name", name, "error", checkErr)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		canEdit = sharedWith
	}

	if r.Method == http.MethodGet {
		data, err := s.buildDetailsPageData(r, base, link, canEdit)
		if err != nil {
			s.logger.Error("details: build page data", "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		s.renderer.Render(w, "details", data)
		return
	}

	// POST — save changes; requires edit access.
	if !canEdit {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	renderError := func(msg string) {
		data, buildErr := s.buildDetailsPageData(r, base, link, canEdit)
		if buildErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		data.Error = msg
		s.renderer.Render(w, "details", data)
	}

	linkTypeStr := r.FormValue("link_type")
	requireAuth := r.FormValue("require_auth") == "on"

	switch linkTypeStr {
	case "alias":
		aliasTargetRaw := strings.TrimSpace(r.FormValue("alias_target"))
		if aliasTargetRaw == "" {
			renderError("Alias target cannot be empty.")
			return
		}
		aliasTargetLower, resolveErr := s.resolveAliasTarget(r, strings.ToLower(aliasTargetRaw), link.NameLower)
		if resolveErr != nil {
			renderError(resolveErr.Error())
			return
		}
		updated, setErr := s.links.SetAlias(r.Context(), link.ID, link.Name, aliasTargetLower, requireAuth, s.cfg.MaxAliasesPerLink)
		if setErr != nil {
			if errors.Is(setErr, db.ErrAliasLimitExceeded) {
				renderError(fmt.Sprintf("Alias limit reached: a link may have at most %d aliases.", s.cfg.MaxAliasesPerLink))
				return
			}
			s.logger.Error("details: set alias", "id", link.ID, "error", setErr)
			renderError("Could not save changes. Please try again.")
			return
		}
		data, buildErr := s.buildDetailsPageData(r, base, updated, canEdit)
		if buildErr != nil {
			s.logger.Error("details: build page data after set alias", "error", buildErr)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		data.Success = "Link saved."
		s.renderer.Render(w, "details", data)

	default:
		// Simple or advanced.
		lt := db.LinkTypeSimple
		if linkTypeStr == "advanced" {
			lt = db.LinkTypeAdvanced
		}
		target := strings.TrimSpace(r.FormValue("target"))
		if err := links.ValidateTarget(target); err != nil {
			renderError(err.Error())
			return
		}
		updated, updateErr := s.links.Update(r.Context(), link.ID, link.Name, target, lt, requireAuth)
		if updateErr != nil {
			s.logger.Error("details: update link", "id", link.ID, "error", updateErr)
			renderError("Could not save changes. Please try again.")
			return
		}
		data, buildErr := s.buildDetailsPageData(r, base, updated, canEdit)
		if buildErr != nil {
			s.logger.Error("details: build page data after update", "error", buildErr)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		data.Success = "Link saved."
		s.renderer.Render(w, "details", data)
	}
}

// buildDetailsPageData assembles detailsPageData for the details page.
func (s *Server) buildDetailsPageData(r *http.Request, base baseData, link *db.Link, canEdit bool) (detailsPageData, error) {
	data := detailsPageData{
		baseData: base,
		Link:     link,
		CanEdit:  canEdit,
	}

	if link.IsAlias() {
		// Load the canonical link so the template can display its target.
		canonical, err := s.links.GetByName(r.Context(), link.AliasTarget)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			return detailsPageData{}, fmt.Errorf("get canonical link: %w", err)
		}
		data.CanonicalLink = canonical // may be nil if canonical was deleted
	} else {
		// Load aliases of this link.
		aliases, err := s.links.GetAliases(r.Context(), link.NameLower)
		if err != nil {
			return detailsPageData{}, fmt.Errorf("get aliases: %w", err)
		}
		data.Aliases = aliases
	}

	if canEdit {
		shares, err := s.links.GetShares(r.Context(), link.ID)
		if err != nil {
			return detailsPageData{}, fmt.Errorf("get shares: %w", err)
		}
		data.Shares = shares

		knownUsers, err := s.users.List(r.Context(), 200, 0)
		if err != nil {
			// Non-fatal: autocomplete just won't be populated.
			s.logger.Error("details: list users for autocomplete", "error", err)
		}
		data.KnownUsers = knownUsers
	}

	return data, nil
}

// handleDetailsShare serves POST /details/{name}/share.
func (s *Server) handleDetailsShare(w http.ResponseWriter, r *http.Request) {
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
		http.Redirect(w, r, "/details/"+name, http.StatusFound)
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

	http.Redirect(w, r, "/details/"+name, http.StatusFound)
}

// handleDetailsUnshare serves POST /details/{name}/unshare.
func (s *Server) handleDetailsUnshare(w http.ResponseWriter, r *http.Request) {
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

	http.Redirect(w, r, "/details/"+name, http.StatusFound)
}

// handleDetailsDelete serves POST /details/{name}/delete.
func (s *Server) handleDetailsDelete(w http.ResponseWriter, r *http.Request) {
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

// handleCreateAlias serves POST /details/{name}/alias.
// Any authenticated user may create an alias for any link.
func (s *Server) handleCreateAlias(w http.ResponseWriter, r *http.Request) {
	canonicalName := strings.ToLower(chi.URLParam(r, "name"))

	if !validateCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}

	id := auth.FromContext(r.Context())
	if id == nil {
		http.Redirect(w, r, "/auth/login?rd=/details/"+canonicalName, http.StatusFound)
		return
	}

	// Verify the canonical link exists and is not itself an alias.
	canonical, err := s.links.GetByName(r.Context(), canonicalName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.logger.Error("create alias: get canonical", "name", canonicalName, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if canonical.IsAlias() {
		http.Error(w, "cannot create an alias of an alias", http.StatusBadRequest)
		return
	}

	aliasName := strings.TrimSpace(r.FormValue("alias_name"))
	if err := links.ValidateName(aliasName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check alias limit before inserting (slight race, but acceptable).
	count, err := s.links.CountAliases(r.Context(), canonicalName)
	if err != nil {
		s.logger.Error("create alias: count aliases", "name", canonicalName, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if count >= s.cfg.MaxAliasesPerLink {
		http.Error(w, fmt.Sprintf("Alias limit reached: a link may have at most %d aliases.", s.cfg.MaxAliasesPerLink), http.StatusUnprocessableEntity)
		return
	}

	_, err = s.links.Create(r.Context(), aliasName, "", id.Email, db.LinkTypeAlias, canonicalName, false)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			http.Error(w, "A link with that name already exists.", http.StatusConflict)
			return
		}
		s.logger.Error("create alias: create", "alias", aliasName, "canonical", canonicalName, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/details/"+aliasName, http.StatusFound)
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
	fmt.Fprintf(w, `<input class="input" type="text" name="name" value="%s" placeholder="my-link" required>`, html.EscapeString(name))
}

// requireEditAccess looks up the named link and checks that the current user
// may edit it (owner, shared user, or admin).  Returns false and writes an HTTP
// error when access is denied.
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
	if !canEditBasic(id, link) {
		// Check whether the user is in the share list.
		ok, checkErr := s.isSharedWith(r.Context(), link.ID, id)
		if checkErr != nil {
			s.logger.Error("edit access: check shares", "link", nameLower, "error", checkErr)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return nil, nil, false
		}
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return nil, nil, false
		}
	}
	return link, id, true
}

// canEditBasic returns true when the identity may edit the link based solely on
// owner and admin status, without consulting the share list.
func canEditBasic(id *auth.Identity, link *db.Link) bool {
	if id == nil {
		return false
	}
	if id.IsAdmin {
		return true
	}
	return strings.EqualFold(id.Email, link.OwnerEmail)
}

// canEdit returns true when the identity may edit the given link based on
// owner and admin status.  Callers that need share-list checks should use
// requireEditAccess instead.
func canEdit(id *auth.Identity, link *db.Link) bool {
	return canEditBasic(id, link)
}

// isSharedWith reports whether the given identity's email is in the share list
// for the link.  Returns false (not an error) when id is nil.
func (s *Server) isSharedWith(ctx context.Context, linkID int64, id *auth.Identity) (bool, error) {
	if id == nil {
		return false, nil
	}
	shares, err := s.links.GetShares(ctx, linkID)
	if err != nil {
		return false, err
	}
	for _, email := range shares {
		if strings.EqualFold(email, id.Email) {
			return true, nil
		}
	}
	return false, nil
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
