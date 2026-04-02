package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
)

type DomainHandler struct {
	DB      *db.DB
	Manager *domain.Manager
	Audit   *audit.Recorder
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

	// Check if domain already exists
	existing, _ := h.DB.GetDomainByName(req.Domain)
	if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "domain already in use"})
		return
	}

	d := &db.Domain{
		ProjectID:   projectID,
		DomainName:  req.Domain,
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

	// Verify DNS
	verified, err := h.Manager.VerifyDomainDNS(target.DomainName)
	if err != nil {
		log.Printf("DNS verification error for %s: %v", target.DomainName, err)
	}

	h.DB.UpdateDomainDNS(domainID, verified)

	if !verified {
		h.DB.UpdateDomainSSL(domainID, "pending", target.SSLExpiresAt)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dns_verified": false,
			"message":      "DNS A record does not point to " + h.Manager.VPSHost,
		})
		claims := auth.GetClaims(r.Context())
		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "domain.verify",
			ResourceType: "domain",
			ResourceID:   domainID,
			ProjectID:    projectID,
			Metadata: map[string]any{
				"domain":       target.DomainName,
				"dns_verified": false,
				"ssl_status":   "pending",
			},
		})
		return
	}

	err = h.Manager.ProvisionDomain(domain.ProvisionConfig{
		ProjectID:     project.ID,
		ProjectName:   project.Name,
		Domain:        target.DomainName,
		Environment:   target.Environment,
		ContainerName: "deployik-" + project.Name + "-" + target.Environment,
	}, false)
	if err != nil {
		log.Printf("SSL cert request failed for %s: %v", target.DomainName, err)
		h.DB.UpdateDomainSSL(domainID, "error", target.SSLExpiresAt)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dns_verified": true,
			"ssl_status":   "error",
			"message":      "DNS verified but SSL cert issuance failed",
		})
		claims := auth.GetClaims(r.Context())
		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "domain.verify",
			ResourceType: "domain",
			ResourceID:   domainID,
			ProjectID:    projectID,
			Metadata: map[string]any{
				"domain":       target.DomainName,
				"dns_verified": true,
				"ssl_status":   "error",
			},
		})
		return
	}

	h.DB.UpdateDomainSSL(domainID, "active", target.SSLExpiresAt)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"dns_verified": true,
		"ssl_status":   "active",
		"message":      "Domain verified and SSL cert active",
	})
	claims := auth.GetClaims(r.Context())
	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "domain.verify",
		ResourceType: "domain",
		ResourceID:   domainID,
		ProjectID:    projectID,
		Metadata: map[string]any{
			"domain":       target.DomainName,
			"dns_verified": true,
			"ssl_status":   "active",
		},
	})
}
