package handlers

import (
	"net/http"

	"github.com/LEFTEQ/lovinka-deployik/internal/version"
)

// HealthHandler serves GET /api/health. Includes build version metadata so
// the SPA can render a "running build" badge in the sidebar footer. Used by
// the docker HEALTHCHECK and by scripts/deploy-vps.sh, so the response stays
// small and the status field is always "ok" when the process is up.
type HealthHandler struct {
	Version *version.Info // nil-safe; omitted from response when nil
}

type healthResponse struct {
	Status  string        `json:"status"`
	Version *version.Info `json:"version,omitempty"`
}

func (h *HealthHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:  "ok",
		Version: h.Version,
	})
}
