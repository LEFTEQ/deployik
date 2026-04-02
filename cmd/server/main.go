package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/LEFTEQ/lovinka-deployik/internal/analytics"
	"github.com/LEFTEQ/lovinka-deployik/internal/api"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/config"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

//go:embed all:web_dist
var embeddedWeb embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		if os.Getenv("DEV_MODE") == "true" {
			log.Printf("Warning: config error (dev mode): %v", err)
			cfg = &config.Config{
				Port:           "8080",
				DatabasePath:   "data/deployik.db",
				JWTSecret:      "dev-jwt-secret",
				EncryptionKey:  "dev-encryption-key",
				FrontendURL:    "http://localhost:5173",
				AllowedOrigins: []string{"http://localhost:5173"},
			}
		} else {
			log.Fatalf("Failed to load config: %v", err)
		}
	}

	// Initialize encryptor
	encryptor, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("Failed to create encryptor: %v", err)
	}

	// Initialize database
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Set up embedded SPA filesystem
	webFS, err := fs.Sub(embeddedWeb, "web_dist")
	if err != nil {
		log.Printf("Warning: embedded web not available: %v", err)
	} else {
		api.SetStaticFS(webFS)
	}

	// Initialize Docker client and build pipeline
	dockerClient, err := build.NewDockerClient()
	if err != nil {
		log.Printf("Warning: Docker client not available: %v", err)
	} else {
		defer dockerClient.Close()
	}

	wsHub := ws.NewHub()
	domainManager := domain.NewManager(domain.ManagerConfig{
		NginxConfDir:   cfg.NginxConfDir,
		ProxyContainer: cfg.ProxyContainerName,
		ProxyCertsDir:  cfg.ProxyCertsDir,
		ProxyHTMLDir:   cfg.ProxyHTMLDir,
		VPSHost:        cfg.VPSHost,
		SSLEmail:       cfg.SSLEmail,
	})
	analyticsService := analytics.NewService(
		database,
		analytics.NewUmamiClient(cfg.AnalyticsUmamiURL, cfg.AnalyticsUmamiUsername, cfg.AnalyticsUmamiPassword),
		cfg.AnalyticsUmamiPublicURL,
		analytics.NewLokiClient(cfg.AnalyticsLokiURL),
	)

	maxBuilds := 1
	pipeline := &build.Pipeline{
		DB:            database,
		Docker:        dockerClient,
		Encryptor:     encryptor,
		Semaphore:     build.NewSemaphore(maxBuilds),
		DomainManager: domainManager,
		BuildDir:      cfg.BuildDir,
		ProxyNetwork:  "proxy",
		Hub:           wsHub,
	}

	if targets, err := database.ListActiveDomainProvisionTargets(); err != nil {
		log.Printf("Warning: failed to list active domains for nginx reconcile: %v", err)
	} else if err := domain.ReconcileActiveConfigs(domainManager, targets); err != nil {
		log.Printf("Warning: failed to reconcile active domain configs: %v", err)
	}

	// Configure OAuth
	oauthConfig := &github.OAuthConfig{
		ClientID:     cfg.GithubClientID,
		ClientSecret: cfg.GithubClientSecret,
		RedirectURI:  cfg.FrontendURL + "/auth/callback",
	}

	// Create router with all dependencies
	router := api.NewRouter(&api.RouterConfig{
		DB:             database,
		JWTSecret:      cfg.JWTSecret,
		Encryptor:      encryptor,
		OAuthConfig:    oauthConfig,
		AllowedUsers:   cfg.AllowedGithubUsers,
		AdminUsers:     cfg.AdminGithubUsers,
		FrontendURL:    cfg.FrontendURL,
		CookieSecure:   cfg.FrontendCookieSecure(),
		AllowedOrigins: cfg.AllowedOrigins,
		Pipeline:       pipeline,
		DomainManager:  domainManager,
		WSHub:          wsHub,
		Analytics:      analyticsService,
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Deployik starting on %s", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
