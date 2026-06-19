package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// AppHandler serves /apps — bundles of projects within a workspace.
type AppHandler struct {
	DB        *db.DB
	Pipeline  *build.Pipeline   // coordinated app deploys (P4); nil disables deploy/rollback
	Encryptor *crypto.Encryptor // decrypts the caller's GitHub token for deploys
}

type createAppRequest struct {
	Name           string   `json:"name"`
	OrganizationID string   `json:"organization_id"`
	ProjectIDs     []string `json:"project_ids"`
}

type updateAppRequest struct {
	Name string `json:"name"`
}

type appProjectsRequest struct {
	ProjectIDs []string `json:"project_ids"`
}

// loadManagedApp loads the app by URL id and verifies the caller is a member of
// its organization. Writes the error response + returns ok=false on failure.
func (h *AppHandler) loadManagedApp(w http.ResponseWriter, r *http.Request) (*db.App, *auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	appID := chi.URLParam(r, "id")
	app, err := h.DB.GetAppForUser(appID, claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load app"})
		return nil, nil, false
	}
	if app == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
		return nil, nil, false
	}
	return app, claims, true
}

func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	apps, err := h.DB.ListAppsForUser(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list apps"})
		return
	}
	if apps == nil {
		apps = []db.App{}
	}
	writeJSON(w, http.StatusOK, apps)
}

func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	// Default to the caller's personal workspace when no org is given, matching
	// project creation (resolveCreateOrganizationID).
	organizationID, err := h.resolveCreateOrganizationID(claims.UserID, strings.TrimSpace(req.OrganizationID))
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "organization not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if !h.canAttachProjects(claims, organizationID, req.ProjectIDs) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	app, err := h.DB.CreateApp(&db.AppCreate{
		OrganizationID: organizationID,
		Name:           req.Name,
		ProjectIDs:     req.ProjectIDs,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create app"})
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (h *AppHandler) Update(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	var req updateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	updated, err := h.DB.UpdateAppName(app.ID, req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update app"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *AppHandler) Delete(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	if err := h.DB.DeleteApp(app.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete app"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AppHandler) AddProjects(w http.ResponseWriter, r *http.Request) {
	app, claims, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	var req appProjectsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if !h.canAttachProjects(claims, app.OrganizationID, req.ProjectIDs) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	if err := h.DB.AddProjectsToApp(app.ID, req.ProjectIDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add projects"})
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *AppHandler) RemoveProject(w http.ResponseWriter, r *http.Request) {
	app, claims, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	projectID := chi.URLParam(r, "pid")
	project, err := h.DB.GetProject(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load project"})
		return
	}
	if project == nil || project.AppID != app.ID || !h.canManageOrg(claims, project.OrganizationID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found in app"})
		return
	}
	if err := h.DB.RemoveProjectFromApp(projectID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove project"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// appHealth is the composite "unified view" payload.
type appHealth struct {
	App     *db.App           `json:"app"`
	Members []appHealthMember `json:"members"`
}

type appHealthMember struct {
	Project          db.Project `json:"project"`
	LatestPreview    *time.Time `json:"latest_preview_deploy_at,omitempty"`
	LatestProduction *time.Time `json:"latest_production_deploy_at,omitempty"`
}

func (h *AppHandler) GetHealth(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	members, err := h.DB.ListProjectsByApp(app.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}
	out := appHealth{App: app, Members: make([]appHealthMember, 0, len(members))}
	for i := range members {
		// GetProject populates the latest-deploy timestamps; ListProjectsByApp
		// returns the lighter row, so re-fetch each member for its deploy state.
		full, err := h.DB.GetProject(members[i].ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load member"})
			return
		}
		if full == nil {
			continue
		}
		out.Members = append(out.Members, appHealthMember{
			Project:          *full,
			LatestPreview:    full.LatestPreviewDeployAt,
			LatestProduction: full.LatestProductionDeployAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// resolveCreateOrganizationID validates an explicit workspace id or defaults to
// the caller's personal workspace, matching ProjectHandler.resolveCreateOrganizationID.
func (h *AppHandler) resolveCreateOrganizationID(userID, organizationID string) (string, error) {
	if organizationID != "" {
		organization, err := h.DB.GetOrganizationForUser(organizationID, userID)
		if err != nil {
			return "", err
		}
		if organization == nil {
			return "", fmt.Errorf("organization not found")
		}
		return organization.ID, nil
	}
	user, err := h.DB.GetUserByID(userID)
	if err != nil {
		return "", fmt.Errorf("failed to load user: %w", err)
	}
	if user == nil {
		return "", fmt.Errorf("user not found")
	}
	organization, err := h.DB.EnsurePersonalOrganization(user)
	if err != nil {
		return "", fmt.Errorf("failed to prepare personal organization: %w", err)
	}
	return organization.ID, nil
}

// canManageOrg reports whether the caller is a member of the organization.
func (h *AppHandler) canManageOrg(claims *auth.Claims, orgID string) bool {
	ok, err := h.DB.IsOrganizationMember(orgID, claims.UserID)
	if err != nil {
		return false
	}
	return ok
}

// canAttachProjects checks every project exists, is in the target org, and the
// caller can access it. Empty list = ok.
func (h *AppHandler) canAttachProjects(claims *auth.Claims, orgID string, projectIDs []string) bool {
	for _, id := range projectIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		project, err := h.DB.GetProject(id)
		if err != nil || project == nil {
			return false
		}
		if project.OrganizationID != orgID {
			return false
		}
		if project.UserID != claims.UserID && !h.canManageOrg(claims, orgID) {
			return false
		}
	}
	return true
}
