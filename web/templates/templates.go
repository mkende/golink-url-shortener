// Package webtemplates embeds the HTML template files so they can be included
// in the binary via go:embed.
package webtemplates

import "embed"

// FS holds all *.html template files embedded at compile time.
//
//go:embed *.html
var FS embed.FS
