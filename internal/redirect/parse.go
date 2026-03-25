package redirect

import "strings"

// ParseRequest extracts the link name and suffix from an HTTP request path.
// The path is expected to start with "/" followed by the link name.
// Returns linkName (lowercased) and suffix (everything after the link name).
// Example: "/docs/api/reference" → linkName="docs", suffix="/api/reference"
// Example: "/docs"              → linkName="docs", suffix=""
func ParseRequest(path string) (linkName, suffix string) {
	// Strip the leading slash.
	trimmed := strings.TrimPrefix(path, "/")

	// Split on the first "/".
	idx := strings.Index(trimmed, "/")
	if idx == -1 {
		return strings.ToLower(trimmed), ""
	}

	return strings.ToLower(trimmed[:idx]), trimmed[idx:]
}
