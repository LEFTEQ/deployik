package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// ScreenshotHandler serves deployment screenshot images and runs on-demand
// captures so existing projects (and stale homepages) can populate without
// requiring a redeploy.
type ScreenshotHandler struct {
	DB                *db.DB
	Docker            *build.DockerClient
	ScreenshotDir     string
	ScreenshotHostDir string
	ProxyNetwork      string
	JWTSecret         string
	Audit             *audit.Recorder
	// Wg, when non-nil, keeps server shutdown waiting on in-flight capture
	// goroutines. Wired to the pipeline's WaitGroup in main.go so a single
	// drain covers both build- and capture-spawned work.
	Wg *sync.WaitGroup
}

// captureBudget caps total wall time for an on-demand capture (queue wait +
// the headless Chrome run). The inner CaptureScreenshot has its own 30s
// timeout for the Docker container itself; this larger window absorbs any
// time spent waiting on the package-level screenshot semaphore.
const captureBudget = 90 * time.Second

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

// Capture handles POST /api/projects/{id}/screenshots/capture?environment=...
// Default: returns 200 ready when a screenshot file already exists, otherwise
// queues a goroutine and returns 202 capturing.
// `?force=1`: skip the idempotent ready check and always run capture.
// `?sync=1`: run inline (no goroutine) and return the result body with the
// concrete error message on failure — used by the manual refresh affordance
// so the UI can surface a useful diagnostic instead of silently failing.
func (h *ScreenshotHandler) Capture(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, claims, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	env := r.URL.Query().Get("environment")
	if env != "preview" && env != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}
	force := r.URL.Query().Get("force") == "1"
	sync := r.URL.Query().Get("sync") == "1"

	deployment, err := h.DB.GetLiveDeployment(projectID, env)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up live deployment"})
		return
	}
	if deployment == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no live deployment for this environment"})
		return
	}

	// Idempotent fast path: return ready when the file already exists, unless
	// the caller wants to force a re-capture (manual refresh, debugging).
	if !force && deployment.ScreenshotPath != "" {
		if _, err := os.Stat(deployment.ScreenshotPath); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":          "ready",
				"deployment_id":   deployment.ID,
				"screenshot_path": deployment.ScreenshotPath,
			})
			return
		}
	}

	// Need a target URL. Without an SSL-active domain there is nothing to capture.
	primary, err := h.DB.GetPrimaryDomain(projectID, env)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to look up primary domain"})
		return
	}
	if primary == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no SSL-active domain for this environment yet"})
		return
	}

	if h.Docker == nil || h.ScreenshotDir == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screenshot capture not configured on this server"})
		return
	}

	if h.Audit != nil && claims != nil {
		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "screenshot.capture",
			ResourceType: "deployment",
			ResourceID:   deployment.ID,
			ProjectID:    projectID,
			DeploymentID: deployment.ID,
			Metadata:     map[string]any{"environment": env, "trigger": "manual", "force": force, "sync": sync},
		})
	}

	if sync {
		path, err := h.captureNow(r.Context(), project, deployment.ID, env, primary.DomainName)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status":        "failed",
				"error":         err.Error(),
				"deployment_id": deployment.ID,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":          "ready",
			"deployment_id":   deployment.ID,
			"screenshot_path": path,
		})
		return
	}

	if h.Wg != nil {
		h.Wg.Add(1)
	}
	go h.runCapture(project, deployment.ID, env, primary.DomainName)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":        "capturing",
		"deployment_id": deployment.ID,
	})
}

// captureNow runs the capture inline and returns the resulting screenshot
// path. Shared by the goroutine path (runCapture) and the sync path.
func (h *ScreenshotHandler) captureNow(parent context.Context, project *db.Project, deploymentID, environment, domainName string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, captureBudget)
	defer cancel()

	url := "https://" + domainName
	if project.IsEnvironmentProtected(environment) && h.JWTSecret != "" {
		token := auth.MintSiteAuthBypassToken(h.JWTSecret, project.ID, environment)
		url = build.AppendBypassToken(url, auth.SiteAuthBypassParam, token)
	}

	path, err := build.CaptureScreenshot(ctx, h.Docker, url, deploymentID, h.ScreenshotDir, h.ScreenshotHostDir, h.ProxyNetwork)
	if err != nil {
		return "", err
	}
	if err := h.DB.UpdateDeploymentScreenshot(deploymentID, path); err != nil {
		return "", fmt.Errorf("persist screenshot path: %w", err)
	}
	return path, nil
}

func (h *ScreenshotHandler) runCapture(project *db.Project, deploymentID, environment, domainName string) {
	if h.Wg != nil {
		defer h.Wg.Done()
	}
	if _, err := h.captureNow(context.Background(), project, deploymentID, environment, domainName); err != nil {
		log.Printf("Screenshot: on-demand capture failed for %s: %v", deploymentID, err)
	}
}
