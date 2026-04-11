package templates_test

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mkende/golink-url-shortener/internal/templates"
)

func newTestRenderer(t *testing.T) *templates.Renderer {
	t.Helper()
	r, err := templates.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("templates.New: %v", err)
	}
	return r
}

func TestNew_ParsesAllTemplates(t *testing.T) {
	// New must succeed; a failure indicates a broken embedded template.
	newTestRenderer(t)
}

func TestRenderer_RenderTo_KnownPage(t *testing.T) {
	r := newTestRenderer(t)
	var buf bytes.Buffer
	// "help" uses base.html; supply the minimal common fields.
	if err := r.RenderTo(&buf, "help", baseData{Title: "Test"}); err != nil {
		t.Fatalf("RenderTo(help): %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestRenderer_RenderTo_UnknownPage(t *testing.T) {
	r := newTestRenderer(t)
	err := r.RenderTo(io.Discard, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown template, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention template name, got: %v", err)
	}
}

// funcMap tests — exercise each template helper via RenderTo.

// baseData mirrors server.baseData for test use.
type baseData struct {
	Title       string
	Identity    any
	CSRFToken   string
	FaviconPath string
	OIDCEnabled bool
	Version     string
}

// linksPageData mirrors server.linksPageData for test use.
type linksPageData struct {
	baseData
	Sort      string
	Dir       string
	Query     string
	Links     any
	Total     int
	Page      int
	TotalPage  int
	TotalPages int
	PageStart int
	PageEnd   int
	OwnedIDs  map[int64]bool
	SharedIDs map[int64]bool
}

func TestSortURL_and_SortIcon(t *testing.T) {
	r := newTestRenderer(t)
	// Exercise sortURL and sortIcon via links.html.
	data := linksPageData{
		baseData: baseData{Title: "Test"},
		Sort:     "name",
		Dir:      "asc",
	}
	var buf bytes.Buffer
	if err := r.RenderTo(&buf, "links", data); err != nil {
		t.Fatalf("RenderTo(links): %v", err)
	}
	body := buf.String()
	// Ascending sort on "name" should produce a descending link for "name".
	if !strings.Contains(body, "sort=name&amp;dir=desc") {
		t.Errorf("expected toggled sort link; body excerpt: %q", body[:min(200, len(body))])
	}
}

func TestNew_NilLogger(t *testing.T) {
	// New(nil) must default to slog.Default() without panicking.
	r, err := templates.New(nil)
	if err != nil {
		t.Fatalf("templates.New(nil): %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil renderer")
	}
}
