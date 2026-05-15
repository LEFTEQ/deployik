package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
)

// ServiceHandler owns the per-project service (sidecar) endpoints. v1 only
// implements List + Attach; Detach/Credentials/RegeneratePassword/Restart/Reset
// are wired in Tasks 12 and 13.
type ServiceHandler struct {
	DB        *db.DB
	Manager   *services.Manager
	Encryptor *crypto.Encryptor
	Audit     *audit.Recorder
}

// attachServiceRequest is the JSON body accepted by POST /api/projects/{id}/services.
// v1 only supports {environment, type=postgres}; future fields (image override,
// resource sizing, custom config) will extend this struct.
type attachServiceRequest struct {
	Environment string         `json:"environment"`
	Type        db.ServiceType `json:"type"`
}

// serviceResponse is the JSON shape returned by List + Attach. It deliberately
// excludes db_password_encrypted (and any plaintext password) so secrets only
// flow through the dedicated /credentials endpoint (Task 12). Adding new
// fields here is the right place for additive metadata (config_json, etc.).
type serviceResponse struct {
	ID            string           `json:"id"`
	ProjectID     string           `json:"project_id"`
	Environment   string           `json:"environment"`
	Type          db.ServiceType   `json:"type"`
	Image         string           `json:"image"`
	DBName        string           `json:"db_name"`
	DBUser        string           `json:"db_user"`
	HostPort      int              `json:"host_port"`
	Status        db.ServiceStatus `json:"status"`
	LastStartedAt db.NullableTime  `json:"last_started_at"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
}

func toServiceResponse(s db.ProjectService) serviceResponse {
	return serviceResponse{
		ID:            s.ID,
		ProjectID:     s.ProjectID,
		Environment:   s.Environment,
		Type:          s.ServiceType,
		Image:         s.Image,
		DBName:        s.DBName,
		DBUser:        s.DBUser,
		HostPort:      s.HostPort,
		Status:        s.Status,
		LastStartedAt: s.LastStartedAt,
		CreatedAt:     s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// List returns every service attached to a project across both environments.
// Credentials (encrypted password) are never included — the dedicated
// /credentials endpoint (Task 12) is the only source for those.
func (h *ServiceHandler) List(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	rows, err := h.DB.ListServicesByProject(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]serviceResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toServiceResponse(row))
	}
	writeJSON(w, http.StatusOK, out)
}

// Attach provisions a new service row (v1: postgres only) for the given
// environment. The container is NOT started here — the first deployment after
// attach brings it up via the pipeline's EnsureServices hook (Task 9). A
// re-attach to the same (project, env, type) returns 409 Conflict.
func (h *ServiceHandler) Attach(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	var req attachServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Environment != "preview" && req.Environment != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}
	if req.Type != db.ServiceTypePostgres {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "v1 only supports type=postgres"})
		return
	}

	spec, err := h.Manager.Provision(r.Context(), project, req.Environment, req.Type)
	if err != nil {
		if errors.Is(err, services.ErrAlreadyProvisioned) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "service already attached for this environment"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Re-fetch the persisted row so the response carries the canonical
	// created_at/updated_at values rather than what Provision had in-memory.
	row, err := h.DB.GetServiceByProjectEnv(project.ID, req.Environment, req.Type)
	if err != nil || row == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "service was provisioned but could not be reloaded"})
		return
	}
	writeJSON(w, http.StatusCreated, toServiceResponse(*row))

	claims := auth.GetClaims(r.Context())
	var userID string
	if claims != nil {
		userID = claims.UserID
	}
	h.Audit.Record(audit.Entry{
		UserID:       userID,
		Action:       "service.attach",
		ResourceType: "service",
		ResourceID:   spec.ServiceID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": req.Environment, "type": string(req.Type)},
	})
}
