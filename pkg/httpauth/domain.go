package httpauth

import (
	"net"
	"net/http"
	"net/url"
)

// redirectToCanonical checks whether r is already on the canonical address. If
// not, it writes a 301 redirect and returns true. If no canonical address is
// configured, or the request already matches it, it returns false and leaves w
// untouched.
//
// trustedNets is used to determine whether to trust X-Forwarded-Proto: the
// header is only honoured when the peer IP falls within one of those ranges.
func redirectToCanonical(canonicalScheme, canonicalHost string, trustedNets []*net.IPNet, w http.ResponseWriter, r *http.Request) bool {
	if canonicalScheme == "" || canonicalHost == "" {
		return false
	}

	reqScheme := "http"
	if r.TLS != nil {
		reqScheme = "https"
	} else if len(trustedNets) > 0 {
		if ip := peerIP(r); ip != nil && ipInRanges(ip, trustedNets) {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				reqScheme = proto
			}
		}
	}

	if reqScheme == canonicalScheme && r.Host == canonicalHost {
		return false
	}

	target := &url.URL{
		Scheme:   canonicalScheme,
		Host:     canonicalHost,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
	return true
}
