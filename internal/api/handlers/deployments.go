package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	ghclient "github.com/LEFTEQ/lovinka-deployik/internal/github"
)

type DeploymentHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	Pipeline  *build.Pipeline
	Audit     *audit.Recorder
}

type triggerDeployRequest struct {
	Environment string `json:"environment"`
	Branch      string `json:"branch"`
	CreateTag   bool   `json:"create_tag"`
	TagName     string `json:"tag_name"`
}

// List returns deployments for a project with optional filters.
func (h *DeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filter := db.DeploymentFilter{
		ProjectID:   projectID,
		Branch:      q.Get("branch"),
		Environment: q.Get("environment"),
		Status:      q.Get("status"),
		TriggeredBy: q.Get("triggered_by"),
		From:        q.Get("from"),
		To:          q.Get("to"),
		Limit:       limit,
		Offset:      offset,
	}

	result, err := h.DB.ListDeploymentsFiltered(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list deployments"})
		return
	}
	if result.Deployments == nil {
		result.Deployments = []db.DeploymentWithUser{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Get returns a single deployment.
func (h *DeploymentHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	did := chi.URLParam(r, "did")
	deployment, _, ok := loadAuthorizedDeployment(w, r, h.DB, did)
	if !ok {
		return
	}
	if deployment.ProjectID != projectID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deployment not found"})
		return
	}
	writeJSON(w, http.StatusOK, deployment)
}

// Trigger starts a new deployment.
func (h *DeploymentHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	projectID := chi.URLParam(r, "id")

	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	var req triggerDeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	env := req.Environment
	if env == "" {
		env = "preview"
	}
	if env != "preview" && env != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = project.Branch
	}

	// Get user's GitHub token
	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user not found"})
		return
	}

	githubToken, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt token"})
		return
	}

	tagName := strings.TrimSpace(req.TagName)
	var releaseCommitSHA string
	var releaseCommitMessage string
	if req.CreateTag {
		if env != "production" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "create_tag is only supported for production releases"})
			return
		}
		if tagName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tag_name is required when create_tag is enabled"})
			return
		}

		client := ghclient.NewClient(githubToken)
		commit, err := client.GetLatestCommit(project.GithubOwner, project.GithubRepo, branch)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to resolve latest commit for release"})
			return
		}
		if err := client.CreateTagReference(project.GithubOwner, project.GithubRepo, tagName, commit.SHA); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		branch = tagName
		releaseCommitSHA = commit.SHA
		releaseCommitMessage = commit.Commit.Message
	}

	// Create deployment record
	deployment := &db.Deployment{
		ProjectID:           project.ID,
		Environment:         env,
		Branch:              branch,
		CommitSHA:           releaseCommitSHA,
		CommitMessage:       releaseCommitMessage,
		Status:              "queued",
		TriggeredBy:         claims.UserID,
		TriggerSource:       "manual",
		TriggeredByUsername: user.Username,
	}
	if err := h.DB.CreateDeployment(deployment); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create deployment"})
		return
	}

	// Hand off to the pipeline, which manages goroutine lifecycle, timeouts,
	// graceful shutdown, and panic recovery.
	h.Pipeline.Dispatch(project, deployment, githubToken, func(line string, stream string) {
		log.Printf("[deploy:%s] %s", deployment.ID[:8], line)
	})

	writeJSON(w, http.StatusAccepted, deployment)
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "deployment.trigger",
		ResourceType: "deployment",
		ResourceID:   deployment.ID,
		ProjectID:    project.ID,
		DeploymentID: deployment.ID,
		Metadata: map[string]any{
			"environment": env,
			"branch":      branch,
			"create_tag":  req.CreateTag,
			"tag_name":    tagName,
		},
	})
}

// GetLogs returns build logs for a deployment.
func (h *DeploymentHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	did := chi.URLParam(r, "did")
	if _, _, ok := loadAuthorizedDeployment(w, r, h.DB, did); !ok {
		return
	}
	logs, err := h.DB.GetBuildLogs(did)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get logs"})
		return
	}
	if logs == nil {
		logs = []db.BuildLog{}
	}
	writeJSON(w, http.StatusOK, logs)
}
