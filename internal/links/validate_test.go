package links_test

import (
	"testing"

	"github.com/mkende/golink-url-shortener/internal/links"
)

func TestValidateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// valid names
		{name: "simple", input: "docs", wantErr: false},
		{name: "with-hyphen", input: "my-link", wantErr: false},
		{name: "with_underscore", input: "my_link", wantErr: false},
		{name: "with.dot", input: "v1.2", wantErr: false},
		{name: "mixed-case", input: "MyLink", wantErr: false},
		{name: "digits", input: "123abc", wantErr: false},

		// invalid names
		{name: "empty", input: "", wantErr: true},
		{name: "slash", input: "foo/bar", wantErr: true},
		{name: "space", input: "my link", wantErr: true},
		{name: "hash", input: "foo#bar", wantErr: true},
		{name: "at-sign", input: "foo@bar", wantErr: true},

		// reserved names (case-insensitive)
		{name: "reserved-new", input: "new", wantErr: true},
		{name: "reserved-edit", input: "edit", wantErr: true},
		{name: "reserved-delete", input: "delete", wantErr: true},
		{name: "reserved-links", input: "links", wantErr: true},
		{name: "reserved-mylinks", input: "mylinks", wantErr: true},
		{name: "reserved-help", input: "help", wantErr: true},
		{name: "reserved-auth", input: "auth", wantErr: true},
		{name: "reserved-api", input: "api", wantErr: true},
		{name: "reserved-healthz", input: "healthz", wantErr: true},
		{name: "reserved-apikeys", input: "apikeys", wantErr: true},
		{name: "reserved-import", input: "import", wantErr: true},
		{name: "reserved-export", input: "export", wantErr: true},
		{name: "reserved-search", input: "search", wantErr: true},
		{name: "reserved-static", input: "static", wantErr: true},
		{name: "reserved-upper", input: "NEW", wantErr: true},
		{name: "reserved-mixed", input: "Help", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := links.ValidateName(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// valid targets
		{name: "http", input: "http://example.com", wantErr: false},
		{name: "https", input: "https://example.com/path?q=1", wantErr: false},
		{name: "https-subdomain", input: "https://sub.example.com", wantErr: false},

		// invalid targets
		{name: "empty", input: "", wantErr: true},
		{name: "javascript", input: "javascript:alert(1)", wantErr: true},
		{name: "javascript-upper", input: "JAVASCRIPT:alert(1)", wantErr: true},
		{name: "data-uri", input: "data:text/html,<h1>hi</h1>", wantErr: true},
		{name: "vbscript", input: "vbscript:msgbox(1)", wantErr: true},
		{name: "relative", input: "/just/a/path", wantErr: true},
		{name: "no-scheme", input: "example.com", wantErr: true},
		{name: "ftp", input: "ftp://example.com", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := links.ValidateTarget(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateTarget(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}
