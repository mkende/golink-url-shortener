package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mkende/golink-redirector/internal/db"
	"github.com/mkende/golink-redirector/internal/redirect"
)

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))
	suffix := ""
	if rest := chi.URLParam(r, "*"); rest != "" {
		suffix = "/" + rest
	}

	link, err := s.links.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.logger.Error("db error looking up link", "name", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var targetURL string
	if link.IsAdvanced {
		vars := redirect.TemplateVars{
			Path:  suffix,
			Parts: splitPath(suffix),
			Args:  splitArgs(r.URL.RawQuery),
			UA:    r.UserAgent(),
			Email: "", // auth not implemented yet
		}
		targetURL, err = redirect.ResolveAdvanced(link.Target, vars)
	} else {
		targetURL, err = redirect.ResolveSimple(link.Target, suffix)
	}

	if err != nil {
		s.logger.Error("redirect resolution failed", "name", name, "error", err)
		http.Error(w, "bad redirect target", http.StatusInternalServerError)
		return
	}

	// Async increment (sync for now; phase 9 will make this buffered)
	go func() {
		if err := s.links.IncrementUseCount(context.Background(), link.ID); err != nil {
			s.logger.Error("failed to increment use count", "id", link.ID, "error", err)
		}
	}()

	http.Redirect(w, r, targetURL, http.StatusFound)
}

// splitPath splits a path suffix on "/" returning all parts including empty
// elements for leading slashes.
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
