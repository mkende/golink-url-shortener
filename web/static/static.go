// Package static embeds static web assets that are served directly by the HTTP
// server. These assets are compiled into the binary so no external files are
// required at runtime.
package static

import "embed"

// FS is the embedded filesystem containing all static assets.
//
//go:embed favicon.svg
var FS embed.FS
