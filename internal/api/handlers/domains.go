package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

type DomainHandler struct {
	DB      *db.DB
	Manager *domain.Manager
	Hub     *ws.Hub
	Audit   *audit.Recorder
	// verifying tracks in-flight domain verifications per project to prevent concurrent runs.
	verifying sync.Map // map[projectID]domainID
}

type addDomainRequest struct {
	Domain      string `json:"domain"`
	Environment string `json:"environment"`
}

type updateDomainRequest struct {
	Environment *string `json:"environment,omitempty"`
	IsPrimary   *bool   `json:"is_primary,omitempty"`
}

func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
		return
	}
	domains, err := h.DB.ListDomains(projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list domains"})
		return
	}
	if domains == nil {
		domains = []db.Domain{}
	}
	writeJSON(w, http.StatusOK, domains)
}

func (h *DomainHandler) Add(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	var req addDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain is required"})
		return
	}

	// Validate BEFORE any normalization writes into templates (nginx config,
	// logs, SSL requests). Rejects CRLF, semicolons, wildcards, etc.
	cleanDomain, err := domain.ValidateHostname(req.Domain)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	env := req.Environment
	if env == "" {
		env = "production"
	}
	if env != "preview" && env != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}

	previewInstanceID := ""
	if env == "preview" {
		instance, _, err := ensurePreviewTarget(h.DB, project, project.Branch)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare preview target"})
			return
		}
		previewInstanceID = instance.ID
	}

	plan := domain.ResolveVariantPlan(cleanDomain, env)
	if plan.CanonicalDomain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain is required"})
		return
	}

	// Check if any managed hostname is already claimed.
	for _, hostname := range plan.AllDomains() {
		existing, _ := h.DB.GetDomainByName(hostname)
		if existing != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "domain already in use"})
			return
		}
	}

	d := &db.Domain{
		ProjectID:         projectID,
		PreviewInstanceID: previewInstanceID,
		DomainName:        plan.CanonicalDomain,
		Environment:       env,
		IsAuto:            false,
		SSLStatus:         "pending",
	}

	if err := h.DB.CreateDomain(d); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to add domain"})
		return
	}

	writeJSON(w, http.StatusCreated, d)
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "domain.add",
		ResourceType: "domain",
		ResourceID:   d.ID,
		ProjectID:    projectID,
		Metadata: map[string]any{
			"domain":      d.DomainName,
			"environment": d.Environment,
		},
	})
}

func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	if _, _, ok := loadAuthorizedProject(w, r, h.DB, projectID); !ok {
		return
	}
	domainID := chi.URLParam(r, "did")

	// Get domain to find its name for nginx cleanup
	domains, _ := h.DB.ListDomains(projectID)
	var domainName string
	found := false
	for _, d := range domains {
		if d.ID == domainID {
			domainName = d.DomainName
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}

	if err := h.DB.DeleteDomainForProject(projectID, domainID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete domain"})
		return
	}

	// Remove nginx config
	if domainName != "" && h.Manager != nil {
		if err := h.Manager.RemoveDomain(domainName); err != nil {
			log.Printf("Warning: failed to remove nginx config for %s: %v", domainName, err)
		} else if err := h.Manager.ReloadProxy(); err != nil {
			log.Printf("Warning: failed to reload proxy after removing %s: %v", domainName, err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "domain.delete",
		ResourceType: "domain",
		ResourceID:   domainID,
		ProjectID:    projectID,
		Metadata: map[string]any{
			"domain": domainName,
		},
	})
}

func (h *DomainHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	domainID := chi.URLParam(r, "did")

	project, claims, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	target, err := h.DB.GetDomainByID(domainID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load domain"})
		return
	}
	if target == nil || target.ProjectID != projectID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}
	if target.IsAuto {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "auto domains cannot be modified"})
		return
	}

	var req updateDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Environment == nil && req.IsPrimary == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing to update"})
		return
	}
	if req.IsPrimary != nil && !*req.IsPrimary {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "is_primary can only be set to true"})
		return
	}

	if req.Environment != nil && *req.Environment != target.Environment {
		newEnv := *req.Environment
		if newEnv != "preview" && newEnv != "production" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
			return
		}
		previewInstanceID := ""
		if newEnv == "preview" {
			instance, _, err := ensurePreviewTarget(h.DB, project, project.Branch)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare preview target"})
				return
			}
			previewInstanceID = instance.ID
		}

		if _, loaded := h.verifying.LoadOrStore(projectID, domainID); loaded {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a verification is already in progress for this project"})
			return
		}
		defer h.verifying.Delete(projectID)

		oldEnv := target.Environment
		if err := h.DB.UpdateDomainEnvironment(domainID, newEnv, previewInstanceID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update environment"})
			return
		}

		target.Environment = newEnv
		target.PreviewInstanceID = previewInstanceID
		target.IsPrimary = false

		if h.Manager != nil && target.SSLStatus == "active" {
			plan := domain.ResolveVariantPlan(target.DomainName, newEnv)
			var previewInstance *db.PreviewInstance
			if newEnv == "preview" && previewInstanceID != "" {
				previewInstance, _ = h.DB.GetPreviewInstanceByID(previewInstanceID)
			}
			if err := h.Manager.ProvisionDomain(domain.ProvisionConfig{
				ProjectID:      project.ID,
				ProjectName:    project.Name,
				Domain:         plan.CanonicalDomain,
				RedirectDomain: plan.RedirectDomain,
				SSLDomains:     plan.AllDomains(),
				Environment:    newEnv,
				ContainerName:  db.DeploymentContainerName(project.Name, newEnv, previewInstance),
				Port:           project.Port,
			}, false, nil); err != nil {
				log.Printf("update domain: re-provision %s for %s: %v", target.DomainName, newEnv, err)
			}
		}

		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "domain.move",
			ResourceType: "domain",
			ResourceID:   domainID,
			ProjectID:    projectID,
			Metadata: map[string]any{
				"domain": target.DomainName,
				"from":   oldEnv,
				"to":     newEnv,
			},
		})
	}

	if req.IsPrimary != nil && *req.IsPrimary {
		if err := h.DB.SetDomainPrimary(projectID, target.Environment, domainID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set primary"})
			return
		}

		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "domain.set_primary",
			ResourceType: "domain",
			ResourceID:   domainID,
			ProjectID:    projectID,
			Metadata: map[string]any{
				"domain":      target.DomainName,
				"environment": target.Environment,
			},
		})
	}

	fresh, err := h.DB.GetDomainByID(domainID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load updated domain"})
		return
	}
	writeJSON(w, http.StatusOK, fresh)
}

func (h *DomainHandler) Verify(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "did")
	projectID := chi.URLParam(r, "id")

	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	// Find the domain
	domains, _ := h.DB.ListDomains(projectID)
	var target *db.Domain
	for _, d := range domains {
		if d.ID == domainID {
			target = &d
			break
		}
	}

	if target == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}

	// Prevent concurrent verifications per project
	if _, loaded := h.verifying.LoadOrStore(projectID, domainID); loaded {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "A domain verification is already in progress for this project",
		})
		return
	}

	claims := auth.GetClaims(r.Context())

	// Return immediately — provisioning runs in background
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "verifying",
		"domain_id": domainID,
	})

	// Run provisioning in background goroutine
	go func() {
		defer h.verifying.Delete(projectID)

		start := time.Now()
		lineNum := 0
		topic := "domain:" + domainID

		// Drop any leftover events from a previous verify session on this
		// domain so a late-connecting WebSocket doesn't replay stale logs.
		h.Hub.ResetBuffer(topic)

		emit := func(step, status, content string) {
			lineNum++
			h.Hub.Publish(ws.LogLine{
				DeploymentID: topic,
				LineNumber:   lineNum,
				Content:      content,
				Stream:       step + ":" + status,
			})
		}

		// DNS verification
		plan := domain.ResolveVariantPlan(target.DomainName, target.Environment)
		var missing []string

		for _, hostname := range plan.AllDomains() {
			emit("dns", "running", fmt.Sprintf("Checking DNS for %s...", hostname))
			verified, err := h.Manager.VerifyDomainDNS(hostname)
			if err != nil {
				log.Printf("DNS verification error for %s: %v", hostname, err)
				emit("dns", "error", fmt.Sprintf("DNS lookup failed for %s: %v", hostname, err))
				missing = append(missing, hostname)
				continue
			}
			if !verified {
				emit("dns", "error", fmt.Sprintf("%s does not point to %s", hostname, h.Manager.VPSHost))
				missing = append(missing, hostname)
			} else {
				emit("dns", "success", fmt.Sprintf("%s → %s", hostname, h.Manager.VPSHost))
			}
		}

		if len(missing) > 0 {
			h.DB.UpdateDomainDNS(domainID, false)
			h.DB.UpdateDomainSSL(domainID, "pending", target.SSLExpiresAt)
			msg := fmt.Sprintf("Point %s to %s before verifying SSL", strings.Join(missing, ", "), h.Manager.VPSHost)
			emit("done", "error", msg)
			h.Audit.Record(audit.Entry{
				UserID: claims.UserID, Action: "domain.verify", ResourceType: "domain",
				ResourceID: domainID, ProjectID: projectID,
				Metadata: map[string]any{"domain": target.DomainName, "dns_verified": false, "ssl_status": "pending"},
			})
			return
		}

		h.DB.UpdateDomainDNS(domainID, true)

		// SSL + Nginx provisioning via ProvisionDomain with logger
		var previewInstance *db.PreviewInstance
		if target.Environment == "preview" && target.PreviewInstanceID != "" {
			previewInstance, _ = h.DB.GetPreviewInstanceByID(target.PreviewInstanceID)
		}
		if err := h.Manager.ProvisionDomain(domain.ProvisionConfig{
			ProjectID:      project.ID,
			ProjectName:    project.Name,
			Domain:         plan.CanonicalDomain,
			RedirectDomain: plan.RedirectDomain,
			SSLDomains:     plan.AllDomains(),
			Environment:    target.Environment,
			ContainerName:  db.DeploymentContainerName(project.Name, target.Environment, previewInstance),
			Port:           project.Port,
		}, false, emit); err != nil {
			log.Printf("SSL cert request failed for %s: %v", target.DomainName, err)
			h.DB.UpdateDomainSSL(domainID, "error", target.SSLExpiresAt)
			durationMs := time.Since(start).Milliseconds()
			emit("done", "error", fmt.Sprintf("DNS verified but SSL/nginx provisioning failed (took %dms)", durationMs))
			h.Audit.Record(audit.Entry{
				UserID: claims.UserID, Action: "domain.verify", ResourceType: "domain",
				ResourceID: domainID, ProjectID: projectID,
				Metadata: map[string]any{"domain": target.DomainName, "dns_verified": true, "ssl_status": "error"},
			})
			return
		}

		h.DB.UpdateDomainSSL(domainID, "active", target.SSLExpiresAt)
		durationMs := time.Since(start).Milliseconds()
		emit("done", "success", fmt.Sprintf("Domain verified and live (took %dms)", durationMs))
		h.Audit.Record(audit.Entry{
			UserID: claims.UserID, Action: "domain.verify", ResourceType: "domain",
			ResourceID: domainID, ProjectID: projectID,
			Metadata: map[string]any{"domain": target.DomainName, "dns_verified": true, "ssl_status": "active"},
		})
	}()
}
