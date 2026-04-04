// Package redirect implements the core URL resolution logic for go-link redirects.
package redirect

import (
	"fmt"
	"net/url"
	"strings"
)

// ResolveSimple computes the redirect URL for a simple (non-template) link.
// suffix is the path suffix after the link name (e.g. "/extra/path"), may be
// empty. query is the raw query string from the incoming request (without the
// leading "?"), which is appended to any query string already present in the
// target URL.
func ResolveSimple(target, suffix, query string) (string, error) {
	if suffix == "" && query == "" {
		return target, nil
	}

	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("invalid target URL %q: %w", target, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid target URL %q: missing scheme or host", target)
	}

	if suffix != "" {
		// Avoid double slash when target has a trailing slash and suffix starts with "/".
		base := strings.TrimRight(u.Path, "/")
		u.Path = base + "/" + strings.TrimLeft(suffix, "/")
	}

	if query != "" {
		if u.RawQuery != "" {
			u.RawQuery = u.RawQuery + "&" + query
		} else {
			u.RawQuery = query
		}
	}

	return u.String(), nil
}
