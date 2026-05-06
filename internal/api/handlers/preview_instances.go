package handlers

import (
	"context"
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
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
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
	writeJSON(w, http.StatusOK, summaries)
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

	domains, err := h.DB.ListDomains(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load preview domains"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	containerName := db.PreviewContainerName(project.Name, instance)
	if h.Docker != nil {
		if containerID, exists := h.Docker.ContainerExists(ctx, containerName); exists {
			if err := h.Docker.StopContainer(ctx, containerID); err != nil {
				log.Printf("Warning: failed to stop preview container %s: %v", containerName, err)
			}
		}
		if r.URL.Query().Get("delete_volume") == "1" && project.DataVolumeEnabled {
			if err := h.Docker.RemoveVolume(ctx, db.DeploymentVolumeName(project.Name, "preview", instance)); err != nil {
				log.Printf("Warning: failed to remove preview volume for %s: %v", instance.ID, err)
			}
		}
	}

	reloadNeeded := false
	if h.Manager != nil {
		for _, d := range domains {
			if d.PreviewInstanceID != instance.ID {
				continue
			}
			if err := h.Manager.RemoveDomain(d.DomainName); err != nil {
				log.Printf("Warning: failed to remove preview domain config %s: %v", d.DomainName, err)
				continue
			}
			reloadNeeded = true
		}
	}

	if err := h.DB.DeleteDomainsForPreviewInstance(projectID, instance.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete preview domains"})
		return
	}
	if err := h.DB.DeletePreviewInstance(projectID, instance.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete preview instance"})
		return
	}

	if reloadNeeded {
		if err := h.Manager.ReloadProxy(); err != nil {
			log.Printf("Warning: failed to reload proxy after deleting preview instance %s: %v", instance.ID, err)
		}
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
				"delete_volume": r.URL.Query().Get("delete_volume") == "1",
			},
		})
	}
}
