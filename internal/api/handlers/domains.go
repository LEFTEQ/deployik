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

	_, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
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

	env := req.Environment
	if env == "" {
		env = "production"
	}

	plan := domain.ResolveVariantPlan(req.Domain, env)
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
		ProjectID:   projectID,
		DomainName:  plan.CanonicalDomain,
		Environment: env,
		IsAuto:      false,
		SSLStatus:   "pending",
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
		} else if err := h.Manager.ReloadNginx(); err != nil {
			log.Printf("Warning: failed to reload nginx after removing %s: %v", domainName, err)
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
		provisionLogger := func(step, status, content string) {
			emit(step, status, content)
		}

		if err := h.Manager.ProvisionDomain(domain.ProvisionConfig{
			ProjectID:      project.ID,
			ProjectName:    project.Name,
			Domain:         plan.CanonicalDomain,
			RedirectDomain: plan.RedirectDomain,
			SSLDomains:     plan.AllDomains(),
			Environment:    target.Environment,
			ContainerName:  "deployik-" + project.Name + "-" + target.Environment,
		}, false, provisionLogger); err != nil {
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
