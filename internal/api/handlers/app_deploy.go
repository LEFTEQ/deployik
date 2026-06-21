package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

const appDeployMemberTimeout = 20 * time.Minute

// appMemberTriggerSource is the deployments.trigger_source recorded for every
// deployment a coordinated app deploy or rollback creates. It MUST be a value
// allowed by the deployments CHECK constraint (migration 008:
// 'manual','webhook','api'). Coordinated rollouts are API/MCP-driven, so 'api'
// fits. Writing an out-of-vocabulary value (e.g. "app_deploy") makes
// CreateDeployment fail the CHECK and aborts the whole rollout before any member
// builds. The app-deploy context is still recorded via app_releases + audit logs.
const appMemberTriggerSource = "api"

// inflightRollouts guards against overlapping coordinated deploy/rollback jobs
// for the same (app, environment). Deployik is single-process, so an in-memory
// set is sufficient; a second request gets 409 until the first finishes.
var inflightRollouts sync.Map // key "appID:env" -> struct{}

func acquireRollout(appID, environment string) bool {
	_, loaded := inflightRollouts.LoadOrStore(appID+":"+environment, struct{}{})
	return !loaded
}

func releaseRollout(appID, environment string) {
	inflightRollouts.Delete(appID + ":" + environment)
}

type appDeployRequest struct {
	Environment string `json:"environment"`
}

type appRollbackRequest struct {
	Environment string `json:"environment"`
	ReleaseID   string `json:"release_id"`
}

func normalizeAppEnvironment(value string) (string, bool) {
	switch value {
	case "", "production":
		return "production", true
	case "preview":
		return "preview", true
	default:
		return "", false
	}
}

// resolveDeployToken loads + decrypts the calling user's GitHub token, used to
// clone every member during a coordinated deploy.
func (h *AppHandler) resolveDeployToken(userID string) (string, string, bool) {
	user, err := h.DB.GetUserByID(userID)
	if err != nil || user == nil {
		return "", "", false
	}
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		return "", "", false
	}
	return token, user.Username, true
}

// Deploy runs a coordinated, ordered, health-gated rollout of an app's members
// for one environment, then records an app_release snapshot. Async: returns 202
// immediately; per-member deployments are visible during the rollout and the
// release row appears when it finishes.
func (h *AppHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	app, claims, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	if h.Pipeline == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "deploys are not enabled on this server"})
		return
	}
	var req appDeployRequest
	// Body is optional (empty → production default), but a malformed body must be
	// rejected — silently defaulting could trigger an unintended production deploy.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	environment, valid := normalizeAppEnvironment(req.Environment)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}

	members, err := h.DB.ListProjectsByApp(app.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}
	if len(members) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "app has no member projects to deploy"})
		return
	}
	token, username, ok := h.resolveDeployToken(claims.UserID)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve GitHub token"})
		return
	}

	if !acquireRollout(app.ID, environment) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a deploy or rollback is already in progress for this app + environment"})
		return
	}
	// Invalidate the health cache now (so the next read reflects the in-flight
	// "deploying" state) and again when the rollout finishes (so the final state
	// isn't masked for up to the TTL).
	invalidateAppHealth(app.ID, environment)
	go func() {
		defer releaseRollout(app.ID, environment)
		defer invalidateAppHealth(app.ID, environment)
		h.runAppDeploy(app, environment, members, claims.UserID, username, token)
	}()

	h.recordAudit(claims.UserID, "app.deploy", app.ID, map[string]any{"environment": environment, "member_count": len(members)})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"app_id":         app.ID,
		"environment":    environment,
		"status":         "deploying",
		"member_count":   len(members),
		"deploy_ordered": app.DeployOrdered,
	})
}

// runAppDeploy is the background rollout. Each member is deployed via the normal
// pipeline (build → health → blue-green swap); RunRollout halts at the first
// unhealthy member. On failure it best-effort rolls the whole app back to the
// last succeeded release. Always records an app_release snapshot.
func (h *AppHandler) runAppDeploy(app *db.App, environment string, members []db.Project, userID, username, token string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("app-deploy panic for app %s: %v", app.ID, rec)
		}
	}()

	batches := build.OrderAppMembers(members, app.DeployOrdered)
	result := build.RunRollout(batches, func(member db.Project) (string, bool) {
		return h.deployMember(member, environment, userID, username, token)
	})

	status := "succeeded"
	if !result.Succeeded {
		status = "failed"
	}
	releaseMembers := make([]db.AppReleaseMember, 0, len(result.Deployed))
	for _, d := range result.Deployed {
		releaseMembers = append(releaseMembers, db.AppReleaseMember{ProjectID: d.ProjectID, DeploymentID: d.DeploymentID})
	}
	if _, err := h.DB.CreateAppRelease(&db.AppRelease{AppID: app.ID, Environment: environment, Status: status}, releaseMembers); err != nil {
		log.Printf("app-deploy: failed to record release for app %s: %v", app.ID, err)
	}

	if !result.Succeeded {
		log.Printf("app-deploy: app %s %s rollout failed at project %s; %d member(s) were already swapped", app.ID, environment, result.FailedProjectID, len(result.Deployed))
		h.rollbackToLastGoodRelease(app, environment, userID, username, token)
	} else {
		log.Printf("app-deploy: app %s %s rollout succeeded (%d members)", app.ID, environment, len(result.Deployed))
	}
}

// deployMember creates a deployment for one member and runs the pipeline
// synchronously, reporting whether it reached a live (healthy + swapped) state.
func (h *AppHandler) deployMember(member db.Project, environment, userID, username, token string) (string, bool) {
	dep := &db.Deployment{
		ProjectID:           member.ID,
		Environment:         environment,
		Branch:              member.Branch,
		Status:              "queued",
		TriggeredBy:         userID,
		TriggerSource:       appMemberTriggerSource,
		TriggeredByUsername: username,
	}
	if environment == "preview" {
		if instance, _, err := ensurePreviewTarget(h.DB, &member, member.Branch); err == nil && instance != nil {
			dep.PreviewInstanceID = instance.ID
		}
	}
	if err := h.DB.CreateDeployment(dep); err != nil {
		log.Printf("app-deploy: create deployment for member %s failed: %v", member.ID, err)
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), appDeployMemberTimeout)
	defer cancel()
	m := member
	h.Pipeline.Deploy(ctx, &m, dep, token, func(line, stream string) {
		log.Printf("[app-deploy:%s] %s", dep.ID[:8], line)
	})

	final, err := h.DB.GetDeployment(dep.ID)
	if err != nil || final == nil {
		return dep.ID, false
	}
	return dep.ID, final.Status == "live"
}

// rollbackToLastGoodRelease redeploys every member to the deployments recorded
// in the most recent succeeded release. Best-effort: logged, never fatal.
func (h *AppHandler) rollbackToLastGoodRelease(app *db.App, environment, userID, username, token string) {
	releases, err := h.DB.ListAppReleases(app.ID, environment)
	if err != nil {
		log.Printf("app-deploy: cannot list releases for rollback of app %s: %v", app.ID, err)
		return
	}
	for _, rel := range releases {
		if rel.Status != "succeeded" {
			continue
		}
		log.Printf("app-deploy: rolling app %s %s back to release %s", app.ID, environment, rel.ID)
		h.redeployRelease(app, environment, rel.ID, userID, username, token)
		return
	}
	log.Printf("app-deploy: no prior succeeded release for app %s %s — nothing to roll back to", app.ID, environment)
}

// Rollback redeploys an app's members to a chosen prior release's deployments.
func (h *AppHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	app, claims, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	if h.Pipeline == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "deploys are not enabled on this server"})
		return
	}
	var req appRollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	environment, valid := normalizeAppEnvironment(req.Environment)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}
	if req.ReleaseID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "release_id is required"})
		return
	}
	release, err := h.DB.GetAppRelease(req.ReleaseID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load release"})
		return
	}
	if release == nil || release.AppID != app.ID || release.Environment != environment {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "release not found for this app/environment"})
		return
	}
	token, username, ok := h.resolveDeployToken(claims.UserID)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve GitHub token"})
		return
	}

	if !acquireRollout(app.ID, environment) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a deploy or rollback is already in progress for this app + environment"})
		return
	}
	// Invalidate the health cache now (so the next read reflects the in-flight
	// rollback) and again when it finishes (so the final state isn't masked).
	invalidateAppHealth(app.ID, environment)
	go func() {
		defer releaseRollout(app.ID, environment)
		defer invalidateAppHealth(app.ID, environment)
		h.redeployRelease(app, environment, release.ID, claims.UserID, username, token)
	}()

	h.recordAudit(claims.UserID, "app.rollback", app.ID, map[string]any{"environment": environment, "release_id": release.ID})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"app_id":      app.ID,
		"environment": environment,
		"release_id":  release.ID,
		"status":      "rolling_back",
	})
}

// redeployRelease redeploys each member of a release to its recorded commit,
// then records a new release (status rolled_back) snapshotting the new
// deployments. Members deploy in deploy_order so a DB/api comes up before web.
func (h *AppHandler) redeployRelease(app *db.App, environment, releaseID, userID, username, token string) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("app-rollback panic for app %s: %v", app.ID, rec)
		}
	}()

	release, err := h.DB.GetAppRelease(releaseID)
	if err != nil || release == nil {
		log.Printf("app-rollback: release %s not found: %v", releaseID, err)
		return
	}

	// Order the recorded members by their project's current deploy_order.
	type target struct {
		project    db.Project
		recordedID string
	}
	targets := make([]target, 0, len(release.Members))
	for _, m := range release.Members {
		project, err := h.DB.GetProject(m.ProjectID)
		if err != nil || project == nil {
			log.Printf("app-rollback: member project %s missing, skipping: %v", m.ProjectID, err)
			continue
		}
		targets = append(targets, target{project: *project, recordedID: m.DeploymentID})
	}
	projects := make([]db.Project, len(targets))
	for i, t := range targets {
		projects[i] = t.project
	}
	recordedByProject := make(map[string]string, len(targets))
	for _, t := range targets {
		recordedByProject[t.project.ID] = t.recordedID
	}

	var (
		mu         sync.Mutex
		newDeploys []db.AppReleaseMember
	)
	batches := build.OrderAppMembers(projects, app.DeployOrdered)
	result := build.RunRollout(batches, func(member db.Project) (string, bool) {
		newID, healthy := h.redeployMemberToDeployment(member, environment, recordedByProject[member.ID], userID, username, token)
		mu.Lock()
		newDeploys = append(newDeploys, db.AppReleaseMember{ProjectID: member.ID, DeploymentID: newID})
		mu.Unlock()
		return newID, healthy
	})

	status := "rolled_back"
	if !result.Succeeded {
		status = "failed"
	}
	if _, err := h.DB.CreateAppRelease(&db.AppRelease{AppID: app.ID, Environment: environment, Status: status}, newDeploys); err != nil {
		log.Printf("app-rollback: failed to record release for app %s: %v", app.ID, err)
	}
	log.Printf("app-rollback: app %s %s rollback to release %s finished: %s", app.ID, environment, releaseID, status)
}

// redeployMemberToDeployment creates a new deployment for member that reuses the
// branch + commit of a recorded deployment, then runs the pipeline.
func (h *AppHandler) redeployMemberToDeployment(member db.Project, environment, recordedDeploymentID, userID, username, token string) (string, bool) {
	branch := member.Branch
	commitSHA := ""
	commitMsg := ""
	if recordedDeploymentID != "" {
		if recorded, err := h.DB.GetDeployment(recordedDeploymentID); err == nil && recorded != nil {
			if recorded.Branch != "" {
				branch = recorded.Branch
			}
			commitSHA = recorded.CommitSHA
			commitMsg = recorded.CommitMessage
		}
	}
	dep := &db.Deployment{
		ProjectID:           member.ID,
		Environment:         environment,
		Branch:              branch,
		CommitSHA:           commitSHA,
		CommitMessage:       commitMsg,
		Status:              "queued",
		TriggeredBy:         userID,
		TriggerSource:       appMemberTriggerSource,
		TriggeredByUsername: username,
	}
	if environment == "preview" {
		if instance, _, err := ensurePreviewTarget(h.DB, &member, branch); err == nil && instance != nil {
			dep.PreviewInstanceID = instance.ID
		}
	}
	if err := h.DB.CreateDeployment(dep); err != nil {
		log.Printf("app-rollback: create deployment for member %s failed: %v", member.ID, err)
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), appDeployMemberTimeout)
	defer cancel()
	m := member
	h.Pipeline.Deploy(ctx, &m, dep, token, func(line, stream string) {
		log.Printf("[app-rollback:%s] %s", dep.ID[:8], line)
	})
	final, err := h.DB.GetDeployment(dep.ID)
	if err != nil || final == nil {
		return dep.ID, false
	}
	return dep.ID, final.Status == "live"
}

// ListReleases returns an app's coordinated-deploy history for an environment.
func (h *AppHandler) ListReleases(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	environment, valid := normalizeAppEnvironment(r.URL.Query().Get("environment"))
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}
	releases, err := h.DB.ListAppReleases(app.ID, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list releases"})
		return
	}
	if releases == nil {
		releases = []db.AppRelease{}
	}
	writeJSON(w, http.StatusOK, releases)
}
