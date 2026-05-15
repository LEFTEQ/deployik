package services

import (
	"fmt"
	"net/url"
)

// PostgresPort is the in-network TCP port Postgres listens on. The same value
// goes into DATABASE_URL (the in-network DSN); the loopback host_port
// recorded on the row is only used for SSH-tunnel external access.
const PostgresPort = 5432

// BuildPostgresEnvInjection composes the env vars that get merged into the
// app container's runtime environment when this spec's sidecar is attached.
// DATABASE_URL uses url.UserPassword so the password is percent-encoded for
// special chars (@, /, :, space, etc) — the spec's plaintext password is
// random base64url today (see Manager.Provision), so this is belt-and-suspenders
// against future password schemes.
func BuildPostgresEnvInjection(spec ServiceSpec) EnvInjection {
	dsn := (&url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(spec.DBUser, spec.DBPasswordPlain),
		Host:   fmt.Sprintf("%s:%d", spec.ContainerName, PostgresPort),
		Path:   "/" + spec.DBName,
	}).String()

	return EnvInjection{
		Env: map[string]string{
			"DATABASE_URL":  dsn,
			"POSTGRES_HOST": spec.ContainerName,
			"POSTGRES_PORT": fmt.Sprintf("%d", PostgresPort),
			"POSTGRES_DB":   spec.DBName,
			"POSTGRES_USER": spec.DBUser,
		},
		Secrets: map[string]string{
			"POSTGRES_PASSWORD": spec.DBPasswordPlain,
		},
	}
}
