package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port               string
	DatabasePath       string
	JWTSecret          string
	EncryptionKey      string
	GithubClientID     string
	GithubClientSecret string
	AllowedGithubUsers []string
	DataDir            string
	NginxConfDir       string
	ProxyContainerName string
	ProxyCertsDir      string
	ProxyHTMLDir       string
	SSLEmail           string
	BuildDir           string
	VPSHost            string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		DatabasePath:       getEnv("DATABASE_PATH", "data/deployik.db"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		EncryptionKey:      os.Getenv("ENCRYPTION_KEY"),
		GithubClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		GithubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		DataDir:            getEnv("DATA_DIR", "data"),
		NginxConfDir:       getEnv("NGINX_CONF_DIR", "/opt/nginx-proxy/conf.d"),
		ProxyContainerName: getEnv("PROXY_CONTAINER_NAME", "nginx-proxy"),
		ProxyCertsDir:      getEnv("PROXY_CERTS_DIR", "/opt/nginx-proxy/certs"),
		ProxyHTMLDir:       getEnv("PROXY_HTML_DIR", "/opt/nginx-proxy/html"),
		SSLEmail:           getEnv("SSL_EMAIL", "admin@example.com"),
		BuildDir:           getEnv("BUILD_DIR", "/tmp/deployik-builds"),
		VPSHost:            getEnv("VPS_HOST", "203.0.113.10"),
	}

	if users := os.Getenv("ALLOWED_GITHUB_USERS"); users != "" {
		for _, u := range strings.Split(users, ",") {
			if trimmed := strings.TrimSpace(u); trimmed != "" {
				cfg.AllowedGithubUsers = append(cfg.AllowedGithubUsers, trimmed)
			}
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
