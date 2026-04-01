package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
	"github.com/LEFTEQ/lovinka-deployik/internal/projectconfig"
)

type ProjectHandler struct {
	DB        *db.DB
	Docker    *build.DockerClient
	Manager   *domain.Manager
	Encryptor *crypto.Encryptor
	Audit     *audit.Recorder
}

var slugRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

type createProjectRequest struct {
	OrganizationID  string `json:"organization_id"`
	Name            string `json:"name"`
	GithubRepo      string `json:"github_repo"`
	GithubOwner     string `json:"github_owner"`
	Branch          string `json:"branch"`
	Framework       string `json:"framework"`
	PackageManager  string `json:"package_manager"`
	RootDirectory   string `json:"root_directory"`
	OutputDirectory string `json:"output_directory"`
	BuildCommand    string `json:"build_command"`
	InstallCommand  string `json:"install_command"`
	NodeVersion     string `json:"node_version"`
}

type updateProjectRequest struct {
	Name            *string `json:"name,omitempty"`
	Branch          *string `json:"branch,omitempty"`
	Framework       *string `json:"framework,omitempty"`
	PackageManager  *string `json:"package_manager,omitempty"`
	RootDirectory   *string `json:"root_directory,omitempty"`
	OutputDirectory *string `json:"output_directory,omitempty"`
	BuildCommand    *string `json:"build_command,omitempty"`
	InstallCommand  *string `json:"install_command,omitempty"`
	NodeVersion     *string `json:"node_version,omitempty"`
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	organizationID := strings.TrimSpace(r.URL.Query().Get("organization_id"))
	if organizationID != "" {
		organization, err := h.DB.GetOrganizationForUser(organizationID, claims.UserID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load organization"})
			return
		}
		if organization == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "organization not found"})
			return
		}
	}

	projects, err := h.DB.ListProjects(claims.UserID, organizationID)
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
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
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

	organizationID, err := h.resolveCreateOrganizationID(claims.UserID, strings.TrimSpace(req.OrganizationID))
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "organization not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	project := &db.Project{
		OrganizationID:  organizationID,
		Name:            name,
		GithubRepo:      req.GithubRepo,
		GithubOwner:     req.GithubOwner,
		Branch:          strings.TrimSpace(req.Branch),
		UserID:          claims.UserID,
		Framework:       req.Framework,
		PackageManager:  req.PackageManager,
		RootDirectory:   req.RootDirectory,
		OutputDirectory: req.OutputDirectory,
		BuildCommand:    req.BuildCommand,
		InstallCommand:  req.InstallCommand,
		NodeVersion:     req.NodeVersion,
		Status:          "active",
	}
	if project.Branch == "" {
		project.Branch = "main"
	}
	if err := projectconfig.ApplyProjectDefaults(project); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
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
		SSLStatus:   "pending",
	}
	h.DB.CreateDomain(previewDomain)

	writeJSON(w, http.StatusCreated, project)
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "project.create",
		ResourceType: "project",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"organization_id": project.OrganizationID,
			"name":            project.Name,
			"github_owner":    project.GithubOwner,
			"github_repo":     project.GithubRepo,
		},
	})
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
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
		project.Branch = strings.TrimSpace(*req.Branch)
	}
	if req.Framework != nil {
		project.Framework = *req.Framework
	}
	if req.PackageManager != nil {
		project.PackageManager = *req.PackageManager
	}
	if req.RootDirectory != nil {
		project.RootDirectory = *req.RootDirectory
	}
	if req.OutputDirectory != nil {
		project.OutputDirectory = *req.OutputDirectory
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
	if err := projectconfig.ApplyProjectDefaults(project); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.DB.UpdateProject(project); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update project"})
		return
	}

	writeJSON(w, http.StatusOK, project)
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "project.update",
		ResourceType: "project",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"name":            project.Name,
			"branch":          project.Branch,
			"framework":       project.Framework,
			"package_manager": project.PackageManager,
			"root_directory":  project.RootDirectory,
		},
	})
}

func (h *ProjectHandler) resolveCreateOrganizationID(userID, organizationID string) (string, error) {
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

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	domains, err := h.DB.ListDomains(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load project domains"})
		return
	}

	h.cleanupProjectRuntime(project, domains)

	if err := h.DB.DeleteAllDomainsForProject(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to release project domains"})
		return
	}

	if err := h.DB.DeleteProject(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete project"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "project.delete",
		ResourceType: "project",
		ResourceID:   id,
		ProjectID:    id,
	})
}

func (h *ProjectHandler) cleanupProjectRuntime(project *db.Project, domains []db.Domain) {
	if project == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, environment := range []string{"preview", "production"} {
		if h.Docker == nil {
			break
		}
		containerName := fmt.Sprintf("deployik-%s-%s", project.Name, environment)
		containerID, exists := h.Docker.ContainerExists(ctx, containerName)
		if !exists {
			continue
		}
		if err := h.Docker.StopContainer(ctx, containerID); err != nil {
			log.Printf("Warning: failed to stop deleted project container %s: %v", containerName, err)
		}
	}

	if h.Manager == nil || len(domains) == 0 {
		return
	}

	reloadNeeded := false
	for _, domain := range domains {
		if err := h.Manager.RemoveDomain(domain.DomainName); err != nil {
			log.Printf("Warning: failed to remove nginx config for deleted project domain %s: %v", domain.DomainName, err)
			continue
		}
		reloadNeeded = true
	}

	if reloadNeeded {
		if err := h.Manager.ReloadNginx(); err != nil {
			log.Printf("Warning: failed to reload nginx after deleting project %s: %v", project.Name, err)
		}
	}
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
