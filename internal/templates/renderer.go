// Package templates provides HTML template rendering for the golink UI.
package templates

import (
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strings"

	webtemplates "github.com/mkende/golink-url-shortener/web/templates"
)

var funcMap = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	// hasKey reports whether the map contains the given key (for ownership checks).
	"hasKey": func(m map[int64]bool, key int64) bool {
		if m == nil {
			return false
		}
		return m[key]
	},
	// sortURL builds a sort link preserving the current query and page.
	"sortURL": func(basePath, currentSort, currentDir, column string) string {
		dir := "asc"
		if currentSort == column && currentDir == "asc" {
			dir = "desc"
		}
		return basePath + "?sort=" + column + "&dir=" + dir
	},
	// sortIcon returns an arrow indicator for the active sort column.
	"sortIcon": func(currentSort, currentDir, column string) string {
		if currentSort != column {
			return ""
		}
		if currentDir == "asc" {
			return " \u25B2"
		}
		return " \u25BC"
	},
}

// Renderer holds parsed HTML templates ready for execution.
type Renderer struct {
	templates map[string]*template.Template
	logger    *slog.Logger
}

// New parses all page templates and returns a ready Renderer.
// Pages are discovered automatically from the embedded FS: every .html file
// that is not base.html and not listed in partials is treated as a page.
// If logger is nil, slog.Default() is used.
// Returns an error if any template fails to parse.
func New(logger *slog.Logger) (*Renderer, error) {
	if logger == nil {
		logger = slog.Default()
	}
	partials := []string{"link_table.html", "pagination.html"}

	entries, err := webtemplates.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	nonPages := map[string]bool{"base.html": true}
	for _, p := range partials {
		nonPages[p] = true
	}
	var pages []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") && !nonPages[e.Name()] {
			pages = append(pages, strings.TrimSuffix(e.Name(), ".html"))
		}
	}

	baseData, err := webtemplates.FS.ReadFile("base.html")
	if err != nil {
		return nil, fmt.Errorf("read base template: %w", err)
	}

	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t := template.New("base.html").Funcs(funcMap)
		if _, err := t.Parse(string(baseData)); err != nil {
			return nil, fmt.Errorf("parse base template: %w", err)
		}
		for _, partial := range partials {
			data, err := webtemplates.FS.ReadFile(partial)
			if err != nil {
				return nil, fmt.Errorf("read partial %s: %w", partial, err)
			}
			if _, err := t.New(partial).Parse(string(data)); err != nil {
				return nil, fmt.Errorf("parse partial %s: %w", partial, err)
			}
		}
		pageData, err := webtemplates.FS.ReadFile(page + ".html")
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", page, err)
		}
		if _, err := t.New(page + ".html").Parse(string(pageData)); err != nil {
			return nil, fmt.Errorf("parse template %s: %w", page, err)
		}
		templates[page] = t
	}
	return &Renderer{templates: templates, logger: logger}, nil
}

// Render writes the named page template to w with the given data using HTTP
// 200 OK. On template-not-found it writes a 500 response.
func (r *Renderer) Render(w http.ResponseWriter, name string, data any) {
	r.RenderStatus(w, http.StatusOK, name, data)
}

// RenderStatus writes the named page template to w with the given HTTP status
// code and data. On template-not-found it writes a 500 response.
func (r *Renderer) RenderStatus(w http.ResponseWriter, status int, name string, data any) {
	t, ok := r.templates[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.Execute(w, data); err != nil {
		// Headers are already sent; we can't change the status code, but we can log.
		r.logger.Error("template execution failed", "template", name, "error", err)
	}
}

// RenderTo renders the named template to w (for testing / non-HTTP use).
func (r *Renderer) RenderTo(w io.Writer, name string, data any) error {
	t, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template not found: %s", name)
	}
	return t.Execute(w, data)
}
