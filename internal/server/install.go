// Package server — install.go serves client install scripts.
//
// Unix:    curl -sSfL https://fonzygrok.com/install.sh | sh
// Windows: irm https://fonzygrok.com/install.ps1 | iex
package server

import (
	_ "embed"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// handleDownload serves client binaries from the downloads directory.
// URL: /download/fonzygrok.exe, /download/fonzygrok-linux-amd64, etc.
func handleDownload(downloadDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract filename from path — only allow simple filenames, no traversal.
		name := strings.TrimPrefix(r.URL.Path, "/download/")
		if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}

		path := filepath.Join(downloadDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename="+name)
		w.Header().Set("Cache-Control", "no-cache, max-age=0")
		w.Write(data)
	}
}
