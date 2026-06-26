// Package services manages per-project sidecar containers (Postgres in v1).
// Lifecycle ops (EnsureRunning, Stop, Restart, Reset, Logs) live here rather
// than internal/build so handlers can call them without importing the deploy
// pipeline.
package services

import (
	"fmt"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

// ServiceSpec is a fully-resolved (plaintext-password) sidecar specification.
// Built by Manager methods from a db.ProjectService row + crypto.Encryptor.
type ServiceSpec struct {
	ServiceID       string
	ProjectID       string
	ProjectName     string // used for container/volume naming
	Environment     string // "preview" or "production"
	Type            db.ServiceType
	Image           string
	DBName          string
	DBUser          string
	DBPasswordPlain string // decrypted; never persist back to disk as-is
	HostPort        int    // loopback host port; 0 means "not yet running"
	ContainerName   string // deployik-<project>-<env>-pg
	VolumeName      string // deployik-<project>-<env>-pg-data
}

// EnvInjection holds the env vars that get merged into the app container's
// runtime environment. POSTGRES_PASSWORD is split out so handlers can decide
// whether to expose it as a secret-store variable; the rest are plain env.
type EnvInjection struct {
	Env     map[string]string // DATABASE_URL, POSTGRES_HOST/PORT/DB/USER
	Secrets map[string]string // POSTGRES_PASSWORD
}

// PostgresContainerName returns the deterministic container name for the
// Postgres sidecar of a (project, environment) pair.
func PostgresContainerName(projectName, environment string) string {
	return fmt.Sprintf("deployik-%s-%s-pg", projectName, environment)
}

// PostgresVolumeName returns the deterministic Docker volume name for the
// Postgres data directory of a (project, environment) pair.
func PostgresVolumeName(projectName, environment string) string {
	return fmt.Sprintf("deployik-%s-%s-pg-data", projectName, environment)
}
