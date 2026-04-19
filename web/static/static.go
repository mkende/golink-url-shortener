// Package static embeds static web assets that are served directly by the HTTP
// server. These assets are compiled into the binary so no external files are
// required at runtime.
package static

import "embed"

// FS is the embedded filesystem containing all static assets.
//
// Vendor assets (Bulma CSS and HTMX) are self-hosted rather than loaded from
// external CDNs. See web/static/app.js for the rationale.
//
//go:embed favicon.svg app.js app.css htmx-1.9.10.min.js bulma-0.9.4.min.css
var FS embed.FS
