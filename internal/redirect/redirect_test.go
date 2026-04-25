package redirect

import (
	"testing"
)

// ----------------------------------------------------------------------------
// ResolveSimple
// ----------------------------------------------------------------------------

func TestResolveSimple(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		suffix  string
		query   string
		want    string
		wantErr bool
	}{
		{
			name:   "no suffix no query",
			target: "https://example.com/docs",
			want:   "https://example.com/docs",
		},
		{
			name:   "with path suffix",
			target: "https://example.com/docs",
			suffix: "/api/reference",
			want:   "https://example.com/docs/api/reference",
		},
		{
			name:   "target trailing slash with suffix",
			target: "https://example.com/docs/",
			suffix: "/api",
			want:   "https://example.com/docs/api",
		},
		{
			name:   "simple root target with suffix",
			target: "https://example.com",
			suffix: "/extra",
			want:   "https://example.com/extra",
		},
		{
			name:   "query only no suffix",
			target: "https://example.com",
			query:  "bar=1",
			want:   "https://example.com?bar=1",
		},
		{
			name:   "suffix and query",
			target: "https://example.com",
			suffix: "/path",
			query:  "baz=2",
			want:   "https://example.com/path?baz=2",
		},
		{
			name:   "query appended to target with existing query",
			target: "https://example.com/docs?existing=1",
			query:  "extra=2",
			want:   "https://example.com/docs?existing=1&extra=2",
		},
		{
			name:   "suffix and query with target query",
			target: "https://example.com/docs?a=1",
			suffix: "/sub",
			query:  "b=2",
			want:   "https://example.com/docs/sub?a=1&b=2",
		},
		{
			name:    "invalid target URL",
			target:  "://bad",
			suffix:  "/x",
			wantErr: true,
		},
		{
			name:    "target missing host",
			target:  "/relative",
			suffix:  "/x",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveSimple(tc.target, tc.suffix, tc.query)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ResolveSimple(%q, %q, %q) error = %v, wantErr %v", tc.target, tc.suffix, tc.query, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("ResolveSimple(%q, %q, %q) = %q, want %q", tc.target, tc.suffix, tc.query, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// ResolveAdvanced
// ----------------------------------------------------------------------------

func TestResolveAdvanced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		templateStr string
		vars        TemplateVars
		want        string
		wantErr     bool
	}{
		{
			name:        "literal template",
			templateStr: "https://example.com/docs",
			vars:        TemplateVars{},
			want:        "https://example.com/docs",
		},
		{
			name:        "using path variable",
			templateStr: "https://example.com/{{.path}}",
			vars:        TemplateVars{Path: "foo/bar"},
			want:        "https://example.com/foo/bar",
		},
		{
			name:        "using parts variable",
			templateStr: `https://example.com/{{index .parts 0}}`,
			vars:        TemplateVars{Parts: []string{"section", "page"}},
			want:        "https://example.com/section",
		},
		{
			name:        "using match function – true branch",
			templateStr: `{{if match "foo" .path}}https://foo.example.com{{else}}https://other.example.com{{end}}`,
			vars:        TemplateVars{Path: "foo/bar"},
			want:        "https://foo.example.com",
		},
		{
			name:        "using match function – false branch",
			templateStr: `{{if match "foo" .path}}https://foo.example.com{{else}}https://other.example.com{{end}}`,
			vars:        TemplateVars{Path: "baz"},
			want:        "https://other.example.com",
		},
		{
			name:        "using extract function",
			templateStr: `https://example.com/search?q={{extract "q=([^&]+)" .path}}`,
			vars:        TemplateVars{Path: "q=hello&page=1"},
			want:        "https://example.com/search?q=hello",
		},
		{
			name:        "using replace function",
			templateStr: `https://example.com/{{replace "/" "-" .path}}`,
			vars:        TemplateVars{Path: "foo/bar"},
			want:        "https://example.com/foo-bar",
		},
		{
			name:        "whitespace trimmed from output",
			templateStr: "  https://example.com  ",
			vars:        TemplateVars{},
			want:        "https://example.com",
		},
		{
			name:        "invalid template syntax",
			templateStr: "{{.Unclosed",
			wantErr:     true,
		},
		{
			name:        "template execution error",
			templateStr: `{{index .parts 99}}`,
			vars:        TemplateVars{},
			wantErr:     true,
		},
		{
			name:        "javascript scheme rejected",
			templateStr: "javascript:alert(1)",
			vars:        TemplateVars{},
			wantErr:     true,
		},
		{
			name:        "data scheme rejected",
			templateStr: "data:text/html,<h1>hi</h1>",
			vars:        TemplateVars{},
			wantErr:     true,
		},
		{
			name:        "protocol-relative URL rejected",
			templateStr: "//evil.com/path",
			vars:        TemplateVars{},
			wantErr:     true,
		},
		{
			name:        "relative path rejected",
			templateStr: "/relative/path",
			vars:        TemplateVars{},
			wantErr:     true,
		},
		{
			name:        "undefined variable evaluates to empty string",
			templateStr: "https://example.com/{{.nonexistent}}page",
			vars:        TemplateVars{},
			want:        "https://example.com/page",
		},
		{
			name:        "alias variable – direct access uses canonical name",
			templateStr: "https://example.com/{{.alias}}",
			vars:        TemplateVars{Alias: "mylink"},
			want:        "https://example.com/mylink",
		},
		{
			name:        "alias variable – via alias uses alias name",
			templateStr: "https://example.com/{{.alias}}",
			vars:        TemplateVars{Alias: "myalias"},
			want:        "https://example.com/myalias",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveAdvanced(tc.templateStr, tc.vars)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ResolveAdvanced(%q) error = %v, wantErr %v", tc.templateStr, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("ResolveAdvanced(%q) = %q, want %q", tc.templateStr, got, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// ParseRequest
// ----------------------------------------------------------------------------

func TestParseRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		path         string
		wantLinkName string
		wantSuffix   string
	}{
		{
			name:         "name only",
			path:         "/docs",
			wantLinkName: "docs",
			wantSuffix:   "",
		},
		{
			name:         "name with one level suffix",
			path:         "/docs/api",
			wantLinkName: "docs",
			wantSuffix:   "/api",
		},
		{
			name:         "name with multi-level suffix",
			path:         "/docs/api/reference",
			wantLinkName: "docs",
			wantSuffix:   "/api/reference",
		},
		{
			name:         "uppercase name lowercased",
			path:         "/DOCS/api",
			wantLinkName: "docs",
			wantSuffix:   "/api",
		},
		{
			name:         "root slash only",
			path:         "/",
			wantLinkName: "",
			wantSuffix:   "",
		},
		{
			name:         "mixed case name only",
			path:         "/MyLink",
			wantLinkName: "mylink",
			wantSuffix:   "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotName, gotSuffix := ParseRequest(tc.path)
			if gotName != tc.wantLinkName {
				t.Errorf("ParseRequest(%q) linkName = %q, want %q", tc.path, gotName, tc.wantLinkName)
			}
			if gotSuffix != tc.wantSuffix {
				t.Errorf("ParseRequest(%q) suffix = %q, want %q", tc.path, gotSuffix, tc.wantSuffix)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// CheckTemplateTargetDomain
// ----------------------------------------------------------------------------

func TestCheckTemplateTargetDomain(t *testing.T) {
	t.Parallel()

	allowed := []string{"example.com", "*.corp.com"}

	tests := []struct {
		name        string
		templateStr string
		wantErr     bool
	}{
		// Static domain in allowed list.
		{"static allowed", "https://example.com/docs", false},
		{"static allowed with path template", "https://example.com/{{.path}}", false},
		{"static allowed with query template", "https://example.com?q={{.path}}", false},
		{"static wildcard subdomain", "https://sub.corp.com/{{.path}}", false},
		// Static domain not in allowed list.
		{"static disallowed", "https://evil.com/docs", true},
		{"static disallowed with path template", "https://evil.com/{{.path}}", true},
		// Any template action at or before the host boundary — rejected.
		{"fully dynamic", "{{if .path}}https://example.com{{end}}", true},
		{"dynamic host prefix", "https://{{.sub}}.example.com/foo", true},
		{"dynamic host suffix", "https://example.com{{.var}}/foo", true},
		// No restriction.
		{"empty allowed list", "https://evil.com/", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			domains := allowed
			if tc.name == "empty allowed list" {
				domains = nil
			}
			err := CheckTemplateTargetDomain(tc.templateStr, domains)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckTemplateTargetDomain(%q) error = %v, wantErr %v", tc.templateStr, err, tc.wantErr)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// ValidateTemplate
// ----------------------------------------------------------------------------

func TestValidateTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		templateStr string
		wantErr     bool
	}{
		{
			name:        "valid literal template",
			templateStr: "https://example.com/docs",
			wantErr:     false,
		},
		{
			name:        "valid template with variables",
			templateStr: "https://example.com/{{.path}}",
			wantErr:     false,
		},
		{
			name:        "valid template with custom functions",
			templateStr: `{{if match "foo" .path}}https://foo.com{{else}}https://bar.com{{end}}`,
			wantErr:     false,
		},
		{
			name:        "invalid syntax – unclosed action",
			templateStr: "{{.Unclosed",
			wantErr:     true,
		},
		{
			name:        "unknown function",
			templateStr: `{{unknownfunc .Path}}`,
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTemplate(tc.templateStr)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateTemplate(%q) error = %v, wantErr %v", tc.templateStr, err, tc.wantErr)
			}
		})
	}
}
