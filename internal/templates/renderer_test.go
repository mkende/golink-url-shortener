package templates_test

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mkende/golink-url-shortener/internal/db"
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
	Title              string
	Identity           any
	CSRFToken          string
	FaviconPath        string
	OIDCEnabled        bool
	Version            string
	AllowAdvancedLinks bool
}

// linksPageData mirrors server.linksPageData for test use.
type linksPageData struct {
	baseData
	Sort       string
	Dir        string
	Query      string
	Links      any
	Total      int
	Page       int
	TotalPage  int
	TotalPages int
	PageStart  int
	PageEnd    int
	OwnedIDs   map[int64]bool
	SharedIDs  map[int64]bool
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

// detailsPageData mirrors server.detailsPageData for template rendering tests.
type detailsPageData struct {
	baseData
	Link          *db.Link
	CanEdit       bool
	Aliases       []*db.Link
	CanonicalLink *db.Link
	Shares        []string
	DefaultDomain string
	Error         string
	Success       string
}

func simpleLink() *db.Link {
	return &db.Link{
		Name:      "mylink",
		NameLower: "mylink",
		Target:    "https://example.com",
		LinkType:  db.LinkTypeSimple,
	}
}

// TestDetailsTemplate_ShareForm verifies that the share-with input is rendered
// as type="text" (not type="email") and carries the required HTMX attributes.
func TestDetailsTemplate_ShareForm(t *testing.T) {
	r := newTestRenderer(t)
	data := detailsPageData{
		baseData: baseData{Title: "Test"},
		Link:     simpleLink(),
		CanEdit:  true,
	}
	var buf bytes.Buffer
	if err := r.RenderTo(&buf, "details", data); err != nil {
		t.Fatalf("RenderTo(details): %v", err)
	}
	body := buf.String()

	if strings.Contains(body, `type="email"`) {
		t.Error(`share input must use type="text", not type="email"`)
	}
	for _, attr := range []string{
		`hx-get="/api/users/search"`,
		`hx-trigger="input changed delay:300ms"`,
		`hx-target="#known-users"`,
		`hx-params="email"`,
	} {
		if !strings.Contains(body, attr) {
			t.Errorf("share input missing HTMX attribute: %s", attr)
		}
	}
}

// TestDetailsTemplate_SharePlaceholder verifies that the placeholder text
// reflects the DefaultDomain setting.
func TestDetailsTemplate_SharePlaceholder(t *testing.T) {
	r := newTestRenderer(t)

	cases := []struct {
		name            string
		defaultDomain   string
		wantPlaceholder string
	}{
		{
			name:            "no default domain",
			defaultDomain:   "",
			wantPlaceholder: "user@example.com",
		},
		{
			name:            "with default domain",
			defaultDomain:   "corp.example.com",
			wantPlaceholder: "user or user@corp.example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := detailsPageData{
				baseData:      baseData{Title: "Test"},
				Link:          simpleLink(),
				CanEdit:       true,
				DefaultDomain: tc.defaultDomain,
			}
			var buf bytes.Buffer
			if err := r.RenderTo(&buf, "details", data); err != nil {
				t.Fatalf("RenderTo(details): %v", err)
			}
			if !strings.Contains(buf.String(), tc.wantPlaceholder) {
				t.Errorf("expected placeholder %q in rendered page", tc.wantPlaceholder)
			}
		})
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
