// Package server — install.go serves the client install script at /install.sh.
//
// Usage: curl -sSfL https://fonzygrok.com/install.sh | sh
package server

import (
	_ "embed"
	"net/http"
)

//go:embed install_script.sh
var installScript []byte

// handleInstallScript serves the client installer shell script.
func handleInstallScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, max-age=0")
	w.Write(installScript)
}
