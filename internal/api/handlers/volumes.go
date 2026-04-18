package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/errdefs"
	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// VolumeHandler manages Docker data volumes for projects.
type VolumeHandler struct {
	DB     *db.DB
	Docker *build.DockerClient
	Audit  *audit.Recorder
}

type volumeInfo struct {
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Exists      bool   `json:"exists"`
	SizeBytes   int64  `json:"size_bytes"`
	CreatedAt   string `json:"created_at,omitempty"`
	MountPath   string `json:"mount_path"`
	InUse       bool   `json:"in_use"`
}

// volumeName is the canonical Docker volume name for a project-environment.
// Keep in sync with the pipeline's volume binding logic.
func volumeName(project *db.Project, env string) string {
	return fmt.Sprintf("deployik-%s-%s-data", project.Name, env)
}

func (h *VolumeHandler) List(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	ctx := r.Context()

	// One /system/df call covers both environments with true on-disk sizes.
	// VolumeInspect/VolumeList leave UsageData nil, so per-volume inspect
	// would always report 0 bytes.
	var usage map[string]*volume.Volume
	if h.Docker != nil {
		u, err := h.Docker.VolumesDiskUsage(ctx)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "docker disk usage failed: " + err.Error()})
			return
		}
		usage = u
	}

	result := make([]volumeInfo, 0, 2)
	for _, env := range []string{"preview", "production"} {
		name := volumeName(project, env)
		info := volumeInfo{
			Environment: env,
			Name:        name,
			MountPath:   project.DataMountPath,
		}
		if v, ok := usage[name]; ok && v != nil {
			info.Exists = true
			info.CreatedAt = v.CreatedAt
			if v.UsageData != nil && v.UsageData.Size >= 0 {
				info.SizeBytes = v.UsageData.Size
			}
			// Best-effort: if the container is running, the volume is in use.
			containerName := fmt.Sprintf("deployik-%s-%s", project.Name, env)
			if h.Docker != nil {
				if _, running := h.Docker.ContainerExists(ctx, containerName); running {
					info.InUse = true
				}
			}
		}
		result = append(result, info)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *VolumeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	env := chi.URLParam(r, "env")
	if env != "preview" && env != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "env must be preview or production"})
		return
	}

	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	name := volumeName(project, env)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if err := h.Docker.RemoveVolume(ctx, name); err != nil {
		writeJSON(w, volumeErrorStatus(err), map[string]string{"error": volumeErrorMessage(err, "remove")})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "volume.delete",
		ResourceType: "volume",
		ResourceID:   name,
		ProjectID:    id,
		Metadata:     map[string]any{"environment": env},
	})
}

func (h *VolumeHandler) Recreate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	env := chi.URLParam(r, "env")
	if env != "preview" && env != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "env must be preview or production"})
		return
	}

	project, _, ok := loadAuthorizedProject(w, r, h.DB, id)
	if !ok {
		return
	}

	name := volumeName(project, env)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// VolumeCreate is idempotent — if we don't successfully remove first, any
	// existing volume keeps its data and we'd report false success. Treat
	// NotFound as "already gone" and propagate every other error (especially
	// Conflict, which means a running container still mounts it).
	if err := h.Docker.RemoveVolume(ctx, name); err != nil && !errdefs.IsNotFound(err) {
		writeJSON(w, volumeErrorStatus(err), map[string]string{"error": volumeErrorMessage(err, "remove")})
		return
	}

	if err := h.Docker.EnsureVolume(ctx, name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create volume: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recreated", "name": name})

	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "volume.recreate",
		ResourceType: "volume",
		ResourceID:   name,
		ProjectID:    id,
		Metadata:     map[string]any{"environment": env},
	})
}

// volumeErrorStatus maps a Docker error into an HTTP status. Conflict (volume
// in use) is the one we specifically want to surface so the UI can ask the
// user to stop the deployment first.
func volumeErrorStatus(err error) int {
	switch {
	case errdefs.IsNotFound(err):
		return http.StatusNotFound
	case errdefs.IsConflict(err), errdefs.IsForbidden(err):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func volumeErrorMessage(err error, action string) string {
	if errdefs.IsConflict(err) || errdefs.IsForbidden(err) {
		return "volume is in use by a running container — stop the deployment first"
	}
	return fmt.Sprintf("failed to %s volume: %s", action, err.Error())
}
