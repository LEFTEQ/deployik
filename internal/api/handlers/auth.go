package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
)

type AuthHandler struct {
	DB           *db.DB
	OAuthConfig  *github.OAuthConfig
	JWTSecret    string
	Encryptor    *crypto.Encryptor
	AllowedUsers []string
	AdminUsers   []string
	FrontendURL  string
	CookieSecure bool
	Audit        *audit.Recorder
}

type authResponse struct {
	User db.User `json:"user"`
}

// GithubCallback handles the OAuth callback from GitHub.
func (h *AuthHandler) GithubCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing code parameter"})
		return
	}

	if err := h.validateOAuthState(r); err != nil {
		h.clearCookie(w, auth.OAuthStateCookieName)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	h.clearCookie(w, auth.OAuthStateCookieName)

	// Exchange code for token
	tokenResp, err := h.OAuthConfig.ExchangeCode(code)
	if err != nil {
		log.Printf("OAuth exchange error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to exchange code"})
		return
	}

	// Get GitHub user
	ghUser, err := github.GetUser(tokenResp.AccessToken)
	if err != nil {
		log.Printf("GitHub user fetch error: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to get user info"})
		return
	}

	// Check allowlist
	if len(h.AllowedUsers) > 0 && !slices.Contains(h.AllowedUsers, ghUser.Login) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "user not in allowlist"})
		return
	}

	// Encrypt the GitHub token for storage
	encryptedToken, err := h.Encryptor.Encrypt(tokenResp.AccessToken)
	if err != nil {
		log.Printf("Token encryption error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	existingUser, _ := h.DB.GetUserByGithubID(ghUser.ID)
	role, err := h.determineRole(existingUser, ghUser.Login)
	if err != nil {
		log.Printf("Role resolution error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to determine user role"})
		return
	}

	// Upsert user
	user := &db.User{
		ID:          db.NewID(),
		GithubID:    ghUser.ID,
		Username:    ghUser.Login,
		AvatarURL:   ghUser.AvatarURL,
		GithubToken: encryptedToken,
		Role:        role,
	}

	// If user exists, keep their ID
	if existingUser != nil {
		user.ID = existingUser.ID
	}

	if err := h.DB.UpsertUser(user); err != nil {
		log.Printf("User upsert error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save user"})
		return
	}

	if err := h.issueSession(w, user, ""); err != nil {
		log.Printf("Session issuance error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start session"})
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		User: *user,
	})
	h.Audit.Record(audit.Entry{
		UserID:       user.ID,
		Action:       "auth.login",
		ResourceType: "user",
		ResourceID:   user.ID,
		Metadata: map[string]any{
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

// GetGithubAuth redirects to GitHub OAuth authorization page.
func (h *AuthHandler) GetGithubAuth(w http.ResponseWriter, r *http.Request) {
	state, err := auth.GenerateOpaqueToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate oauth state"})
		return
	}

	h.setCookie(w, &http.Cookie{
		Name:     auth.OAuthStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((10 * time.Minute).Seconds()),
	})

	url := h.OAuthConfig.AuthorizeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// RefreshToken issues a new access token from a valid refresh token.
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	refreshCookie, err := r.Cookie(auth.RefreshCookieName)
	if err != nil || strings.TrimSpace(refreshCookie.Value) == "" {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing refresh token"})
		return
	}

	refreshTokenHash := auth.HashToken(strings.TrimSpace(refreshCookie.Value))
	session, err := h.DB.GetActiveRefreshSessionByHash(refreshTokenHash)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load refresh session"})
		return
	}
	if session == nil {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
		return
	}

	user, err := h.DB.GetUserByID(session.UserID)
	if err != nil || user == nil {
		h.clearSessionCookies(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	if err := h.issueSession(w, user, session.ID); err != nil {
		if err == sql.ErrNoRows {
			h.clearSessionCookies(w)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to rotate session"})
		return
	}

	writeJSON(w, http.StatusOK, authResponse{User: *user})
	h.Audit.Record(audit.Entry{
		UserID:       user.ID,
		Action:       "auth.refresh",
		ResourceType: "user",
		ResourceID:   user.ID,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var userID string
	if claims := auth.GetClaims(r.Context()); claims != nil {
		userID = claims.UserID
	} else if accessCookie, err := r.Cookie(auth.AccessCookieName); err == nil && strings.TrimSpace(accessCookie.Value) != "" {
		if claims, err := auth.ValidateAccessToken(h.JWTSecret, strings.TrimSpace(accessCookie.Value)); err == nil {
			userID = claims.UserID
		}
	}
	if refreshCookie, err := r.Cookie(auth.RefreshCookieName); err == nil && strings.TrimSpace(refreshCookie.Value) != "" {
		if err := h.DB.RevokeRefreshSessionByHash(auth.HashToken(strings.TrimSpace(refreshCookie.Value))); err != nil {
			log.Printf("Refresh session revoke error: %v", err)
		}
	}
	h.clearSessionCookies(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	h.Audit.Record(audit.Entry{
		UserID:       userID,
		Action:       "auth.logout",
		ResourceType: "user",
		ResourceID:   userID,
	})
}

func (h *AuthHandler) issueSession(w http.ResponseWriter, user *db.User, rotateFromSessionID string) error {
	accessToken, err := auth.GenerateAccessToken(h.JWTSecret, user.ID, user.Username, user.Role)
	if err != nil {
		return err
	}

	refreshToken, err := auth.GenerateOpaqueToken()
	if err != nil {
		return err
	}
	refreshTokenHash := auth.HashToken(refreshToken)
	refreshExpiry := time.Now().Add(auth.RefreshTokenExpiry)

	if rotateFromSessionID == "" {
		if err := h.DB.CreateRefreshSession(&db.RefreshSession{
			UserID:    user.ID,
			TokenHash: refreshTokenHash,
			ExpiresAt: refreshExpiry,
		}); err != nil {
			return err
		}
	} else if err := h.DB.RotateRefreshSession(rotateFromSessionID, user.ID, refreshTokenHash, refreshExpiry); err != nil {
		return err
	}

	h.setCookie(w, &http.Cookie{
		Name:     auth.AccessCookieName,
		Value:    accessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.AccessTokenExpiry.Seconds()),
	})
	h.setCookie(w, &http.Cookie{
		Name:     auth.RefreshCookieName,
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.RefreshTokenExpiry.Seconds()),
	})
	return nil
}

func (h *AuthHandler) clearSessionCookies(w http.ResponseWriter) {
	h.clearCookie(w, auth.AccessCookieName)
	h.clearCookie(w, auth.RefreshCookieName)
}

func (h *AuthHandler) clearCookie(w http.ResponseWriter, name string) {
	h.setCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *AuthHandler) setCookie(w http.ResponseWriter, cookie *http.Cookie) {
	http.SetCookie(w, cookie)
}

func (h *AuthHandler) validateOAuthState(r *http.Request) error {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		return errors.New("missing oauth state")
	}

	cookie, err := r.Cookie(auth.OAuthStateCookieName)
	if err != nil {
		return errors.New("missing oauth state cookie")
	}

	if strings.TrimSpace(cookie.Value) == "" || cookie.Value != state {
		return errors.New("invalid oauth state")
	}

	return nil
}

func (h *AuthHandler) determineRole(existingUser *db.User, githubLogin string) (string, error) {
	if existingUser != nil {
		if slices.Contains(h.AdminUsers, githubLogin) {
			return "admin", nil
		}
		return existingUser.Role, nil
	}

	if slices.Contains(h.AdminUsers, githubLogin) {
		return "admin", nil
	}

	var count int
	if err := h.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return "", err
	}
	if count == 0 && len(h.AdminUsers) == 0 {
		return "admin", nil
	}
	return "user", nil
}

// GetMe returns the current authenticated user.
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	user, err := h.DB.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	writeJSON(w, http.StatusOK, user)
}
