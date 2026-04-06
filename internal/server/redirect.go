package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mkende/golink-url-shortener/internal/auth"
	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/internal/redirect"
)

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))
	suffix := chi.URLParam(r, "*") // no leading "/"

	link, err := s.links.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			s.renderNotFound(w, r, name)
			return
		}
		s.logr(r.Context()).Error("db error looking up link", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Per-link auth enforcement
	id := auth.FromContext(r.Context())
	if link.RequireAuth && id == nil {
		loginURL := "/auth/login?rd=" + url.QueryEscape(r.URL.RequestURI())
		if s.cfg.OIDC.Enabled && s.cfg.CanonicalDomain != "" {
			loginURL = "https://" + s.cfg.CanonicalDomain + loginURL
		}
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}

	// Global require_auth_for_redirects enforcement
	if s.cfg.RequireAuthForRedirects && id == nil {
		loginURL := "/auth/login?rd=" + url.QueryEscape(r.URL.RequestURI())
		if s.cfg.OIDC.Enabled && s.cfg.CanonicalDomain != "" {
			loginURL = "https://" + s.cfg.CanonicalDomain + loginURL
		}
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}

	email := ""
	if id != nil {
		email = id.Email
	}

	// For alias links, resolve to the canonical link and apply its redirect
	// logic.  The alias name is passed as the Alias template variable.
	aliasName := ""
	if link.IsAlias() {
		aliasName = name
		canonical, err := s.links.GetByName(r.Context(), link.AliasTarget)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				s.logr(r.Context()).Error("alias target not found", "alias", name, "target", link.AliasTarget)
				http.Error(w, "alias target not found", http.StatusNotFound)
				return
			}
			s.logr(r.Context()).Error("db error resolving alias target", "alias", name, "target", link.AliasTarget, "error", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// Increment stats for the alias link, not the canonical.
		s.useCounter.Increment(link.ID)
		// Use the canonical link for redirect resolution.
		link = canonical
	}

	var targetURL string
	if link.IsAdvanced() {
		vars := redirect.TemplateVars{
			Path:  suffix,
			Parts: splitPath(suffix),
			Args:  splitArgs(r.URL.RawQuery),
			UA:    r.UserAgent(),
			Email: email,
			Alias: aliasName,
		}
		targetURL, err = redirect.ResolveAdvanced(link.Target, vars)
	} else {
		targetURL, err = redirect.ResolveSimple(link.Target, suffix, r.URL.RawQuery)
	}

	if err != nil {
		s.logr(r.Context()).Error("redirect resolution failed", "name", name, "error", err)
		http.Error(w, "bad redirect target", http.StatusInternalServerError)
		return
	}

	// For non-alias links, record the use count here.  Alias links already
	// incremented above.
	if aliasName == "" {
		s.useCounter.Increment(link.ID)
	}

	http.Redirect(w, r, targetURL, http.StatusFound)
}

// splitPath splits a path suffix on "/" returning all non-empty path segments.
func splitPath(suffix string) []string {
	if suffix == "" {
		return nil
	}
	return strings.Split(suffix, "/")
}

// splitArgs splits a raw query string on "&" returning individual key=value pairs.
func splitArgs(query string) []string {
	if query == "" {
		return nil
	}
	return strings.Split(query, "&")
}
