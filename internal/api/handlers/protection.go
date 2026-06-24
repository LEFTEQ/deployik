package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
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
	Password    string `json:"password"` // optional custom password; generated when empty
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
		plaintext, custom, err := h.setOrGeneratePassword(project.ID, req.Environment, req.Password)
		if err != nil {
			status, msg := passwordWriteError(err)
			writeJSON(w, status, map[string]string{"error": msg})
			return
		}

		h.regenerateNginxForEnvironment(project, req.Environment)

		h.Audit.Record(audit.Entry{
			UserID:       claims.UserID,
			Action:       "protection.enable",
			ResourceType: "project",
			ResourceID:   project.ID,
			ProjectID:    project.ID,
			Metadata:     map[string]any{"environment": req.Environment, "custom": custom},
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
	Password    string `json:"password"` // optional custom password; generated when empty
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

	plaintext, custom, err := h.setOrGeneratePassword(project.ID, req.Environment, req.Password)
	if err != nil {
		status, msg := passwordWriteError(err)
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}

	h.regenerateNginxForEnvironment(project, req.Environment)

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "protection.regenerate",
		ResourceType: "project",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": req.Environment, "custom": custom},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"environment": req.Environment,
		"password":    plaintext,
	})
}

// RevealPassword handles GET /api/projects/{id}/protection/password?environment=
// Returns the decrypted password for the given environment to an authorized caller.
// This is an explicit, audited action — it is not folded into the protection status read.
func (h *ProtectionHandler) RevealPassword(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")
	project, claims, ok := loadAuthorizedProject(w, r, h.DB, projectID)
	if !ok {
		return
	}

	environment := r.URL.Query().Get("environment")
	if environment != "preview" && environment != "production" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be 'preview' or 'production'"})
		return
	}

	encrypted, err := h.DB.GetProjectPassword(project.ID, environment)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if encrypted == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no password set for this environment"})
		return
	}

	plaintext, err := h.Encryptor.Decrypt(encrypted)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.Audit.Record(audit.Entry{
		UserID:       claims.UserID,
		Action:       "protection.reveal",
		ResourceType: "project",
		ResourceID:   project.ID,
		ProjectID:    project.ID,
		Metadata:     map[string]any{"environment": environment},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"environment": environment,
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

	// Support both JSON (API) and form-encoded (auth page form)
	var req verifyRequest
	isFormPost := strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
	if isFormPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		req.Password = r.FormValue("password")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	if req.Password == "" {
		if isFormPost {
			http.Redirect(w, r, r.Header.Get("Referer")+"?error=1", http.StatusSeeOther)
			return
		}
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
		if isFormPost {
			referer := r.Header.Get("Referer")
			if referer == "" {
				referer = "/"
			}
			http.Redirect(w, r, referer+"?error=1", http.StatusSeeOther)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	// Issue signed site-auth cookie
	expiry := time.Now().Add(siteAuthTTL).Unix()
	cookieValue := signSiteAuth(h.JWTSecret, projectID, environment, expiry)

	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     siteAuthCookieName,
		Value:    cookieValue,
		Path:     "/",
		MaxAge:   int(siteAuthTTL.Seconds()),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})

	// Heal browsers poisoned by a deployed PWA's service worker. The auth page
	// used to be served with status 200, so Workbox-style service workers could
	// precache it as the app shell and keep replaying the password screen from
	// cache even after a successful login (this POST is the only request from
	// such a browser that still reaches the server). "storage" unregisters
	// service workers and clears CacheStorage; "cookies" is deliberately
	// excluded so the auth cookie set above survives.
	w.Header().Set("Clear-Site-Data", `"cache", "storage"`)

	// If this is a form POST (not JSON fetch), redirect back to the site root
	if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		redirectTo := r.Header.Get("Referer")
		if redirectTo == "" {
			redirectTo = "/"
		}
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Check handles GET /api/site-auth/check (called internally by nginx auth_request).
// Accepts either a valid site-auth cookie OR a one-shot bypass token carried in
// the original request URI (forwarded as X-Original-URI by the nginx template).
func (h *ProtectionHandler) Check(w http.ResponseWriter, r *http.Request) {
	expectedProject := r.Header.Get("X-Deployik-Project")
	expectedEnv := r.Header.Get("X-Deployik-Environment")

	if expectedProject == "" || expectedEnv == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if originalURI := r.Header.Get("X-Original-URI"); originalURI != "" {
		if token := auth.ExtractBypassToken(originalURI); token != "" {
			if auth.VerifySiteAuthBypass(h.JWTSecret, token, expectedProject, expectedEnv) {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
	}

	if originalURI := r.Header.Get("X-Original-URI"); originalURI != "" {
		if token := auth.ExtractStaticBypassToken(originalURI); token != "" {
			// One DB read, only on explicit bypass-link requests (rare — ordinary
			// visitors use the cookie path below and never reach this read).
			if nonce, err := h.DB.GetProjectBypassNonce(expectedProject); err == nil && nonce != "" {
				if auth.VerifyStaticBypass(h.JWTSecret, token, expectedProject, expectedEnv, nonce) {
					w.WriteHeader(http.StatusOK)
					return
				}
			}
		}
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

// maxPasswordLength bounds custom passwords. There is intentionally NO minimum
// length — the only requirement is a non-empty value.
const maxPasswordLength = 256

// passwordValidationError marks a rejected custom password (maps to HTTP 400).
type passwordValidationError struct{ msg string }

func (e passwordValidationError) Error() string { return e.msg }

// validateCustomPassword enforces the single rule for a user-supplied password:
// non-empty (an empty password would lock everyone out, since Verify rejects
// empty submissions) and within a sane length bound. No minimum length.
func validateCustomPassword(p string) error {
	if p == "" {
		return passwordValidationError{"password must not be empty"}
	}
	if len(p) > maxPasswordLength {
		return passwordValidationError{fmt.Sprintf("password must be at most %d characters", maxPasswordLength)}
	}
	return nil
}

// setOrGeneratePassword stores a validated custom password when one is provided,
// otherwise generates a random one. Returns the plaintext and whether it was custom.
func (h *ProtectionHandler) setOrGeneratePassword(projectID, environment, custom string) (plaintext string, isCustom bool, err error) {
	if custom == "" {
		plaintext, err = h.generateAndStorePassword(projectID, environment)
		return plaintext, false, err
	}
	if err = validateCustomPassword(custom); err != nil {
		return "", true, err
	}
	encrypted, encErr := h.Encryptor.Encrypt(custom)
	if encErr != nil {
		return "", true, fmt.Errorf("encrypt password: %w", encErr)
	}
	if err = h.DB.SetProjectPassword(projectID, environment, encrypted); err != nil {
		return "", true, err
	}
	return custom, true, nil
}

// passwordWriteError maps a setOrGeneratePassword error to an HTTP status + client message.
func passwordWriteError(err error) (int, string) {
	var ve passwordValidationError
	if errors.As(err, &ve) {
		return http.StatusBadRequest, ve.msg
	}
	return http.StatusInternalServerError, "failed to set password"
}

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
		var previewInstance *db.PreviewInstance
		if d.Environment == "preview" && d.PreviewInstanceID != "" {
			previewInstance, _ = h.DB.GetPreviewInstanceByID(d.PreviewInstanceID)
		}
		containerName := db.DeploymentContainerName(project.Name, environment, previewInstance)

		_, err := h.Manager.WriteNginxConfig(domain.ProvisionConfig{
			ProjectID:         project.ID,
			ProjectName:       project.Name,
			Domain:            plan.CanonicalDomain,
			RedirectDomain:    plan.RedirectDomain,
			Environment:       d.Environment,
			ContainerName:     containerName,
			Port:              updatedProject.Port,
			PasswordProtected: passwordProtected,
		})
		if err == nil {
			wroteConfig = true
		}
	}

	if wroteConfig {
		_ = h.Manager.ReloadProxy()
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
