package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
)

type PreviewInstanceHandler struct {
	DB      *db.DB
	Docker  *build.DockerClient
	Manager *domain.Manager
	Audit   *audit.Recorder
}

func ensurePreviewTarget(database *db.DB, project *db.Project, branch string) (*db.PreviewInstance, []db.Domain, error) {
	instance, err := database.GetOrCreatePreviewInstance(project, branch)
	if err != nil {
		return nil, nil, err
	}
	domains, err := database.EnsurePreviewAutoDomains(project, instance)
	if err != nil {
		return nil, nil, err
	}
	return instance, domains, nil
}

func (h *PreviewInstanceHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	summaries, err := h.DB.ListPreviewInstanceSummaries(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list preview instances"})
		return
	}
	if summaries == nil {
		summaries = []db.PreviewInstanceSummary{}
	}

	h.attachVolumeInfo(r.Context(), project, summaries)

	writeJSON(w, http.StatusOK, summaries)
}

// attachVolumeInfo fills in each branch's isolated data-volume existence + size
// from a single Docker /system/df lookup, so the UI can offer to delete the
// volume only when one is actually attached. It's a no-op (leaving
// VolumeExists=false) when the project has no data volumes or Docker is
// unavailable, mirroring the volumes handler's reliance on /system/df.
func (h *PreviewInstanceHandler) attachVolumeInfo(ctx context.Context, project *db.Project, summaries []db.PreviewInstanceSummary) {
	if h.Docker == nil || !project.DataVolumeEnabled || len(summaries) == 0 {
		return
	}
	usage, err := h.Docker.VolumesDiskUsage(ctx)
	if err != nil {
		log.Printf("Warning: failed to read docker disk usage for preview volumes: %v", err)
		return
	}
	for i := range summaries {
		instance := summaries[i].PreviewInstance
		name := db.DeploymentVolumeName(project.Name, "preview", &instance)
		if v, ok := usage[name]; ok && v != nil {
			summaries[i].VolumeExists = true
			if v.UsageData != nil && v.UsageData.Size >= 0 {
				summaries[i].VolumeSizeBytes = v.UsageData.Size
			}
		}
	}
}

func (h *PreviewInstanceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	instanceID := chi.URLParam(r, "piid")
	project, claims, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	instance, err := h.DB.GetPreviewInstanceByID(instanceID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load preview instance"})
		return
	}
	if instance == nil || instance.ProjectID != projectID || instance.Status == "deleted" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "preview instance not found"})
		return
	}
	if instance.IsDefault {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "default preview cannot be deleted"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	deleteVolume := r.URL.Query().Get("delete_volume") == "1"
	if err := teardownPreviewInstance(ctx, h.DB, h.Docker, h.Manager, project, instance, deleteVolume); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	if h.Audit != nil && claims != nil {
		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "preview_instance.delete",
			ResourceType: "preview_instance",
			ResourceID:   instance.ID,
			ProjectID:    projectID,
			Metadata: map[string]any{
				"branch":        instance.Branch,
				"branch_slug":   instance.BranchSlug,
				"delete_volume": deleteVolume,
			},
		})
	}
}

// teardownPreviewInstance stops the preview container, removes its proxy configs
// and domain rows, soft-deletes the instance, and reloads the proxy if any vhost
// was removed. The container/proxy side is best-effort (logs and continues); an
// error is returned only when a database write fails. Safe to call when pieces
// are already gone, so GitHub webhook redelivery is harmless. Shared by the
// manual delete endpoint and the branch-deletion webhook path.
func teardownPreviewInstance(ctx context.Context, database *db.DB, docker *build.DockerClient, manager *domain.Manager, project *db.Project, instance *db.PreviewInstance, deleteVolume bool) error {
	domains, err := database.ListDomains(project.ID)
	if err != nil {
		return fmt.Errorf("load preview domains: %w", err)
	}

	containerName := db.PreviewContainerName(project.Name, instance)
	if docker != nil {
		if containerID, exists := docker.ContainerExists(ctx, containerName); exists {
			if err := docker.StopContainer(ctx, containerID); err != nil {
				log.Printf("Warning: failed to stop preview container %s: %v", containerName, err)
			}
		}
		if deleteVolume && project.DataVolumeEnabled {
			if err := docker.RemoveVolume(ctx, db.DeploymentVolumeName(project.Name, "preview", instance)); err != nil {
				log.Printf("Warning: failed to remove preview volume for %s: %v", instance.ID, err)
			}
		}
	}

	reloadNeeded := false
	if manager != nil {
		for _, d := range domains {
			if d.PreviewInstanceID != instance.ID {
				continue
			}
			if err := manager.RemoveDomain(d.DomainName); err != nil {
				log.Printf("Warning: failed to remove preview domain config %s: %v", d.DomainName, err)
				continue
			}
			reloadNeeded = true
		}
	}

	if err := database.DeleteDomainsForPreviewInstance(project.ID, instance.ID); err != nil {
		return fmt.Errorf("delete preview domains: %w", err)
	}
	if err := database.DeletePreviewInstance(project.ID, instance.ID); err != nil {
		return fmt.Errorf("delete preview instance: %w", err)
	}

	if reloadNeeded && manager != nil {
		if err := manager.ReloadProxy(); err != nil {
			log.Printf("Warning: failed to reload proxy after deleting preview instance %s: %v", instance.ID, err)
		}
	}
	return nil
}
