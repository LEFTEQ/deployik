package services

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// requireDocker skips the test when `docker` isn't on PATH or the daemon
// isn't reachable. Matches how internal/build skips its docker-touching tests.
func requireDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode skips docker tests")
	}
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

func TestEnsureRunningIdempotent(t *testing.T) {
	requireDocker(t)
	t.Parallel()

	docker, err := build.NewDockerClient()
	if err != nil {
		t.Fatalf("NewDockerClient: %v", err)
	}
	defer docker.Close()

	spec := &ServiceSpec{
		ProjectName:     "pgtest-" + db.NewID()[:8],
		Environment:     "preview",
		Type:            db.ServiceTypePostgres,
		Image:           PostgresImage,
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "test-" + db.NewID()[:8],
	}
	spec.ContainerName = PostgresContainerName(spec.ProjectName, spec.Environment)
	spec.VolumeName = PostgresVolumeName(spec.ProjectName, spec.Environment)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = Stop(ctx, docker, spec)
		_ = docker.RemoveVolume(ctx, spec.VolumeName)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := EnsureRunning(ctx, docker, "", spec); err != nil {
		t.Fatalf("first EnsureRunning: %v", err)
	}
	if spec.HostPort <= 0 {
		t.Fatalf("HostPort not set after EnsureRunning: %d", spec.HostPort)
	}
	port1 := spec.HostPort

	// Second call should be a no-op and preserve the host port.
	if err := EnsureRunning(ctx, docker, "", spec); err != nil {
		t.Fatalf("second EnsureRunning: %v", err)
	}
	if spec.HostPort != port1 {
		t.Errorf("HostPort changed across idempotent calls: %d → %d", port1, spec.HostPort)
	}
}

func TestResetDataWipesVolume(t *testing.T) {
	requireDocker(t)
	t.Parallel()

	docker, err := build.NewDockerClient()
	if err != nil {
		t.Fatalf("NewDockerClient: %v", err)
	}
	defer docker.Close()

	spec := &ServiceSpec{
		ProjectName:     "pgreset-" + db.NewID()[:8],
		Environment:     "preview",
		Type:            db.ServiceTypePostgres,
		Image:           PostgresImage,
		DBName:          "app",
		DBUser:          "app",
		DBPasswordPlain: "test-" + db.NewID()[:8],
	}
	spec.ContainerName = PostgresContainerName(spec.ProjectName, spec.Environment)
	spec.VolumeName = PostgresVolumeName(spec.ProjectName, spec.Environment)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = Stop(ctx, docker, spec)
		_ = docker.RemoveVolume(ctx, spec.VolumeName)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := EnsureRunning(ctx, docker, "", spec); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}

	// Create a marker table so we can verify reset wiped it.
	exec1 := exec.CommandContext(ctx, "docker", "exec", spec.ContainerName,
		"psql", "-U", spec.DBUser, "-d", spec.DBName, "-c", "CREATE TABLE marker (id INT);")
	if out, err := exec1.CombinedOutput(); err != nil {
		t.Fatalf("create marker: %v\n%s", err, out)
	}

	if err := ResetData(ctx, docker, "", spec); err != nil {
		t.Fatalf("ResetData: %v", err)
	}

	// Marker table must be gone.
	exec2 := exec.CommandContext(ctx, "docker", "exec", spec.ContainerName,
		"psql", "-U", spec.DBUser, "-d", spec.DBName, "-tAc",
		"SELECT to_regclass('marker') IS NULL;")
	out, err := exec2.CombinedOutput()
	if err != nil {
		t.Fatalf("check marker: %v\n%s", err, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "t") {
		t.Errorf("marker table survived ResetData: %s", out)
	}
}
