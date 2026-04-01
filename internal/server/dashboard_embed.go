// Package server — dashboard_embed.go embeds all dashboard assets
// (templates, CSS, JS) into the binary via embed.FS.
//
// Zero external file dependencies at runtime.
// REF: SPR-019 T-070
package server

import "embed"

//go:embed dashboard_assets
var dashboardFS embed.FS
