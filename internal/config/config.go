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
	BaseDomain              string
	PreviewDomainSuffix     string
	AnalyticsUmamiURL       string
	AnalyticsUmamiPublicURL string
	AnalyticsUmamiScriptURL string
	AnalyticsUmamiUsername  string
	AnalyticsUmamiPassword  string
	AnalyticsLokiURL        string
	ProxyType               string
	ProxyConfigFormat       string
	ProxyHTTP3              bool
	ProxyReloadCmd          string
	ProxySSLCert            string
	ProxySSLKey             string
	ProxySSLWildcardDomains []string
	WebhookURL              string
	ScreenshotDir           string
	MonitoringToken         string
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
		VPSHost:                 strings.TrimSpace(os.Getenv("VPS_HOST")),
		AnalyticsUmamiURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_URL")), "/"),
		AnalyticsUmamiPublicURL: strings.TrimRight(strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_PUBLIC_URL")), "/"),
		AnalyticsUmamiScriptURL: strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_SCRIPT_URL")),
		AnalyticsUmamiUsername:  strings.TrimSpace(os.Getenv("ANALYTICS_UMAMI_USERNAME")),
		AnalyticsUmamiPassword:  os.Getenv("ANALYTICS_UMAMI_PASSWORD"),
		AnalyticsLokiURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("ANALYTICS_LOKI_URL")), "/"),
		ProxyType:               getEnv("PROXY_TYPE", "docker"),
		ProxyConfigFormat:       getEnv("PROXY_CONFIG_FORMAT", "nginx"),
		// Off by default: emitting `listen 443 quic` against a proxy nginx
		// that lacks http_v3_module fails the config test on every deploy.
		ProxyHTTP3:              strings.EqualFold(getEnv("PROXY_HTTP3", "false"), "true"),
		ProxyReloadCmd:          os.Getenv("PROXY_RELOAD_CMD"),
		ProxySSLCert:            os.Getenv("PROXY_SSL_CERT"),
		ProxySSLKey:             os.Getenv("PROXY_SSL_KEY"),
		ProxySSLWildcardDomains: splitCSV(os.Getenv("PROXY_SSL_WILDCARD_DOMAINS")),
		MonitoringToken:         os.Getenv("MONITORING_TOKEN"),
	}

	if cfg.AnalyticsUmamiPublicURL == "" {
		cfg.AnalyticsUmamiPublicURL = cfg.AnalyticsUmamiURL
	}
	if cfg.AnalyticsUmamiScriptURL == "" && cfg.AnalyticsUmamiPublicURL != "" {
		cfg.AnalyticsUmamiScriptURL = cfg.AnalyticsUmamiPublicURL + "/script.js"
	}

	cfg.WebhookURL = getEnv("WEBHOOK_URL", cfg.FrontendURL+"/api/webhooks/github")
	cfg.ScreenshotDir = getEnv("SCREENSHOT_DIR", filepath.Join(cfg.DataDir, "screenshots"))

	// Auto-generated preview domains (e.g. my-app.preview.<base-domain>) need a
	// base domain the operator controls. Set BASE_DOMAIN (the apex, e.g.
	// example.com) for the default preview.<base-domain> suffix, or override the
	// full suffix directly with PREVIEW_DOMAIN_SUFFIX. The trailing dot is
	// tolerated. Whether this is required is enforced at startup (see main.go):
	// mandatory in production, falls back to a local suffix in DEV_MODE.
	cfg.BaseDomain = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(os.Getenv("BASE_DOMAIN")), "."))
	cfg.PreviewDomainSuffix = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(os.Getenv("PREVIEW_DOMAIN_SUFFIX")), "."))
	if cfg.PreviewDomainSuffix == "" && cfg.BaseDomain != "" {
		cfg.PreviewDomainSuffix = "preview." + cfg.BaseDomain
	}

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
