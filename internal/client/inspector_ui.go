package client

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

//go:embed inspector_assets/*
var inspectorAssets embed.FS

// serveInspectorAsset serves files from the embedded inspector_assets directory.
func serveInspectorAsset(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	// Strip leading slash for embed FS lookup.
	fsPath := "inspector_assets" + path

	data, err := inspectorAssets.ReadFile(fsPath)
	if err != nil {
		// Try to find the file without the leading path.
		if fsErr, ok := err.(*fs.PathError); ok {
			_ = fsErr
		}
		http.NotFound(w, r)
		return
	}

	// Set content type based on extension.
	ext := filepath.Ext(path)
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		if strings.HasSuffix(path, ".js") {
			ct = "application/javascript"
		} else if strings.HasSuffix(path, ".css") {
			ct = "text/css"
		} else {
			ct = "text/html; charset=utf-8"
		}
	}

	w.Header().Set("Content-Type", ct)
	w.Write(data)
}
