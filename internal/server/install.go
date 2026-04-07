// Package server — install.go serves client install scripts.
//
// Unix:    curl -sSfL https://fonzygrok.com/install.sh | sh
// Windows: irm https://fonzygrok.com/install.ps1 | iex
package server

import (
	_ "embed"
	"net/http"
)

//go:embed install_script.sh
var installScript []byte

//go:embed install_script.ps1
var installScriptPS1 []byte

// handleInstallScript serves the client installer shell script (Unix).
func handleInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, max-age=0")
	w.Write(installScript)
}

// handleInstallScriptPS1 serves the client installer PowerShell script (Windows).
func handleInstallScriptPS1(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, max-age=0")
	w.Write(installScriptPS1)
}
