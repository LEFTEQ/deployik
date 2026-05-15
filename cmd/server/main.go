package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/analytics"
	"github.com/LEFTEQ/lovinka-deployik/internal/api"
	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/config"
	"github.com/LEFTEQ/lovinka-deployik/internal/crypto"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
	"github.com/LEFTEQ/lovinka-deployik/internal/domain"
	projectemail "github.com/LEFTEQ/lovinka-deployik/internal/email"
	"github.com/LEFTEQ/lovinka-deployik/internal/github"
	"github.com/LEFTEQ/lovinka-deployik/internal/services"
	"github.com/LEFTEQ/lovinka-deployik/internal/version"
	"github.com/LEFTEQ/lovinka-deployik/internal/ws"
)

//go:embed all:web_dist
var embeddedWeb embed.FS

// Build metadata injected by `go build -ldflags="-X main.<name>=<value>"`.
// Populated in CI via Docker build args; defaults below apply for local
// `make dev-api` (or any build that omits -ldflags).
var (
	gitSHA    = "dev"
	buildTime = "unknown"
	ghRunID   = ""
	ghRepo    = "lefteq/lovinka-deployik"
)

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

		// Bootstrap the named buildx builder in the background so startup isn't
		// blocked on pulling the BuildKit image on first boot. A failure here is
		// non-fatal; the first build will surface a clearer error if it sticks.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if err := dockerClient.EnsureBuildxBuilder(ctx); err != nil {
				log.Printf("Warning: buildx builder bootstrap failed: %v", err)
			} else {
				log.Printf("Buildx builder %q ready", build.BuildxBuilderName)
			}
		}()
	}

	wsHub := ws.NewHub()
	domainManager := domain.NewManager(domain.ManagerConfig{
		NginxConfDir:      cfg.NginxConfDir,
		ProxyContainer:    cfg.ProxyContainerName,
		ProxyCertsDir:     cfg.ProxyCertsDir,
		ProxyHTMLDir:      cfg.ProxyHTMLDir,
		VPSHost:           cfg.VPSHost,
		SSLEmail:          cfg.SSLEmail,
		ProxyType:         cfg.ProxyType,
		ProxyConfigFormat: cfg.ProxyConfigFormat,
		ProxyReloadCmd:    cfg.ProxyReloadCmd,
		ProxySSLCert:      cfg.ProxySSLCert,
		ProxySSLKey:       cfg.ProxySSLKey,
	})
	analyticsService := analytics.NewService(
		database,
		analytics.NewUmamiClient(cfg.AnalyticsUmamiURL, cfg.AnalyticsUmamiUsername, cfg.AnalyticsUmamiPassword),
		cfg.AnalyticsUmamiPublicURL,
		cfg.AnalyticsUmamiScriptURL,
		analytics.NewLokiClient(cfg.AnalyticsLokiURL),
	)
	emailService := projectemail.NewService(database, encryptor, nil)

	// Pipeline lifecycle: a long-lived context that graceful shutdown can cancel,
	// plus a WaitGroup that tracks deploy + screenshot goroutines so the drain
	// phase can wait for them to finish before the process exits.
	pipelineCtx, cancelPipeline := context.WithCancel(context.Background())
	defer cancelPipeline()
	var pipelineWg sync.WaitGroup

	// Resolve the host-filesystem path that backs cfg.ScreenshotDir so nested
	// chrome containers receive a bind source the daemon can find. When deployik
	// runs outside a container, this returns the same path. Logged so an
	// operator can confirm at boot.
	screenshotHostDir := dockerClient.ResolveHostPath(context.Background(), cfg.ScreenshotDir)
	if screenshotHostDir != cfg.ScreenshotDir {
		log.Printf("Screenshot bind source resolved: %s -> %s", cfg.ScreenshotDir, screenshotHostDir)
	}

	// Services manager: owns sidecar (Postgres in v1) lifecycle. Constructed
	// before the pipeline so we can wire it into EnsureServices below; also
	// handed to the router so per-project /services routes + the WS logs
	// endpoint can resolve specs without crossing the build → services →
	// build import boundary.
	servicesMgr := &services.Manager{
		DB:           database,
		Encryptor:    encryptor,
		Docker:       dockerClient,
		ProxyNetwork: "proxy",
	}

	maxBuilds := 1
	pipeline := &build.Pipeline{
		DB:                database,
		Docker:            dockerClient,
		Encryptor:         encryptor,
		Semaphore:         build.NewSemaphore(maxBuilds),
		DomainManager:     domainManager,
		BuildDir:          cfg.BuildDir,
		ProxyNetwork:      "proxy",
		ProxyType:         cfg.ProxyType,
		Hub:               wsHub,
		ScreenshotDir:     cfg.ScreenshotDir,
		ScreenshotHostDir: screenshotHostDir,
		JWTSecret:         cfg.JWTSecret,
		Ctx:               pipelineCtx,
		Wg:                &pipelineWg,
		MaxBuildDuration:  15 * time.Minute,
	}

	// Bridge the pipeline's function-pointer hook to the typed services.Manager
	// without creating a build → services import cycle.
	pipeline.EnsureServices = func(ctx context.Context, project *db.Project, environment string, userVars []string) ([]string, error) {
		inj, err := servicesMgr.EnsureForDeployment(ctx, project, environment)
		if err != nil {
			return nil, err
		}
		if inj == nil {
			return userVars, nil
		}
		return services.MergeWithUserOverride(userVars, *inj), nil
	}

	// Write the auth page HTML for password-protected sites
	authPagesDir := cfg.ProxyHTMLDir
	if err := domain.WriteAuthPage(authPagesDir); err != nil {
		log.Printf("Warning: failed to write auth page: %v", err)
	}

	if targets, err := database.ListActiveDomainProvisionTargets(); err != nil {
		log.Printf("Warning: failed to list active domains for nginx reconcile: %v", err)
	} else if err := domain.ReconcileActiveConfigs(domainManager, targets, dockerClient); err != nil {
		log.Printf("Warning: failed to reconcile active domain configs: %v", err)
	}

	// Configure OAuth
	oauthConfig := &github.OAuthConfig{
		ClientID:     cfg.GithubClientID,
		ClientSecret: cfg.GithubClientSecret,
		RedirectURI:  cfg.FrontendURL + "/auth/callback",
	}

	versionInfo := version.New(gitSHA, buildTime, ghRunID, ghRepo)

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
		Services:       servicesMgr,
		DomainManager:  domainManager,
		WSHub:          wsHub,
		Analytics:      analyticsService,
		Email:          emailService,
		WebhookURL:     cfg.WebhookURL,
		ScreenshotDir:  cfg.ScreenshotDir,
		DevMode:        os.Getenv("DEV_MODE") == "true",
		Version:        versionInfo,
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout intentionally left at zero: WebSocket endpoints (build logs,
		// domain verification logs) hold the response open for minutes. The WS
		// handlers set their own read/write deadlines to catch stalls.
		IdleTimeout: 120 * time.Second,
	}

	sigCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Deployik listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	case <-sigCtx.Done():
		log.Println("Shutdown signal received; draining...")
	}

	// Give in-flight HTTP requests time to finish.
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Cancel live deploys and wait for pipeline goroutines to finish, bounded.
	cancelPipeline()
	done := make(chan struct{})
	go func() {
		pipelineWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Println("Pipeline drained")
	case <-time.After(60 * time.Second):
		log.Println("Pipeline drain timed out; forcing exit")
	}
}
