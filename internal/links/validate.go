package links

import (
	"fmt"
	"regexp"
	"strings"
)

// reservedNames is the set of endpoint path segments that cannot be used as
// link names because they conflict with server routes.
var reservedNames = map[string]bool{
	"new":      true,
	"edit":     true,
	"details":  true,
	"delete":   true,
	"links":    true,
	"mylinks":  true,
	"help":     true,
	"auth":     true,
	"api":      true,
	"healthz":  true,
	"apikeys":  true,
	"import":   true,
	"export":   true,
	"search":   true,
	"static":   true,
}

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9\-_.]+$`)

// ValidateName checks whether a link name is allowed. It returns a descriptive
// error if the name is empty, contains invalid characters, or conflicts with a
// reserved server endpoint.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("link name cannot be empty")
	}
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("link name may only contain letters, digits, hyphens, underscores, and dots")
	}
	if reservedNames[strings.ToLower(name)] {
		return fmt.Errorf("link name %q is reserved", name)
	}
	return nil
}

// ValidateTarget checks that a redirect target URL is safe to store and use.
// It rejects dangerous schemes and requires http or https.
func ValidateTarget(target string) error {
	if target == "" {
		return fmt.Errorf("target URL cannot be empty")
	}
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "vbscript:") {
		return fmt.Errorf("target URL scheme is not allowed")
	}
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("target URL must use http or https scheme")
	}
	return nil
}
