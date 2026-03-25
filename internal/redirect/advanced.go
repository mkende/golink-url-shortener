package redirect

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// isSafeURL returns true when the URL produced by an advanced template is safe
// to redirect to.  It must use http or https; javascript:, data:, vbscript:
// and protocol-relative URLs are all rejected.
func isSafeURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// TemplateVars holds the template variables available to advanced redirect templates.
type TemplateVars struct {
	// Path is the full path suffix after the link name (e.g. "/foo/bar").
	Path string
	// Parts is the suffix split on "/" (e.g. ["", "foo", "bar"] for "/foo/bar").
	Parts []string
	// Args holds query parameters split on "&" (e.g. ["q=hello", "page=1"]).
	Args []string
	// UA is the User-Agent header value from the incoming request.
	UA string
	// Email is the authenticated user's email address; empty if not authenticated.
	Email string
}

// templateFuncs returns the custom template function map used for advanced links.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// match returns true when pattern matches s (partial match via MatchString).
		// Returns false on a bad pattern rather than panicking.
		"match": func(pattern, s string) bool {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false
			}
			return re.MatchString(s)
		},
		// extract returns the first submatch group of pattern in s.
		// Returns an empty string if there is no match or the pattern is invalid.
		"extract": func(pattern, s string) string {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return ""
			}
			sub := re.FindStringSubmatch(s)
			if len(sub) < 2 {
				return ""
			}
			return sub[1]
		},
		// replace performs a regexp ReplaceAllString on s. Returns s unchanged on
		// a bad pattern.
		"replace": func(pattern, repl, s string) string {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return s
			}
			return re.ReplaceAllString(s, repl)
		},
	}
}

// parseTemplate parses templateStr with all custom functions registered.
func parseTemplate(templateStr string) (*template.Template, error) {
	t, err := template.New("redirect").Funcs(templateFuncs()).Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}
	return t, nil
}

// ResolveAdvanced executes a Go template target with the given variables.
// Returns the resulting URL string or an error if template execution fails or
// the produced URL uses a disallowed scheme.
func ResolveAdvanced(templateStr string, vars TemplateVars) (string, error) {
	t, err := parseTemplate(templateStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}

	result := strings.TrimSpace(buf.String())
	if !isSafeURL(result) {
		return "", fmt.Errorf("advanced template produced a disallowed URL scheme: %q", result)
	}
	return result, nil
}

// ValidateTemplate checks that a template string parses and is safe to use.
// Returns an error describing the problem, or nil if valid.
func ValidateTemplate(templateStr string) error {
	t, err := parseTemplate(templateStr)
	if err != nil {
		return err
	}

	// Dry-run with zero-value vars to catch obvious runtime errors.
	var buf bytes.Buffer
	if err := t.Execute(&buf, TemplateVars{}); err != nil {
		return fmt.Errorf("template dry-run error: %w", err)
	}
	return nil
}
