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
