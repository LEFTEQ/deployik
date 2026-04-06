package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port                    string
	DatabasePath            string
	JWTSecret               string
	EncryptionKey           string
	GithubClientID          string
	GithubClientSecret      string
	AllowedGithubUsers      []string
	AdminGithubUsers        []string
	FrontendURL             string
	AllowedOrigins          []string
	DataDir                 string
	NginxConfDir            string
	ProxyContainerName      string
	ProxyCertsDir           string
	ProxyHTMLDir            string
	SSLEmail                string
	BuildDir                string
	VPSHost                 string
	AnalyticsUmamiURL       string
	AnalyticsUmamiPublicURL string
	AnalyticsUmamiScriptURL string
	AnalyticsUmamiUsername  string
	AnalyticsUmamiPassword  string
	AnalyticsLokiURL        string
	WebhookURL              string
	ScreenshotDir           string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:                    getEnv("PORT", "8080"),
		DatabasePath:            getEnv("DATABASE_PATH", "data/deployik.db"),
		JWTSecret:               os.Getenv("JWT_SECRET"),
		EncryptionKey:           os.Getenv("ENCRYPTION_KEY"),
		GithubClientID:          os.Getenv("GITHUB_CLIENT_ID"),
		GithubClientSecret:      os.Getenv("GITHUB_CLIENT_SECRET"),
		FrontendURL:             strings.TrimRight(getEnv("FRONTEND_URL", "http://localhost:5173"), "/"),
		DataDir:                 getEnv("DATA_DIR", "data"),
		NginxConfDir:            getEnv("NGINX_CONF_DIR", "/opt/nginx-proxy/conf.d"),
		ProxyContainerName:      getEnv("PROXY_CONTAINER_NAME", "nginx-proxy"),
		ProxyCertsDir:           getEnv("PROXY_CERTS_DIR", "/opt/nginx-proxy/certs"),
		ProxyHTMLDir:            getEnv("PROXY_HTML_DIR", "/opt/nginx-proxy/html"),
		SSLEmail:                getEnv("SSL_EMAIL", "admin@example.com"),
		BuildDir:                getEnv("BUILD_DIR", "/tmp/deployik-builds"),
		VPSHost:                 getEnv("VPS_HOST", "203.0.113.10"),
		AnalyticsUmamiURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_URL")), "/"),
		AnalyticsUmamiPublicURL: strings.TrimRight(strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_PUBLIC_URL")), "/"),
		AnalyticsUmamiScriptURL: strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_SCRIPT_URL")),
		AnalyticsUmamiUsername:  strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_USERNAME")),
		AnalyticsUmamiPassword:  os.Getenv("ANALYTICS_UMAMI_PASSWORD"),
		AnalyticsLokiURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("ANALYTICS_LOKI_URL")), "/"),
	}

	if cfg.AnalyticsUmamiPublicURL == "" {
		cfg.AnalyticsUmamiPublicURL = cfg.AnalyticsUmamiURL
	}
	if cfg.AnalyticsUmamiScriptURL == "" && cfg.AnalyticsUmamiPublicURL != "" {
		cfg.AnalyticsUmamiScriptURL = cfg.AnalyticsUmamiPublicURL + "/script.js"
	}

	cfg.WebhookURL = getEnv("WEBHOOK_URL", cfg.FrontendURL+"/api/webhooks/github")
	cfg.ScreenshotDir = getEnv("SCREENSHOT_DIR", filepath.Join(cfg.DataDir, "screenshots"))

	if users := os.Getenv("ALLOWED_GITHUB_USERS"); users != "" {
		cfg.AllowedGithubUsers = splitCSV(users)
	}

	if users := os.Getenv("ADMIN_GITHUB_USERS"); users != "" {
		cfg.AdminGithubUsers = splitCSV(users)
	}

	allowedOrigins := map[string]struct{}{}
	if cfg.FrontendURL != "" {
		allowedOrigins[cfg.FrontendURL] = struct{}{}
	}
	for _, origin := range splitCSV(os.Getenv("ALLOWED_ORIGINS")) {
		allowedOrigins[strings.TrimRight(origin, "/")] = struct{}{}
	}
	for origin := range allowedOrigins {
		if origin != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, origin)
		}
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY environment variable is required")
	}

	return cfg, nil
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	var values []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func (cfg *Config) FrontendCookieSecure() bool {
	frontendURL, err := url.Parse(cfg.FrontendURL)
	if err != nil {
		return false
	}
	return frontendURL.Scheme == "https"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
