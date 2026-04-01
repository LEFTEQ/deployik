package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"slices"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
)

type AuthHandler struct {
	DB               *db.DB
	OAuthConfig      *github.OAuthConfig
	JWTSecret        string
	Encryptor        *crypto.Encryptor
	AllowedUsers     []string
	FrontendURL      string
}

type authResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	User         db.User `json:"user"`
}

// GetGithubAuth redirects to GitHub OAuth authorization page.
func (h *AuthHandler) GetGithubAuth(w http.ResponseWriter, r *http.Request) {
	// Generate random state
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	// In production, state should be stored and verified on callback.
	// For MVP, we'll include it but skip server-side verification.
	url := h.OAuthConfig.AuthorizeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// GithubCallback handles the OAuth callback from GitHub.
func (h *AuthHandler) GithubCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing code parameter"})
		return
	}

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

	// Determine role: first user is admin
	role := "user"
	existingUser, _ := h.DB.GetUserByGithubID(ghUser.ID)
	if existingUser != nil {
		role = existingUser.Role
	} else {
		// Check if any users exist
		var count int
		h.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
		if count == 0 {
			role = "admin"
		}
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

	// Generate JWT tokens
	accessToken, err := auth.GenerateAccessToken(h.JWTSecret, user.ID, user.Username, user.Role)
	if err != nil {
		log.Printf("JWT generation error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	refreshToken, err := auth.GenerateRefreshToken(h.JWTSecret, user.ID)
	if err != nil {
		log.Printf("Refresh token error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate refresh token"})
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         *user,
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshToken issues a new access token from a valid refresh token.
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	userID, err := auth.ValidateRefreshToken(h.JWTSecret, req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
		return
	}

	user, err := h.DB.GetUserByID(userID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	accessToken, err := auth.GenerateAccessToken(h.JWTSecret, user.ID, user.Username, user.Role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	newRefreshToken, err := auth.GenerateRefreshToken(h.JWTSecret, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate refresh token"})
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		User:         *user,
	})
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
