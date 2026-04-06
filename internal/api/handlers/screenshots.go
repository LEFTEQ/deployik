package handlers

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// ScreenshotHandler serves deployment screenshot images.
type ScreenshotHandler struct {
	DB *db.DB
}

// Get serves the screenshot PNG for a deployment.
func (h *ScreenshotHandler) Get(w http.ResponseWriter, r *http.Request) {
	did := chi.URLParam(r, "did")
	if _, _, ok := loadAuthorizedDeployment(w, r, h.DB, did); !ok {
		return
	}

	deployment, err := h.DB.GetDeployment(did)
	if err != nil || deployment == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return
	}

	if deployment.ScreenshotPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no screenshot available"})
		return
	}

	if _, err := os.Stat(deployment.ScreenshotPath); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "screenshot file not found"})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, deployment.ScreenshotPath)
}
