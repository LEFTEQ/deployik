package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/LEFTEQ/lovinka-deployik/internal/analytics"
	"github.com/LEFTEQ/lovinka-deployik/internal/api/handlers"
	"github.com/LEFTEQ/lovinka-deployik/internal/api/middleware"
	"github.com/LEFTEQ/lovinka-deployik/internal/audit"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

// RouterConfig holds all dependencies needed by the router.
type RouterConfig struct {
	DB             *db.DB
	JWTSecret      string
	Encryptor      *crypto.Encryptor
	OAuthConfig    *github.OAuthConfig
	AllowedUsers   []string
	AdminUsers     []string
	FrontendURL    string
	CookieSecure   bool
	AllowedOrigins []string
	Pipeline       *build.Pipeline
	DomainManager  *domain.Manager
	WSHub          *ws.Hub
	Analytics      *analytics.Service
	WebhookURL     string
	ScreenshotDir  string
	DevMode        bool
}

func NewRouter(cfg *RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.CORS(cfg.AllowedOrigins))

	auditRecorder := &audit.Recorder{DB: cfg.DB}
	oauthLimiter := middleware.NewRateLimiter(20, time.Minute)
	refreshLimiter := middleware.NewRateLimiter(30, time.Minute)
	mutationLimiter := middleware.NewRateLimiter(60, time.Minute)
	wsLimiter := middleware.NewRateLimiter(30, time.Minute)

	authHandler := &handlers.AuthHandler{
		DB:           cfg.DB,
		OAuthConfig:  cfg.OAuthConfig,
		JWTSecret:    cfg.JWTSecret,
		Encryptor:    cfg.Encryptor,
		AllowedUsers: cfg.AllowedUsers,
		AdminUsers:   cfg.AdminUsers,
		FrontendURL:  cfg.FrontendURL,
		CookieSecure: cfg.CookieSecure,
		Audit:        auditRecorder,
	}

	r.Route("/api", func(r chi.Router) {
		// Public routes
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		})

		// Auth routes (public)
		r.With(oauthLimiter.Middleware("oauth_start")).Get("/auth/github", authHandler.GetGithubAuth)
		r.With(oauthLimiter.Middleware("oauth_callback")).Get("/auth/github/callback", authHandler.GithubCallback)

		// Dev-only login (bypasses GitHub OAuth for local testing)
		if cfg.DevMode {
			r.Post("/auth/dev-login", authHandler.DevLogin)
		}
		r.With(refreshLimiter.Middleware("auth_refresh")).Post("/auth/refresh", authHandler.RefreshToken)
		r.With(refreshLimiter.Middleware("auth_logout")).Post("/auth/logout", authHandler.Logout)

		// Webhook routes (public, signature-validated)
		webhookHandler := &handlers.WebhookHandler{
			DB:        cfg.DB,
			Encryptor: cfg.Encryptor,
			Pipeline:  cfg.Pipeline,
		}
		webhookLimiter := middleware.NewRateLimiter(60, time.Minute)
		r.With(webhookLimiter.Middleware("webhook_github")).
			Post("/webhooks/github", webhookHandler.HandleGithub)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Authenticate(cfg.JWTSecret))
			r.Get("/auth/me", authHandler.GetMe)

			// GitHub
			var dockerClient *build.DockerClient
			if cfg.Pipeline != nil {
				dockerClient = cfg.Pipeline.Docker
			}
			projectHandler := &handlers.ProjectHandler{
				DB:        cfg.DB,
				Docker:    dockerClient,
				Manager:   cfg.DomainManager,
				Encryptor: cfg.Encryptor,
				Audit:     auditRecorder,
				Analytics: cfg.Analytics,
			}
			r.Get("/github/repos", projectHandler.ListGithubRepos)
			r.Get("/github/branches", projectHandler.ListGithubBranches)

			organizationHandler := &handlers.OrganizationHandler{DB: cfg.DB}
			r.Get("/organizations", organizationHandler.List)

			platformHandler := &handlers.PlatformHandler{}
			if cfg.DomainManager != nil {
				platformHandler.DNSTargetIP = cfg.DomainManager.VPSHost
			}
			r.Get("/platform", platformHandler.Get)

			// Projects
			r.Get("/projects", projectHandler.List)
			r.With(mutationLimiter.Middleware("project_create")).Post("/projects", projectHandler.Create)
			r.Get("/projects/{id}", projectHandler.Get)
			r.With(mutationLimiter.Middleware("project_update")).Patch("/projects/{id}", projectHandler.Update)
			r.With(mutationLimiter.Middleware("project_delete")).Delete("/projects/{id}", projectHandler.Delete)
			projectAnalyticsHandler := &handlers.ProjectAnalyticsHandler{DB: cfg.DB, Analytics: cfg.Analytics}
			r.Get("/projects/{id}/analytics", projectAnalyticsHandler.Get)
			r.With(mutationLimiter.Middleware("project_analytics_verify")).Post("/projects/{id}/analytics/verify", projectAnalyticsHandler.Verify)

			// Deployments
			deployHandler := &handlers.DeploymentHandler{DB: cfg.DB, Encryptor: cfg.Encryptor, Pipeline: cfg.Pipeline, Audit: auditRecorder}
			r.Get("/projects/{id}/deployments", deployHandler.List)
			r.With(mutationLimiter.Middleware("deployment_trigger")).Post("/projects/{id}/deployments", deployHandler.Trigger)
			r.Get("/projects/{id}/deployments/{did}", deployHandler.Get)
			r.Get("/deployments/{did}/logs", deployHandler.GetLogs)

			// Auto-build
			autobuildHandler := &handlers.AutoBuildHandler{
				DB:         cfg.DB,
				Encryptor:  cfg.Encryptor,
				Audit:      auditRecorder,
				WebhookURL: cfg.WebhookURL,
			}
			r.Get("/projects/{id}/auto-build", autobuildHandler.Get)
			r.With(mutationLimiter.Middleware("autobuild_update")).Put("/projects/{id}/auto-build", autobuildHandler.Put)
			r.With(mutationLimiter.Middleware("autobuild_delete")).Delete("/projects/{id}/auto-build", autobuildHandler.Delete)

			// Screenshots
			screenshotHandler := &handlers.ScreenshotHandler{DB: cfg.DB}
			r.Get("/deployments/{did}/screenshot", screenshotHandler.Get)

			// Domains
			domainHandler := &handlers.DomainHandler{DB: cfg.DB, Manager: cfg.DomainManager, Audit: auditRecorder}
			r.Get("/projects/{id}/domains", domainHandler.List)
			r.With(mutationLimiter.Middleware("domain_add")).Post("/projects/{id}/domains", domainHandler.Add)
			r.With(mutationLimiter.Middleware("domain_delete")).Delete("/projects/{id}/domains/{did}", domainHandler.Delete)
			r.With(mutationLimiter.Middleware("domain_verify")).Post("/projects/{id}/domains/{did}/verify", domainHandler.Verify)

			// Environment Variables
			envHandler := &handlers.VariableHandler{
				DB:        cfg.DB,
				Encryptor: cfg.Encryptor,
				Kind:      db.VariableKindEnv,
				Audit:     auditRecorder,
			}
			r.Get("/projects/{id}/env", envHandler.List)
			r.With(mutationLimiter.Middleware("env_bulk_set")).Put("/projects/{id}/env", envHandler.BulkSet)
			r.With(mutationLimiter.Middleware("env_upsert")).Post("/projects/{id}/env", envHandler.Upsert)
			r.With(mutationLimiter.Middleware("env_delete")).Delete("/projects/{id}/env/{key}", envHandler.Delete)

			secretHandler := &handlers.VariableHandler{
				DB:        cfg.DB,
				Encryptor: cfg.Encryptor,
				Kind:      db.VariableKindSecret,
				Audit:     auditRecorder,
			}
			r.Get("/projects/{id}/secrets", secretHandler.List)
			r.With(mutationLimiter.Middleware("secret_bulk_set")).Put("/projects/{id}/secrets", secretHandler.BulkSet)
			r.With(mutationLimiter.Middleware("secret_upsert")).Post("/projects/{id}/secrets", secretHandler.Upsert)
			r.With(mutationLimiter.Middleware("secret_delete")).Delete("/projects/{id}/secrets/{key}", secretHandler.Delete)
		})
	})

	// WebSocket routes
	r.With(wsLimiter.Middleware("ws_logs")).Get("/ws/deployments/{did}/logs", ws.LogsHandler(cfg.WSHub, cfg.DB, cfg.JWTSecret, cfg.AllowedOrigins))

	// Serve embedded SPA for all non-API routes
	r.NotFound(SPAHandler())

	return r
}
