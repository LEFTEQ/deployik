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

	"github.com/LEFTEQ/lovinka-deployik/internal/analytics"
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
	DB         *db.DB
	Docker     *build.DockerClient
	Manager    *domain.Manager
	Encryptor  *crypto.Encryptor
	Audit      *audit.Recorder
	Analytics  *analytics.Service
	DevMode    bool
	Pipeline   *build.Pipeline // used for initial deploy on project creation
	WebhookURL string          // used for auto-build webhook on project creation
}

var slugRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

type createProjectRequest struct {
	OrganizationID    string `json:"organization_id"`
	Name              string `json:"name"`
	GithubRepo        string `json:"github_repo"`
	GithubOwner       string `json:"github_owner"`
	Branch            string `json:"branch"`
	Framework         string `json:"framework"`
	PackageManager    string `json:"package_manager"`
	RootDirectory     string `json:"root_directory"`
	OutputDirectory   string `json:"output_directory"`
	BuildCommand      string `json:"build_command"`
	InstallCommand    string `json:"install_command"`
	NodeVersion       string `json:"node_version"`
	Port              int    `json:"port"`
	HostNetworkAccess bool   `json:"host_network_access"`
	DataVolumeEnabled bool   `json:"data_volume_enabled"`
	DataMountPath     string `json:"data_mount_path"`
}

type updateProjectRequest struct {
	Name              *string `json:"name,omitempty"`
	Branch            *string `json:"branch,omitempty"`
	Framework         *string `json:"framework,omitempty"`
	PackageManager    *string `json:"package_manager,omitempty"`
	RootDirectory     *string `json:"root_directory,omitempty"`
	OutputDirectory   *string `json:"output_directory,omitempty"`
	BuildCommand      *string `json:"build_command,omitempty"`
	InstallCommand    *string `json:"install_command,omitempty"`
	NodeVersion       *string `json:"node_version,omitempty"`
	Port              *int    `json:"port,omitempty"`
	HostNetworkAccess *bool   `json:"host_network_access,omitempty"`
	DataVolumeEnabled *bool   `json:"data_volume_enabled,omitempty"`
	DataMountPath     *string `json:"data_mount_path,omitempty"`
}

// validateProjectPort rejects obviously-invalid ports. 0 is treated as "unset"
// by the caller and defaulted to 3000 at persist time.
func validateProjectPort(port int) error {
	if port == 0 {
		return nil
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
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

	projects, err := h.DB.ListProjectsWithLatestDeployment(claims.UserID, organizationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list projects"})
		return
	}
	if projects == nil {
		projects = []db.ProjectWithLatestDeployment{}
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

	if err := validateProjectPort(req.Port); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
		OrganizationID:    organizationID,
		Name:              name,
		GithubRepo:        req.GithubRepo,
		GithubOwner:       req.GithubOwner,
		Branch:            strings.TrimSpace(req.Branch),
		UserID:            claims.UserID,
		Framework:         req.Framework,
		PackageManager:    req.PackageManager,
		RootDirectory:     req.RootDirectory,
		OutputDirectory:   req.OutputDirectory,
		BuildCommand:      req.BuildCommand,
		InstallCommand:    req.InstallCommand,
		NodeVersion:       req.NodeVersion,
		Port:              req.Port,
		HostNetworkAccess: req.HostNetworkAccess,
		DataVolumeEnabled: req.DataVolumeEnabled,
		DataMountPath:     req.DataMountPath,
		Status:            "active",
	}
	if project.DataMountPath == "" {
		project.DataMountPath = "/app/data"
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
		IsPrimary:   true,
		SSLStatus:   "pending",
	}
	h.DB.CreateDomain(previewDomain)

	h.syncProjectAnalytics(project, []db.Domain{*previewDomain})

	// Best-effort: provision GitHub webhook + auto-build config.
	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		log.Printf("Warning: could not load user for post-create setup (project %s): %v", project.ID, err)
	} else {
		h.setupAutoBuildBestEffort(r.Context(), project, user)
		h.dispatchInitialDeployBestEffort(r.Context(), project, user)
	}

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
		// Volume and container names are derived from project.Name; renaming
		// would silently orphan any existing data volume and leave the running
		// container on the old name. Block renames when a volume is attached;
		// the user can disable the volume (and accept data loss) first, or we
		// can revisit this once volumes are keyed by project.ID.
		// TODO(pr1-followup): re-key volume naming by project.ID so renames are safe.
		if name != project.Name && project.DataVolumeEnabled {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "cannot rename project while a persistent data volume is attached — disable the volume first (this will abandon its data)",
			})
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
	if req.Port != nil {
		if err := validateProjectPort(*req.Port); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		port := *req.Port
		if port == 0 {
			port = 3000
		}
		project.Port = port
	}
	if req.HostNetworkAccess != nil {
		project.HostNetworkAccess = *req.HostNetworkAccess
	}
	if req.DataVolumeEnabled != nil {
		project.DataVolumeEnabled = *req.DataVolumeEnabled
	}
	if req.DataMountPath != nil {
		mp := strings.TrimSpace(*req.DataMountPath)
		if mp == "" {
			mp = "/app/data"
		}
		project.DataMountPath = mp
	}
	if err := projectconfig.ApplyProjectDefaults(project); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.DB.UpdateProject(project); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update project"})
		return
	}

	if domains, err := h.DB.ListDomains(project.ID); err != nil {
		log.Printf("Warning: failed to load domains for analytics sync on project %s: %v", project.ID, err)
	} else {
		h.syncProjectAnalytics(project, domains)
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

	if h.Analytics != nil {
		if err := h.Analytics.DeleteProjectAnalytics(context.Background(), id); err != nil {
			log.Printf("Warning: failed to remove analytics for deleted project %s: %v", id, err)
		}
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

func (h *ProjectHandler) syncProjectAnalytics(project *db.Project, domains []db.Domain) {
	if h.Analytics == nil || project == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := h.Analytics.EnsureProject(ctx, project, domains); err != nil {
		log.Printf("Warning: failed to sync analytics for project %s: %v", project.ID, err)
	}
}

// setupAutoBuildBestEffort provisions a GitHub webhook and inserts an
// auto_build_configs row for the project. Failures are logged and silently
// swallowed — the project creation response is unaffected.
func (h *ProjectHandler) setupAutoBuildBestEffort(ctx context.Context, project *db.Project, user *db.User) {
	if h.WebhookURL == "" {
		log.Printf("Warning: WebhookURL not configured; skipping auto-build setup for project %s", project.ID)
		return
	}
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		log.Printf("Warning: failed to decrypt token for auto-build setup (project %s): %v", project.ID, err)
		return
	}
	config, err := provisionWebhook(ctx, h.DB, h.Encryptor, project, token, h.WebhookURL, project.Branch, "*", false)
	if err != nil {
		log.Printf("Warning: auto-build webhook setup failed for project %s: %v", project.ID, err)
		return
	}
	h.Audit.Record(audit.Entry{
		UserID:       user.ID,
		Action:       "auto_build_create",
		ResourceType: "auto_build_config",
		ResourceID:   config.ID,
		ProjectID:    project.ID,
		Metadata: map[string]any{
			"source": "project_create",
		},
	})
}

// dispatchInitialDeployBestEffort creates a queued preview deployment and
// hands it to the pipeline. Failures are logged and silently swallowed.
func (h *ProjectHandler) dispatchInitialDeployBestEffort(ctx context.Context, project *db.Project, user *db.User) {
	if h.Pipeline == nil {
		log.Printf("Warning: Pipeline not configured; skipping initial deploy for project %s", project.ID)
		return
	}
	token, err := h.Encryptor.Decrypt(user.GithubToken)
	if err != nil {
		log.Printf("Warning: failed to decrypt token for initial deploy (project %s): %v", project.ID, err)
		return
	}
	deployment := &db.Deployment{
		ProjectID:           project.ID,
		Environment:         "preview",
		Branch:              project.Branch,
		Status:              "queued",
		TriggerSource:       "api",
		TriggeredBy:         user.ID,
		TriggeredByUsername: user.Username,
	}
	if err := h.DB.CreateDeployment(deployment); err != nil {
		log.Printf("Warning: failed to create initial deployment for project %s: %v", project.ID, err)
		return
	}
	h.Pipeline.Dispatch(project, deployment, token, func(line, stream string) {
		log.Printf("[deploy:%s] %s", deployment.ID[:8], line)
	})
	h.Audit.Record(audit.Entry{
		UserID:       user.ID,
		Action:       "deployment_create",
		ResourceType: "deployment",
		ResourceID:   deployment.ID,
		ProjectID:    project.ID,
		DeploymentID: deployment.ID,
		Metadata: map[string]any{
			"source":      "project_create",
			"environment": "preview",
		},
	})
	_ = ctx // ctx available for future use; Dispatch spawns its own goroutine
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
		if err := h.Manager.ReloadProxy(); err != nil {
			log.Printf("Warning: failed to reload proxy after deleting project %s: %v", project.Name, err)
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

	// In dev mode with no GitHub token, return mock repos
	if h.DevMode && user.GithubToken == "" {
		writeJSON(w, http.StatusOK, devMockRepos(user.Username))
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

	// In dev mode with no GitHub token, return mock branches
	if h.DevMode && user.GithubToken == "" {
		writeJSON(w, http.StatusOK, devMockBranches())
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

func devMockRepos(username string) []github.Repo {
	return []github.Repo{
		{
			ID:            1,
			FullName:      username + "/my-portfolio",
			Name:          "my-portfolio",
			Owner:         github.Owner{Login: username, AvatarURL: "https://github.com/identicons/" + username + ".png"},
			Private:       false,
			DefaultBranch: "main",
			Language:      "TypeScript",
			UpdatedAt:     time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		},
		{
			ID:            2,
			FullName:      username + "/landing-page",
			Name:          "landing-page",
			Owner:         github.Owner{Login: username, AvatarURL: "https://github.com/identicons/" + username + ".png"},
			Private:       false,
			DefaultBranch: "main",
			Language:      "JavaScript",
			UpdatedAt:     time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		},
		{
			ID:            3,
			FullName:      username + "/api-service",
			Name:          "api-service",
			Owner:         github.Owner{Login: username, AvatarURL: "https://github.com/identicons/" + username + ".png"},
			Private:       true,
			DefaultBranch: "develop",
			Language:      "Go",
			UpdatedAt:     time.Now().Add(-72 * time.Hour).Format(time.RFC3339),
		},
		{
			ID:            4,
			FullName:      username + "/docs-site",
			Name:          "docs-site",
			Owner:         github.Owner{Login: username, AvatarURL: "https://github.com/identicons/" + username + ".png"},
			Private:       false,
			DefaultBranch: "main",
			Language:      "MDX",
			UpdatedAt:     time.Now().Add(-168 * time.Hour).Format(time.RFC3339),
		},
	}
}

func devMockBranches() []github.Branch {
	return []github.Branch{
		{Name: "main", Commit: struct {
			SHA string `json:"sha"`
		}{SHA: "abc1234567890def"}},
		{Name: "develop", Commit: struct {
			SHA string `json:"sha"`
		}{SHA: "def4567890abc123"}},
		{Name: "feature/auth", Commit: struct {
			SHA string `json:"sha"`
		}{SHA: "fea7890abcdef456"}},
		{Name: "fix/styling", Commit: struct {
			SHA string `json:"sha"`
		}{SHA: "f1x234567890abcd"}},
	}
}
