package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
)

// ProtectionHandler handles password protection management and site-auth verification.
type ProtectionHandler struct {
	DB        *db.DB
	Encryptor *crypto.Encryptor
	JWTSecret string // HMAC key for signing site-auth cookies
	Manager   *domain.Manager
	Audit     *audit.Recorder
}

// siteAuthCookieName is the cookie that stores the signed site auth token.
const siteAuthCookieName = "deployik_site_auth"

// siteAuthTTL is the validity duration for a site-auth cookie.
const siteAuthTTL = 24 * time.Hour

// --- Protected endpoints (JWT required) ---

// Get handles GET /api/projects/{id}/protection
func (h *ProtectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, _, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{
		"preview_enabled":    project.PreviewPassword != "",
		"production_enabled": project.ProductionPassword != "",
	})
}

type updateProtectionRequest struct {
	Environment string `json:"environment"`
	Enabled     bool   `json:"enabled"`
}

// Update handles PUT /api/projects/{id}/protection
func (h *ProtectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, claims, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	var req updateProtectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Environment != "preview" && req.Environment != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}

	if req.Enabled {
		plaintext, err := h.generateAndStorePassword(project.ID, req.Environment)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to set password"})
			return
		}

		h.regenerateNginxForEnvironment(project, req.Environment)

		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "protection.enable",
			ResourceType: "project",
			ResourceID:   project.ID,
			ProjectID:    project.ID,
			Metadata:     map[string]any{"environment": req.Environment},
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"environment": req.Environment,
			"enabled":     true,
			"password":    plaintext,
		})
	} else {
		if err := h.DB.ClearProjectPassword(project.ID, req.Environment); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to clear password"})
			return
		}

		h.regenerateNginxForEnvironment(project, req.Environment)

		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "protection.disable",
			ResourceType: "project",
			ResourceID:   project.ID,
			ProjectID:    project.ID,
			Metadata:     map[string]any{"environment": req.Environment},
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"environment": req.Environment,
			"enabled":     false,
		})
	}
}

type regenerateProtectionRequest struct {
	Environment string `json:"environment"`
}

// Regenerate handles POST /api/projects/{id}/protection/regenerate
func (h *ProtectionHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, claims, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	var req regenerateProtectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Environment != "preview" && req.Environment != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}

	plaintext, err := h.generateAndStorePassword(project.ID, req.Environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to regenerate password"})
		return
	}

	h.regenerateNginxForEnvironment(project, req.Environment)

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "protection.regenerate",
		ResourceType: "project",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": req.Environment},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"environment": req.Environment,
		"password":    plaintext,
	})
}

// --- Public endpoints (no JWT) ---

type verifyRequest struct {
	Password string `json:"password"`
}

// Verify handles POST /api/site-auth/verify (called from /_deployik/verify via nginx proxy)
// The project_id and environment are read from headers set by nginx.
func (h *ProtectionHandler) Verify(w http.ResponseWriter, r *http.Request) {
	projectID := r.Header.Get("X-Deployik-Project")
	environment := r.Header.Get("X-Deployik-Environment")

	if projectID == "" || environment == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing project context"})
		return
	}

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Password == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	encryptedPassword, err := h.DB.GetProjectPassword(projectID, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if encryptedPassword == "" {
		// No password set — site is public; auth not needed
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	plaintext, err := h.Encryptor.Decrypt(encryptedPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if !hmac.Equal([]byte(req.Password), []byte(plaintext)) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	// Issue signed site-auth cookie
	expiry := time.Now().Add(siteAuthTTL).Unix()
	cookieValue := signSiteAuth(h.JWTSecret, projectID, environment, expiry)

	http.SetCookie(w, &http.Cookie{
		Name:     siteAuthCookieName,
		Value:    cookieValue,
		Path:     "/",
		MaxAge:   int(siteAuthTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Check handles GET /api/site-auth/check (called internally by nginx auth_request)
func (h *ProtectionHandler) Check(w http.ResponseWriter, r *http.Request) {
	expectedProject := r.Header.Get("X-Deployik-Project")
	expectedEnv := r.Header.Get("X-Deployik-Environment")

	if expectedProject == "" || expectedEnv == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	cookie, err := r.Cookie(siteAuthCookieName)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !verifySiteAuth(h.JWTSecret, cookie.Value, expectedProject, expectedEnv) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- Helpers ---

// generateAndStorePassword creates a random 16-char base64url password, encrypts it, and stores it.
// Returns the plaintext password.
func (h *ProtectionHandler) generateAndStorePassword(projectID, environment string) (string, error) {
	raw := make([]byte, 12) // 12 bytes -> 16 chars base64url (no padding)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)

	encrypted, err := h.Encryptor.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("encrypt password: %w", err)
	}

	if err := h.DB.SetProjectPassword(projectID, environment, encrypted); err != nil {
		return "", err
	}

	return plaintext, nil
}

// regenerateNginxForEnvironment rewrites nginx configs for all active domains in the given environment.
// Errors are logged but not fatal — the password is already stored.
func (h *ProtectionHandler) regenerateNginxForEnvironment(project *db.Project, environment string) {
	if h.Manager == nil {
		return
	}

	// Reload the project to get the current password state after the DB write
	updatedProject, err := h.DB.GetProject(project.ID)
	if err != nil || updatedProject == nil {
		return
	}

	domains, err := h.DB.ListDomains(project.ID)
	if err != nil {
		return
	}

	passwordProtected := (environment == "preview" && updatedProject.PreviewPassword != "") ||
		(environment == "production" && updatedProject.ProductionPassword != "")

	wroteConfig := false
	for _, d := range domains {
		if d.Environment != environment || d.SSLStatus != "active" {
			continue
		}

		plan := domain.ResolveVariantPlan(d.DomainName, d.Environment)
		containerName := fmt.Sprintf("deployik-%s-%s", project.Name, environment)

		_, err := h.Manager.WriteNginxConfig(domain.ProvisionConfig{
			ProjectID:         project.ID,
			ProjectName:       project.Name,
			Domain:            plan.CanonicalDomain,
			RedirectDomain:    plan.RedirectDomain,
			Environment:       d.Environment,
			ContainerName:     containerName,
			PasswordProtected: passwordProtected,
		})
		if err == nil {
			wroteConfig = true
		}
	}

	if wroteConfig {
		_ = h.Manager.ReloadNginx()
	}
}

// signSiteAuth creates an HMAC-SHA256 signed token: "projectID:environment:expiry:signature"
func signSiteAuth(secret, projectID, environment string, expiry int64) string {
	msg := fmt.Sprintf("%s:%s:%d", projectID, environment, expiry)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s:%s:%d:%s", projectID, environment, expiry, sig)
}

// verifySiteAuth validates a cookie value: checks HMAC signature, expiry, project, and environment.
func verifySiteAuth(secret, cookieValue, expectedProject, expectedEnv string) bool {
	// Format: projectID:environment:expiry:signature
	// projectID may contain hyphens but not colons, same for environment
	// Split from the right: last part is signature, third-from-right is expiry
	lastColon := strings.LastIndex(cookieValue, ":")
	if lastColon < 0 {
		return false
	}
	sig := cookieValue[lastColon+1:]
	rest := cookieValue[:lastColon]

	secondLastColon := strings.LastIndex(rest, ":")
	if secondLastColon < 0 {
		return false
	}
	expiryStr := rest[secondLastColon+1:]
	projectAndEnv := rest[:secondLastColon]

	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return false
	}

	if time.Now().Unix() > expiry {
		return false
	}

	// Reconstruct expected message from the parsed parts
	msg := fmt.Sprintf("%s:%d", projectAndEnv, expiry)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return false
	}

	// Validate that the embedded project and environment match the request
	expectedPrefix := fmt.Sprintf("%s:%s", expectedProject, expectedEnv)
	if projectAndEnv != expectedPrefix {
		return false
	}

	return true
}

