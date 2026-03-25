// Package redirect implements the core URL resolution logic for go-link redirects.
package redirect

import (
	"fmt"
	"net/url"
	"strings"
)

// ResolveSimple computes the redirect URL for a simple (non-template) link.
// suffix is the path suffix after the link name (e.g. "/extra/path"), may be empty.
// fragment is the URL fragment (not available server-side but included for completeness).
func ResolveSimple(target, suffix string) (string, error) {
	if suffix == "" {
		return target, nil
	}

	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("invalid target URL %q: %w", target, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid target URL %q: missing scheme or host", target)
	}

	// Avoid double slash when target has a trailing slash and suffix starts with "/".
	base := strings.TrimRight(u.Path, "/")
	u.Path = base + "/" + strings.TrimLeft(suffix, "/")

	return u.String(), nil
}
