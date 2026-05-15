package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/errdefs"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
)

// PostgresPort is the in-network TCP port Postgres listens on. The same value
// goes into DATABASE_URL (the in-network DSN); the loopback host_port
// recorded on the row is only used for SSH-tunnel external access.
const PostgresPort = 5432

// PostgresImage is the pinned image tag — v1 uses postgres:16. Changing this
// across an existing fleet would require a pg_upgrade strategy; out of scope.
const PostgresImage = "postgres:16"

// waitReadyTimeout caps how long EnsureRunning waits for pg_isready before
// declaring the sidecar failed. 60s is generous on cold-start; warm restarts
// usually pass on the second poll (~2s).
const waitReadyTimeout = 60 * time.Second

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

// EnsureRunning brings the Postgres sidecar for spec to a healthy state.
// Idempotent: if the container exists and is running it returns the live
// host port; if the container exists but is stopped it starts it; if it
// doesn't exist it creates the volume, pulls the image lazily, runs the
// container, and waits for pg_isready.
//
// On success the live host_port (from docker inspect) is set on spec.HostPort.
// Callers should persist it via db.UpdateServiceHostPort.
func EnsureRunning(ctx context.Context, docker *build.DockerClient, proxyNetwork string, spec *ServiceSpec) error {
	id, exists := docker.ContainerExists(ctx, spec.ContainerName)
	if exists {
		// "exists" is broader than "running": ContainerExists returns true for
		// created/paused/exited/dead containers too. Use IsContainerRunning to
		// gate the early return — otherwise we'd call GetHostPort on a stopped
		// container, get "" back, and silently persist HostPort=0.
		running, err := docker.IsContainerRunning(ctx, id)
		if err != nil {
			return fmt.Errorf("inspect pg container state: %w", err)
		}
		if running {
			port, err := docker.GetHostPort(ctx, id, PostgresPort)
			if err != nil {
				return fmt.Errorf("inspect host port for running pg: %w", err)
			}
			hp, err := strconv.Atoi(port)
			if err != nil || hp <= 0 {
				return fmt.Errorf("parse pg host port %q: %w", port, err)
			}
			spec.HostPort = hp
			return nil
		}
		// Container exists but is stopped — remove it so we get a clean
		// recreate via the path below. StopContainer is force-remove.
		if err := docker.StopContainer(ctx, id); err != nil {
			return fmt.Errorf("clean up stopped pg container: %w", err)
		}
	}

	if err := docker.EnsureVolume(ctx, spec.VolumeName); err != nil {
		return fmt.Errorf("ensure pg volume: %w", err)
	}

	if err := ensurePostgresImage(ctx); err != nil {
		return fmt.Errorf("pull postgres image: %w", err)
	}

	envVars := []string{
		"POSTGRES_DB=" + spec.DBName,
		"POSTGRES_USER=" + spec.DBUser,
		"POSTGRES_PASSWORD=" + spec.DBPasswordPlain,
		"PGDATA=/var/lib/postgresql/data/pgdata",
	}
	mountSpec := spec.VolumeName + ":/var/lib/postgresql/data"

	containerID, err := docker.RunContainer(ctx, spec.ContainerName, PostgresImage, envVars, proxyNetwork, build.RunContainerOptions{
		VolumeBinds:  []string{mountSpec},
		Port:         PostgresPort,
		BindHostPort: true, // bind to 127.0.0.1:<random> for SSH-tunnel access
	})
	if err != nil {
		return fmt.Errorf("run pg container: %w", err)
	}

	if err := WaitReady(ctx, spec, waitReadyTimeout); err != nil {
		return fmt.Errorf("pg not ready in %s: %w", waitReadyTimeout, err)
	}

	port, err := docker.GetHostPort(ctx, containerID, PostgresPort)
	if err != nil {
		return fmt.Errorf("inspect host port after start: %w", err)
	}
	hp, err := strconv.Atoi(port)
	if err != nil || hp <= 0 {
		return fmt.Errorf("parse pg host port after start %q: %w", port, err)
	}
	spec.HostPort = hp
	return nil
}

// ensurePostgresImage lazily pulls postgres:16 if it isn't already present.
// Uses `docker pull` via exec rather than ImagePull's stream-decode loop
// because we don't need to surface progress — the deploy log shows a single
// "Pulling postgres:16..." line.
func ensurePostgresImage(ctx context.Context) error {
	pullCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(pullCtx, "docker", "pull", PostgresImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// WaitReady polls pg_isready inside the container until it succeeds or the
// timeout elapses. Uses `docker exec` rather than connecting through the
// network because the loopback host_port may not be assigned yet at this
// point — and we don't want to depend on the proxy network being reachable
// from the deployik process itself.
func WaitReady(ctx context.Context, spec *ServiceSpec, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	var lastOut []byte
	for time.Now().Before(deadline) {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		cmd := exec.CommandContext(probeCtx, "docker", "exec", spec.ContainerName,
			"pg_isready", "-U", spec.DBUser, "-d", spec.DBName)
		out, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			return nil
		}
		lastErr, lastOut = err, out
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if lastErr == nil {
		return fmt.Errorf("pg_isready did not succeed within %s", timeout)
	}
	return fmt.Errorf("pg_isready did not succeed within %s: %w (tail: %s)",
		timeout, lastErr, strings.TrimSpace(string(lastOut)))
}

// Stop halts and removes the Postgres container. The named volume is NOT
// removed — call ResetData to also wipe data.
func Stop(ctx context.Context, docker *build.DockerClient, spec *ServiceSpec) error {
	id, _ := docker.ContainerExists(ctx, spec.ContainerName)
	if id == "" {
		return nil // already gone
	}
	return docker.StopContainer(ctx, id)
}

// Restart stops and re-runs the container, preserving volume data. Used by
// the [Restart] button.
func Restart(ctx context.Context, docker *build.DockerClient, proxyNetwork string, spec *ServiceSpec) error {
	if err := Stop(ctx, docker, spec); err != nil {
		return fmt.Errorf("stop before restart: %w", err)
	}
	return EnsureRunning(ctx, docker, proxyNetwork, spec)
}

// ResetData removes the container AND the named volume, then recreates an
// empty Postgres instance. Used by [Reset] (typed-confirm in the UI).
func ResetData(ctx context.Context, docker *build.DockerClient, proxyNetwork string, spec *ServiceSpec) error {
	if err := Stop(ctx, docker, spec); err != nil {
		return fmt.Errorf("stop before reset: %w", err)
	}
	if err := docker.RemoveVolume(ctx, spec.VolumeName); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove pg volume: %w", err)
	}
	return EnsureRunning(ctx, docker, proxyNetwork, spec)
}

// Logs streams `docker logs --follow` of the pg container into w until ctx is
// cancelled or the container exits. Used by the WS handler at
// /ws/projects/{id}/services/{env}/logs.
func Logs(ctx context.Context, spec *ServiceSpec, w io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--follow", "--tail", "200", spec.ContainerName)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		log.Printf("services.Logs: docker logs %s ended: %v", spec.ContainerName, err)
		return err
	}
	return nil
}
