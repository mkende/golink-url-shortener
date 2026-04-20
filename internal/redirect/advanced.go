package redirect

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"text/template"

	"github.com/mkende/golink-url-shortener/internal/links"
)

// TemplateVars holds the template variables available to advanced redirect templates.
type TemplateVars struct {
	// Path is the path suffix after the link name, without a leading slash
	// (e.g. "foo/bar" for go/name/foo/bar).
	Path string
	// Parts is the path suffix split on "/" (e.g. ["foo", "bar"] for "foo/bar").
	Parts []string
	// Args holds query parameters split on "&" (e.g. ["q=hello", "page=1"]).
	Args []string
	// UA is the User-Agent header value from the incoming request.
	UA string
	// Email is the authenticated user's email address; empty if not authenticated.
	Email string
	// Alias is the short link name actually used for this request.  When the
	// request arrives via an alias link this is the alias name; when accessed
	// directly it is the canonical name.  Always available, even for
	// non-alias links.
	Alias string
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

// toMap converts TemplateVars to a map with lowercase keys so that template
// authors can write {{.path}} instead of {{.Path}}.
func (v TemplateVars) toMap() map[string]any {
	return map[string]any{
		"path":  v.Path,
		"parts": v.Parts,
		"args":  v.Args,
		"ua":    v.UA,
		"email": v.Email,
		"alias": v.Alias,
	}
}

// executeTemplate runs the parsed template with data, recovering from panics
// (e.g. out-of-range index) and returning them as errors. Occurrences of
// "<no value>" (produced by undefined map keys) are stripped from the output
// so that missing variables silently become empty strings.
func executeTemplate(t *template.Template, data any) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("template execution panic: %v", r)
		}
	}()
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution error: %w", err)
	}
	return strings.ReplaceAll(buf.String(), "<no value>", ""), nil
}

// ResolveAdvanced executes a Go template target with the given variables.
// Returns the resulting URL string or an error if template execution fails or
// the produced URL uses a disallowed scheme. Undefined variables are treated as
// empty strings to be as lenient as possible.
func ResolveAdvanced(templateStr string, vars TemplateVars) (string, error) {
	t, err := parseTemplate(templateStr)
	if err != nil {
		return "", err
	}

	raw, err := executeTemplate(t, vars.toMap())
	if err != nil {
		return "", err
	}

	result := strings.TrimSpace(raw)
	if err := links.ValidateTarget(result); err != nil {
		return "", fmt.Errorf("advanced template produced an invalid URL: %w", err)
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
	if _, err := executeTemplate(t, TemplateVars{}.toMap()); err != nil {
		return fmt.Errorf("template dry-run error: %w", err)
	}
	return nil
}

// CheckTemplateTargetDomain validates that templateStr begins with a fully
// static domain that matches one of the allowedDomains. "Fully static" means
// the scheme and host contain no template actions: the first "{{" must appear
// only after a path, query, or fragment separator. Any template action at or
// before the host boundary is rejected outright — the domain must be known
// with certainty when domain restrictions are in effect.
// Returns nil when allowedDomains is empty (no restriction).
func CheckTemplateTargetDomain(templateStr string, allowedDomains []string) error {
	if len(allowedDomains) == 0 {
		return nil
	}
	staticPart := strings.TrimSpace(templateStr)
	if i := strings.Index(templateStr, "{{"); i >= 0 {
		staticPart = strings.TrimSpace(templateStr[:i])
		// Require that the cut leaves a URL with a non-empty path, query, or
		// fragment — proof that the "{{" is past the authority. If none of
		// those are present the action is at or inside the host.
		u, err := url.Parse(staticPart)
		if err != nil || u.Hostname() == "" || (u.Path == "" && u.RawQuery == "" && u.Fragment == "") {
			return fmt.Errorf("advanced link target must have a static domain when domain restrictions are configured")
		}
	}
	return links.CheckAdvancedLinkDomain(staticPart, allowedDomains)
}
