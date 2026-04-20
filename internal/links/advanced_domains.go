package links

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateDomainPattern returns an error if pattern is not a valid domain
// pattern. Valid patterns are either an exact hostname ("example.com") or a
// leading-wildcard hostname ("*.example.com"). In the wildcard form the star
// must be the very first character and immediately followed by a dot; no other
// star may appear anywhere in the pattern.
func ValidateDomainPattern(pattern string) error {
	if strings.HasPrefix(pattern, "*.") {
		base := pattern[2:]
		if base == "" {
			return fmt.Errorf("domain pattern %q: wildcard must specify a base domain", pattern)
		}
		if strings.ContainsRune(base, '*') {
			return fmt.Errorf("domain pattern %q: only one leading wildcard is allowed", pattern)
		}
		return validateHostname(base, pattern)
	}
	if strings.ContainsRune(pattern, '*') {
		return fmt.Errorf("domain pattern %q: wildcard '*' is only allowed as the leading '*.'", pattern)
	}
	return validateHostname(pattern, pattern)
}

// validateHostname returns an error if s contains characters not valid in a
// DNS hostname (letters, digits, hyphens, dots). pattern is used only in the
// error message.
func validateHostname(s, pattern string) error {
	if s == "" {
		return fmt.Errorf("domain pattern %q: hostname cannot be empty", pattern)
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '.':
		default:
			return fmt.Errorf("domain pattern %q: invalid character %q", pattern, c)
		}
	}
	return nil
}

// MatchesDomainPattern reports whether host matches a single domain pattern.
// Matching is case-insensitive. For wildcard patterns ("*.example.com") the
// subdomain prefix may itself contain dots (allowing multi-level subdomains)
// but must consist only of valid hostname characters (letters, digits, hyphens,
// dots) — slashes or other separators are never permitted, providing a
// defensive check against any exploitation loophole.
func MatchesDomainPattern(host, pattern string) bool {
	host = strings.ToLower(host)
	pattern = strings.ToLower(pattern)

	if !strings.HasPrefix(pattern, "*.") {
		return host == pattern
	}

	// Wildcard: *.example.com → suffix is ".example.com"
	suffix := pattern[1:] // ".example.com"
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	prefix := host[:len(host)-len(suffix)]
	if prefix == "" {
		// The bare base domain itself does not satisfy a wildcard pattern.
		return false
	}
	// Subdomain prefix must contain only valid hostname characters.
	for _, c := range prefix {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// MatchesAnyDomainPattern reports whether host matches at least one of the
// patterns. Returns true when patterns is empty (no restriction).
func MatchesAnyDomainPattern(host string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if MatchesDomainPattern(host, p) {
			return true
		}
	}
	return false
}

// CheckAdvancedLinkDomain validates that the host of resolvedURL is covered by
// at least one of the allowed domain patterns. Returns nil when patterns is
// empty (no restriction). Returns an error when the URL cannot be parsed, when
// the host is empty, or when no pattern matches.
func CheckAdvancedLinkDomain(resolvedURL string, patterns []string) error {
	if len(patterns) == 0 {
		return nil
	}
	u, err := url.Parse(resolvedURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("cannot determine domain of URL %q", resolvedURL)
	}
	host := u.Hostname() // strips any port
	if !MatchesAnyDomainPattern(host, patterns) {
		return fmt.Errorf("domain %q is not in the allowed domains list for advanced links", host)
	}
	return nil
}
