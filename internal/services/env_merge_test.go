package services

import (
	"strings"
	"testing"
)

func TestMergeWithUserOverrideInjectsAllKeysWhenUserMapEmpty(t *testing.T) {
	t.Parallel()

	inj := EnvInjection{
		Env:     map[string]string{"DATABASE_URL": "postgres://app:pwd@host:5432/app", "POSTGRES_HOST": "host"},
		Secrets: map[string]string{"POSTGRES_PASSWORD": "pwd"},
	}
	merged := MergeWithUserOverride(nil, inj)
	if len(merged) != 3 {
		t.Fatalf("expected 3 vars, got %d (%v)", len(merged), merged)
	}
	if findVar(merged, "DATABASE_URL") != "postgres://app:pwd@host:5432/app" {
		t.Errorf("DATABASE_URL missing or wrong")
	}
	if findVar(merged, "POSTGRES_PASSWORD") != "pwd" {
		t.Errorf("POSTGRES_PASSWORD missing or wrong")
	}
}

func TestMergeWithUserOverrideUserKeysWin(t *testing.T) {
	t.Parallel()

	inj := EnvInjection{
		Env:     map[string]string{"DATABASE_URL": "postgres://injected"},
		Secrets: map[string]string{"POSTGRES_PASSWORD": "injected"},
	}
	// User has DATABASE_URL set to a managed Neon DB; injection must NOT clobber it.
	userVars := []string{"DATABASE_URL=postgres://user-managed", "OTHER_VAR=keep"}
	merged := MergeWithUserOverride(userVars, inj)

	if findVar(merged, "DATABASE_URL") != "postgres://user-managed" {
		t.Errorf("user DATABASE_URL should win, got %q", findVar(merged, "DATABASE_URL"))
	}
	if findVar(merged, "POSTGRES_PASSWORD") != "injected" {
		t.Errorf("POSTGRES_PASSWORD (not user-set) should be injected, got %q", findVar(merged, "POSTGRES_PASSWORD"))
	}
	if findVar(merged, "OTHER_VAR") != "keep" {
		t.Errorf("unrelated user var lost: %v", merged)
	}
}

func TestMergeWithUserOverrideSecretsObeyUserOverride(t *testing.T) {
	t.Parallel()

	inj := EnvInjection{
		Secrets: map[string]string{"POSTGRES_PASSWORD": "injected"},
	}
	merged := MergeWithUserOverride([]string{"POSTGRES_PASSWORD=user-set"}, inj)
	if findVar(merged, "POSTGRES_PASSWORD") != "user-set" {
		t.Errorf("user POSTGRES_PASSWORD should win, got %q", findVar(merged, "POSTGRES_PASSWORD"))
	}
}

// findVar returns the VALUE of NAME from a slice of "KEY=VAL" strings, or ""
// if missing. Mirrors how Docker accepts envVars in RunContainer.
func findVar(envVars []string, name string) string {
	prefix := name + "="
	for _, v := range envVars {
		if strings.HasPrefix(v, prefix) {
			return v[len(prefix):]
		}
	}
	return ""
}

func TestBuildPostgresEnvInjection(t *testing.T) {
	t.Parallel()

	spec := ServiceSpec{
		ContainerName:   "deployik-myapp-preview-pg",
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "s3cr3t",
	}
	inj := BuildPostgresEnvInjection(spec)

	if got := inj.Env["DATABASE_URL"]; got != "postgresql://app:s3cr3t@deployik-myapp-preview-pg:5432/app" {
		t.Errorf("DATABASE_URL = %q", got)
	}
	if got := inj.Env["POSTGRES_HOST"]; got != "deployik-myapp-preview-pg" {
		t.Errorf("POSTGRES_HOST = %q", got)
	}
	if got := inj.Env["POSTGRES_PORT"]; got != "5432" {
		t.Errorf("POSTGRES_PORT = %q", got)
	}
	if got := inj.Env["POSTGRES_DB"]; got != "app" {
		t.Errorf("POSTGRES_DB = %q", got)
	}
	if got := inj.Env["POSTGRES_USER"]; got != "app" {
		t.Errorf("POSTGRES_USER = %q", got)
	}
	if _, present := inj.Env["POSTGRES_PASSWORD"]; present {
		t.Error("POSTGRES_PASSWORD should be in Secrets, not Env")
	}
	if got := inj.Secrets["POSTGRES_PASSWORD"]; got != "s3cr3t" {
		t.Errorf("POSTGRES_PASSWORD secret = %q", got)
	}
}

func TestBuildPostgresEnvInjectionEscapesPasswordInDSN(t *testing.T) {
	t.Parallel()

	spec := ServiceSpec{
		ContainerName:   "deployik-myapp-preview-pg",
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "p@ss word/with:colon",
	}
	inj := BuildPostgresEnvInjection(spec)
	got := inj.Env["DATABASE_URL"]
	// The plaintext characters @, /, :, and space MUST be percent-encoded.
	// If they appear unencoded, parsers like pgx or libpq will misread the DSN.
	if strings.Contains(got, "@ss word/with:colon") {
		t.Fatalf("DATABASE_URL leaks unencoded password chars: %q", got)
	}
	if !strings.Contains(got, "p%40ss%20word%2Fwith%3Acolon") {
		t.Errorf("expected percent-encoded password in DSN, got %q", got)
	}
}
