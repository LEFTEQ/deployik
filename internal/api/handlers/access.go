package handlers

import (
	"net/http"

	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/authz"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

func loadAuthorizedProject(w http.ResponseWriter, r *http.Request, database *db.DB, projectID string) (*db.Project, *auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	project, err := authz.LoadProject(database, claims, projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get project"})
		return nil, nil, false
	}
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return nil, nil, false
	}
	return project, claims, true
}

func loadAuthorizedDeployment(w http.ResponseWriter, r *http.Request, database *db.DB, deploymentID string) (*db.Deployment, *auth.Claims, bool) {
	claims := auth.GetClaims(r.Context())
	deployment, err := authz.LoadDeployment(database, claims, deploymentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get deployment"})
		return nil, nil, false
	}
	if deployment == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return nil, nil, false
	}
	return deployment, claims, true
}
