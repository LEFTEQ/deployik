package authz

import (
	"github.com/lefteq/lovinka-deployik/internal/auth"
	"github.com/lefteq/lovinka-deployik/internal/db"
)

func CanAccessProject(claims *auth.Claims, project *db.Project) bool {
	if claims == nil || project == nil {
		return false
	}
	return claims.Role == "admin" || project.UserID == claims.UserID
}

func LoadProject(database *db.DB, claims *auth.Claims, projectID string) (*db.Project, error) {
	if claims == nil {
		return nil, nil
	}
	if claims != nil && claims.Role == "admin" {
		return database.GetProject(projectID)
	}
	return database.GetProjectForUser(projectID, claims.UserID)
}

func LoadDeployment(database *db.DB, claims *auth.Claims, deploymentID string) (*db.Deployment, error) {
	if claims == nil {
		return nil, nil
	}
	if claims != nil && claims.Role == "admin" {
		return database.GetDeployment(deploymentID)
	}
	return database.GetDeploymentForUser(deploymentID, claims.UserID)
}
