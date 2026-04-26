package api

import (
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
	projectemail "github.com/LEFTEQ/lovinka-deployik/internal/email"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
	"github.com/LEFTEQ/lovinka-deployik/internal/version"
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
	Email          *projectemail.Service
	WebhookURL     string
	ScreenshotDir  string
	DevMode        bool
	Version        *version.Info
}

func NewRouter(cfg *RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(middleware.TrustedProxyRealIP())
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(middleware.BodyLimit(middleware.DefaultBodyLimit))

	auditRecorder := &audit.Recorder{DB: cfg.DB}
	oauthLimiter := middleware.NewRateLimiter(20, time.Minute)
	refreshLimiter := middleware.NewRateLimiter(30, time.Minute)
	mutationLimiter := middleware.NewRateLimiter(60, time.Minute)
	wsLimiter := middleware.NewRateLimiter(30, time.Minute)
	inspectLimiter := middleware.NewRateLimiter(20, time.Minute)

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

	// Site-auth routes (public, called by nginx and auth page)
	protectionHandler := &handlers.ProtectionHandler{
		DB:        cfg.DB,
		Encryptor: cfg.Encryptor,
		JWTSecret: cfg.JWTSecret,
		Manager:   cfg.DomainManager,
		Audit:     auditRecorder,
	}
	r.Post("/api/site-auth/verify", protectionHandler.Verify)
	r.Get("/api/site-auth/check", protectionHandler.Check)

	r.Route("/api", func(r chi.Router) {
		// Public routes
		healthHandler := &handlers.HealthHandler{Version: cfg.Version}
		r.Get("/health", healthHandler.Get)

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
			r.Use(middleware.Authenticate(cfg.JWTSecret, cfg.DB))
			r.Get("/auth/me", authHandler.GetMe)

			// Personal Access Tokens — used by the deployik-howto skill and any
			// future external tooling that needs Bearer auth without a browser session.
			tokenHandler := &handlers.TokenHandler{DB: cfg.DB, Audit: auditRecorder}
			r.Get("/me/tokens", tokenHandler.List)
			r.With(mutationLimiter.Middleware("token_create")).Post("/me/tokens", tokenHandler.Create)
			r.With(mutationLimiter.Middleware("token_revoke")).Delete("/me/tokens/{id}", tokenHandler.Revoke)

			// GitHub
			var dockerClient *build.DockerClient
			if cfg.Pipeline != nil {
				dockerClient = cfg.Pipeline.Docker
			}
			projectHandler := &handlers.ProjectHandler{
				DB:         cfg.DB,
				Docker:     dockerClient,
				Manager:    cfg.DomainManager,
				Encryptor:  cfg.Encryptor,
				Audit:      auditRecorder,
				Analytics:  cfg.Analytics,
				DevMode:    cfg.DevMode,
				Pipeline:   cfg.Pipeline,
				WebhookURL: cfg.WebhookURL,
			}
			r.Get("/github/repos", projectHandler.ListGithubRepos)
			r.Get("/github/branches", projectHandler.ListGithubBranches)
			inspectHandler := &handlers.InspectHandler{
				DB:        cfg.DB,
				Encryptor: cfg.Encryptor,
			}
			r.With(inspectLimiter.Middleware("github_inspect")).
				Get("/github/repos/{owner}/{repo}/inspect", inspectHandler.Get)

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
			projectEmailHandler := &handlers.ProjectEmailHandler{DB: cfg.DB, Email: cfg.Email, Audit: auditRecorder}
			r.Get("/projects/{id}/email", projectEmailHandler.Get)
			r.With(mutationLimiter.Middleware("project_email_update")).Put("/projects/{id}/email", projectEmailHandler.Update)
			r.With(mutationLimiter.Middleware("project_email_test_smtp")).Post("/projects/{id}/email/test-smtp", projectEmailHandler.TestSMTP)

			// Deployments
			deployHandler := &handlers.DeploymentHandler{DB: cfg.DB, Encryptor: cfg.Encryptor, Pipeline: cfg.Pipeline, Audit: auditRecorder}
			r.Get("/projects/{id}/deployments", deployHandler.List)
			r.With(mutationLimiter.Middleware("deployment_trigger")).Post("/projects/{id}/deployments", deployHandler.Trigger)
			r.Get("/projects/{id}/deployments/{did}", deployHandler.Get)
			r.Get("/deployments/{did}/logs", deployHandler.GetLogs)

			// Password protection
			r.Get("/projects/{id}/protection", protectionHandler.Get)
			r.With(mutationLimiter.Middleware("protection_update")).Put("/projects/{id}/protection", protectionHandler.Update)
			r.With(mutationLimiter.Middleware("protection_regenerate")).Post("/projects/{id}/protection/regenerate", protectionHandler.Regenerate)

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

			// Volumes
			volumeHandler := &handlers.VolumeHandler{DB: cfg.DB, Docker: dockerClient, Audit: auditRecorder}
			r.Get("/projects/{id}/volumes", volumeHandler.List)
			r.With(mutationLimiter.Middleware("volume_delete")).Delete("/projects/{id}/volumes/{env}", volumeHandler.Delete)
			r.With(mutationLimiter.Middleware("volume_recreate")).Post("/projects/{id}/volumes/{env}/recreate", volumeHandler.Recreate)

			// Domains
			domainHandler := &handlers.DomainHandler{DB: cfg.DB, Manager: cfg.DomainManager, Hub: cfg.WSHub, Audit: auditRecorder}
			r.Get("/projects/{id}/domains", domainHandler.List)
			r.With(mutationLimiter.Middleware("domain_add")).Post("/projects/{id}/domains", domainHandler.Add)
			r.With(mutationLimiter.Middleware("domain_update")).Patch("/projects/{id}/domains/{did}", domainHandler.Update)
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
	r.With(wsLimiter.Middleware("ws_domain_logs")).Get("/ws/domains/{did}/logs", ws.DomainLogsHandler(cfg.WSHub, cfg.DB, cfg.JWTSecret, cfg.AllowedOrigins))

	// Serve embedded SPA for all non-API routes
	r.NotFound(SPAHandler())

	return r
}
