package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/analytics"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type ProjectAnalyticsHandler struct {
	DB        *db.DB
	Analytics *analytics.Service
}

func (h *ProjectAnalyticsHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	domains, err := h.DB.ListDomains(project.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load project domains"})
		return
	}

	payload, err := h.loadPayload(r, project, domains)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load analytics"})
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func (h *ProjectAnalyticsHandler) Verify(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	domains, err := h.DB.ListDomains(project.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load project domains"})
		return
	}

	payload, err := h.loadPayload(r, project, domains)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify analytics"})
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func (h *ProjectAnalyticsHandler) loadPayload(r *http.Request, project *db.Project, domains []db.Domain) (analytics.ProjectPayload, error) {
	if h.Analytics == nil {
		return analytics.ProjectPayload{
			Environment: string(analytics.NormalizeEnvironment(r.URL.Query().Get("environment"))),
			Range:       string(analytics.NormalizeRange(r.URL.Query().Get("range"))),
			Timezone:    analytics.NormalizeTimezone(r.URL.Query().Get("timezone")),
		}, nil
	}

	return h.Analytics.GetProjectPayload(r.Context(), project, domains, analytics.QueryOptions{
		Environment: analytics.NormalizeEnvironment(r.URL.Query().Get("environment")),
		Range:       analytics.NormalizeRange(r.URL.Query().Get("range")),
		Timezone:    analytics.NormalizeTimezone(r.URL.Query().Get("timezone")),
	})
}
