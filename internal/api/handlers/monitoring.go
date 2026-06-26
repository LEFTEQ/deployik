package handlers

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

// MonitoringHandler serves GET /api/monitoring/targets — a Prometheus http_sd
// document listing every active project's primary production URL. It is a
// system-scoped, token-gated endpoint (NOT per-user) so the external devops
// monitor can discover all live production sites in one read. When no
// MONITORING_TOKEN is configured the endpoint 404s, so it stays dark until
// deliberately enabled.
type MonitoringHandler struct {
	DB    *db.DB
	Token string
}

// sdTarget mirrors the Prometheus http_sd_configs JSON shape:
// https://prometheus.io/docs/prometheus/latest/http_sd/
type sdTarget struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func (h *MonitoringHandler) Targets(w http.ResponseWriter, r *http.Request) {
	if h.Token == "" {
		http.NotFound(w, r)
		return
	}
	if !h.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	targets, err := h.DB.ListProductionMonitorTargets()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list monitoring targets"})
		return
	}

	out := make([]sdTarget, 0, len(targets))
	for _, t := range targets {
		healthPath := t.HealthPath
		if healthPath == "" {
			healthPath = "/"
		}
		protected := "false"
		if t.Protected {
			protected = "true"
		}
		out = append(out, sdTarget{
			Targets: []string{"https://" + t.DomainName},
			Labels: map[string]string{
				"project":      t.ProjectName,
				"deployik_env": "production",
				"protected":    protected,
				"health_path":  healthPath,
			},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// authorized does a constant-time comparison of the Bearer credential against
// the configured system token.
func (h *MonitoringHandler) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.Token)) == 1
}
