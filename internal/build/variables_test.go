package build

import (
	"reflect"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestResolveDeploymentVariablesKeepsSecretsRuntimeOnly(t *testing.T) {
	t.Parallel()

	buildEnvVars, runtimeEnvVars := resolveDeploymentVariables(
		[]db.ProjectVariable{
			{Key: "NEXT_PUBLIC_API_URL", Value: "https://api.example.com"},
			{Key: "LOG_LEVEL", Value: "debug"},
		},
		[]db.ProjectVariable{
			{Key: "DATABASE_URL", Value: "postgres://secret"},
			{Key: "JWT_SECRET", Value: "super-secret"},
		},
	)

	wantBuild := []EnvVar{
		{Key: "NEXT_PUBLIC_API_URL", Value: "https://api.example.com"},
	}
	if !reflect.DeepEqual(buildEnvVars, wantBuild) {
		t.Fatalf("build env vars = %#v, want %#v", buildEnvVars, wantBuild)
	}

	wantRuntime := []string{
		"NEXT_PUBLIC_API_URL=https://api.example.com",
		"LOG_LEVEL=debug",
		"DATABASE_URL=postgres://secret",
		"JWT_SECRET=super-secret",
	}
	if !reflect.DeepEqual(runtimeEnvVars, wantRuntime) {
		t.Fatalf("runtime env vars = %#v, want %#v", runtimeEnvVars, wantRuntime)
	}
}
