package links_test

import (
	"testing"

	"github.com/mkende/golink-url-shortener/internal/links"
)

func TestValidateDomainPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		wantErr bool
	}{
		{"example.com", false},
		{"sub.example.com", false},
		{"*.example.com", false},
		{"*.sub.example.com", false},
		{"go.corp.example.com", false},
		// Invalid patterns.
		{"", true},
		{"*", true},
		{"*.", true},
		{"*.example.*", true},
		{"example.*", true},
		{"*example.com", true},  // star not followed by dot
		{"exam_ple.com", true},  // underscore not allowed
		{"example.com/path", true},
		{"*.*.example.com", true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.pattern, func(t *testing.T) {
			t.Parallel()
			err := links.ValidateDomainPattern(tc.pattern)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateDomainPattern(%q) error = %v, wantErr %v", tc.pattern, err, tc.wantErr)
			}
		})
	}
}

func TestMatchesDomainPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host    string
		pattern string
		want    bool
	}{
		{"example.com", "example.com", true},
		{"EXAMPLE.COM", "example.com", true},           // case-insensitive
		{"example.com", "other.com", false},
		{"sub.example.com", "example.com", false},       // sub-domain doesn't match exact
		{"sub.example.com", "*.example.com", true},
		{"deep.sub.example.com", "*.example.com", true}, // multi-level sub-domain
		{"example.com", "*.example.com", false},         // bare domain fails wildcard
		{"sub.other.com", "*.example.com", false},
		{"sub-name.example.com", "*.example.com", true},
		{"a.b.c.example.com", "*.example.com", true},    // deep nesting
		// Defensive: unusual but possible host strings.
		{"notexample.com", "example.com", false},
		{"xexample.com", "*.example.com", false},        // doesn't end with .example.com
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.host+"__"+tc.pattern, func(t *testing.T) {
			t.Parallel()
			got := links.MatchesDomainPattern(tc.host, tc.pattern)
			if got != tc.want {
				t.Errorf("MatchesDomainPattern(%q, %q) = %v, want %v", tc.host, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestMatchesAnyDomainPattern(t *testing.T) {
	t.Parallel()

	t.Run("empty patterns always matches", func(t *testing.T) {
		t.Parallel()
		if !links.MatchesAnyDomainPattern("evil.com", nil) {
			t.Error("expected true for empty patterns")
		}
	})

	t.Run("matches first pattern", func(t *testing.T) {
		t.Parallel()
		if !links.MatchesAnyDomainPattern("example.com", []string{"example.com", "other.com"}) {
			t.Error("expected match")
		}
	})

	t.Run("matches second pattern", func(t *testing.T) {
		t.Parallel()
		if !links.MatchesAnyDomainPattern("sub.other.com", []string{"example.com", "*.other.com"}) {
			t.Error("expected match")
		}
	})

	t.Run("no match", func(t *testing.T) {
		t.Parallel()
		if links.MatchesAnyDomainPattern("evil.com", []string{"example.com", "*.other.com"}) {
			t.Error("expected no match")
		}
	})
}

func TestCheckAdvancedLinkDomain(t *testing.T) {
	t.Parallel()

	patterns := []string{"example.com", "*.internal.com"}

	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com/path", false},
		{"https://example.com:8080/path", false},     // port stripped
		{"https://sub.internal.com/path", false},
		{"https://deep.sub.internal.com/path", false},
		{"http://example.com/path", false},            // http is also fine
		{"https://evil.com/path", true},
		{"https://notexample.com/path", true},
		{"://invalid", true},
		{"https:///no-host", true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			err := links.CheckAdvancedLinkDomain(tc.url, patterns)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckAdvancedLinkDomain(%q) error = %v, wantErr %v", tc.url, err, tc.wantErr)
			}
		})
	}

	t.Run("empty patterns skips check", func(t *testing.T) {
		t.Parallel()
		if err := links.CheckAdvancedLinkDomain("https://anywhere.com/path", nil); err != nil {
			t.Errorf("expected nil for empty patterns, got %v", err)
		}
	})
}
