package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
)

type ProjectHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
}

var slugRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

type createProjectRequest struct {
	Name           string `json:"name"`
	GithubRepo     string `json:"github_repo"`
	GithubOwner    string `json:"github_owner"`
	Branch         string `json:"branch"`
	Framework      string `json:"framework"`
	BuildCommand   string `json:"build_command"`
	InstallCommand string `json:"install_command"`
	NodeVersion    string `json:"node_version"`
}

type updateProjectRequest struct {
	Name           *string `json:"name,omitempty"`
	Branch         *string `json:"branch,omitempty"`
	Framework      *string `json:"framework,omitempty"`
	BuildCommand   *string `json:"build_command,omitempty"`
	InstallCommand *string `json:"install_command,omitempty"`
	NodeVersion    *string `json:"node_version,omitempty"`
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	projects, err := h.DB.ListProjects(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list projects"})
		return
	}
	if projects == nil {
		projects = []db.Project{}
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, err := h.DB.GetProject(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get project"})
		return
	}
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate name (used as subdomain)
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !slugRegex.MatchString(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be lowercase alphanumeric with hyphens (e.g., my-app)"})
		return
	}

	if req.GithubRepo == "" || req.GithubOwner == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "github_repo and github_owner are required"})
		return
	}

	// Defaults
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}
	framework := req.Framework
	if framework == "" {
		framework = "nextjs"
	}
	buildCmd := req.BuildCommand
	if buildCmd == "" {
		buildCmd = "bun run build"
	}
	installCmd := req.InstallCommand
	if installCmd == "" {
		installCmd = "bun install"
	}
	nodeVersion := req.NodeVersion
	if nodeVersion == "" {
		nodeVersion = "22"
	}

	project := &db.Project{
		Name:           name,
		GithubRepo:     req.GithubRepo,
		GithubOwner:    req.GithubOwner,
		Branch:         branch,
		UserID:         claims.UserID,
		Framework:      framework,
		BuildCommand:   buildCmd,
		InstallCommand: installCmd,
		NodeVersion:    nodeVersion,
		Status:         "active",
	}

	if err := h.DB.CreateProject(project); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "project name already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create project"})
		return
	}

	// Auto-create preview domain
	previewDomain := &db.Domain{
		ProjectID:   project.ID,
		DomainName:  name + ".preview.example.com",
		Environment: "preview",
		IsAuto:      true,
		SSLStatus:   "active", // wildcard cert covers this
	}
	h.DB.CreateDomain(previewDomain)

	writeJSON(w, http.StatusCreated, project)
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.DB.GetProject(id)
	if err != nil || project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name != nil {
		name := strings.ToLower(strings.TrimSpace(*req.Name))
		if !slugRegex.MatchString(name) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid name"})
			return
		}
		project.Name = name
	}
	if req.Branch != nil {
		project.Branch = *req.Branch
	}
	if req.Framework != nil {
		project.Framework = *req.Framework
	}
	if req.BuildCommand != nil {
		project.BuildCommand = *req.BuildCommand
	}
	if req.InstallCommand != nil {
		project.InstallCommand = *req.InstallCommand
	}
	if req.NodeVersion != nil {
		project.NodeVersion = *req.NodeVersion
	}

	if err := h.DB.UpdateProject(project); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update project"})
		return
	}

	writeJSON(w, http.StatusOK, project)
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, err := h.DB.GetProject(id)
	if err != nil || project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	if err := h.DB.DeleteProject(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete project"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListGithubRepos lists the authenticated user's GitHub repos.
func (h *ProjectHandler) ListGithubRepos(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())

	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	// Decrypt GitHub token
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt token"})
		return
	}

	client := github.NewClient(token)
	repos, err := client.ListRepos(1, 100)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch repos from GitHub"})
		return
	}

	writeJSON(w, http.StatusOK, repos)
}

// ListGithubBranches lists branches for a repository.
func (h *ProjectHandler) ListGithubBranches(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")

	if owner == "" || repo == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "owner and repo query params required"})
		return
	}

	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt token"})
		return
	}

	client := github.NewClient(token)
	branches, err := client.ListBranches(owner, repo)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch branches"})
		return
	}

	writeJSON(w, http.StatusOK, branches)
}
