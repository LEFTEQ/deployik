package api

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// staticFS holds the embedded filesystem for the SPA.
// Set by SetStaticFS during initialization.
var staticFS fs.FS

// SetStaticFS sets the filesystem used to serve the SPA.
func SetStaticFS(f fs.FS) {
	staticFS = f
}

// SPAHandler returns an http.HandlerFunc that serves the embedded React SPA.
// For any file that exists in the embedded FS, it serves that file.
// For all other paths, it serves index.html (SPA fallback).
func SPAHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Don't serve SPA for API routes
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/ws") {
			http.NotFound(w, r)
			return
		}

		if staticFS == nil {
			// During development, proxy or serve a placeholder
			if os.Getenv("DEV_MODE") == "true" {
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`<!DOCTYPE html><html><body><h1>Deployik API</h1><p>Frontend dev server: <a href="http://localhost:5173">http://localhost:5173</a></p></body></html>`))
				return
			}
			http.Error(w, "SPA not embedded", http.StatusInternalServerError)
			return
		}

		// Try to serve the file directly
		upath := r.URL.Path
		if upath == "/" {
			upath = "/index.html"
		}
		upath = strings.TrimPrefix(upath, "/")

		// Check if file exists
		if _, err := fs.Stat(staticFS, upath); err == nil {
			// Set content type based on extension
			ext := path.Ext(upath)
			switch ext {
			case ".js":
				w.Header().Set("Content-Type", "application/javascript")
			case ".css":
				w.Header().Set("Content-Type", "text/css")
			case ".svg":
				w.Header().Set("Content-Type", "image/svg+xml")
			case ".png":
				w.Header().Set("Content-Type", "image/png")
			case ".ico":
				w.Header().Set("Content-Type", "image/x-icon")
			}

			// Cache static assets (hashed filenames)
			if strings.Contains(upath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}

			http.ServeFileFS(w, r, staticFS, upath)
			return
		}

		// SPA fallback: serve index.html for client-side routing
		data, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	}
}
