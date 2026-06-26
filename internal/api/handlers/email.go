package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/lefteq/lovinka-deployik/internal/audit"
	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
	projectemail "github.com/lefteq/lovinka-deployik/internal/email"
)

type ProjectEmailHandler struct {
	DB    *db.DB
	Email *projectemail.Service
	Audit *audit.Recorder
}

func (h *ProjectEmailHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}
	if h.Email == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "email support is not configured"})
		return
	}
	payload, err := h.Email.GetProjectPayload(r.Context(), project)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load email settings"})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *ProjectEmailHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}
	if h.Email == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "email support is not configured"})
		return
	}

	var req projectemail.SaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	payload, err := h.Email.SaveProjectSettings(r.Context(), project, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "email.update",
		ResourceType: "project_email_settings",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"provider":      payload.Settings.Provider,
			"smtp_host":     payload.Settings.SMTPHost,
			"smtp_port":     payload.Settings.SMTPPort,
			"smtp_security": payload.Settings.SMTPSecurity,
		},
	})
	writeJSON(w, http.StatusOK, payload)
}

func (h *ProjectEmailHandler) TestSMTP(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}
	if h.Email == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "email support is not configured"})
		return
	}
	payload, err := h.Email.TestProjectSMTP(r.Context(), project)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "status": payload.Settings.Status})
		return
	}
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "email.test_smtp",
		ResourceType: "project_email_settings",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"status": payload.Settings.Status,
		},
	})
	writeJSON(w, http.StatusOK, payload)
}
