package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/LEFTEQ/lovinka-deployik/internal/api/handlers"
	"github.com/LEFTEQ/lovinka-deployik/internal/api/middleware"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

// RouterConfig holds all dependencies needed by the router.
type RouterConfig struct {
	DB            *db.DB
	JWTSecret     string
	Encryptor     *crypto.Encryptor
	OAuthConfig   *github.OAuthConfig
	AllowedUsers  []string
	FrontendURL   string
	Pipeline      *build.Pipeline
	DomainManager *domain.Manager
	WSHub         *ws.Hub
}

func NewRouter(cfg *RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.CORS)

	authHandler := &handlers.AuthHandler{
		DB:           cfg.DB,
		OAuthConfig:  cfg.OAuthConfig,
		JWTSecret:    cfg.JWTSecret,
		Encryptor:    cfg.Encryptor,
		AllowedUsers: cfg.AllowedUsers,
		FrontendURL:  cfg.FrontendURL,
	}

	r.Route("/api", func(r chi.Router) {
		// Public routes
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		})

		// Auth routes (public)
		r.Get("/auth/github", authHandler.GetGithubAuth)
		r.Get("/auth/github/callback", authHandler.GithubCallback)
		r.Post("/auth/refresh", authHandler.RefreshToken)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))
			r.Get("/auth/me", authHandler.GetMe)

			// GitHub
			projectHandler := &handlers.ProjectHandler{DB: cfg.DB, Encryptor: cfg.Encryptor}
			r.Get("/github/repos", projectHandler.ListGithubRepos)
			r.Get("/github/branches", projectHandler.ListGithubBranches)

			// Projects
			r.Get("/projects", projectHandler.List)
			r.Post("/projects", projectHandler.Create)
			r.Get("/projects/{id}", projectHandler.Get)
			r.Patch("/projects/{id}", projectHandler.Update)
			r.Delete("/projects/{id}", projectHandler.Delete)

			// Deployments
			deployHandler := &handlers.DeploymentHandler{DB: cfg.DB, Encryptor: cfg.Encryptor, Pipeline: cfg.Pipeline}
			r.Get("/projects/{id}/deployments", deployHandler.List)
			r.Post("/projects/{id}/deployments", deployHandler.Trigger)
			r.Get("/projects/{id}/deployments/{did}", deployHandler.Get)
			r.Get("/deployments/{did}/logs", deployHandler.GetLogs)

			// Domains
			domainHandler := &handlers.DomainHandler{DB: cfg.DB, Manager: cfg.DomainManager}
			r.Get("/projects/{id}/domains", domainHandler.List)
			r.Post("/projects/{id}/domains", domainHandler.Add)
			r.Delete("/projects/{id}/domains/{did}", domainHandler.Delete)
			r.Post("/projects/{id}/domains/{did}/verify", domainHandler.Verify)

			// Environment Variables
			envHandler := &handlers.VariableHandler{
				DB:        cfg.DB,
				Encryptor: cfg.Encryptor,
				Kind:      db.VariableKindEnv,
			}
			r.Get("/projects/{id}/env", envHandler.List)
			r.Put("/projects/{id}/env", envHandler.BulkSet)
			r.Delete("/projects/{id}/env/{key}", envHandler.Delete)

			secretHandler := &handlers.VariableHandler{
				DB:        cfg.DB,
				Encryptor: cfg.Encryptor,
				Kind:      db.VariableKindSecret,
			}
			r.Get("/projects/{id}/secrets", secretHandler.List)
			r.Put("/projects/{id}/secrets", secretHandler.BulkSet)
			r.Delete("/projects/{id}/secrets/{key}", secretHandler.Delete)
		})
	})

	// WebSocket routes
	r.Get("/ws/deployments/{did}/logs", ws.LogsHandler(cfg.WSHub, cfg.JWTSecret))

	// Serve embedded SPA for all non-API routes
	r.NotFound(SPAHandler())

	return r
}
