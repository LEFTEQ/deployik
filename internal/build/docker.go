package build

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// DockerClient wraps the Docker SDK client.
type DockerClient struct {
	cli *client.Client
}

// NewDockerClient creates a Docker client using the default socket.
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &DockerClient{cli: cli}, nil
}

// ResolveHostPath translates a path inside our own container into the host
// path that backs it, by inspecting our container's mount config. Returns
// the input path unchanged when running outside a container or when no
// mount covers the requested path.
//
// This is needed because the Docker daemon resolves bind-mount Source paths
// against the host filesystem, not the caller's. Without this, a deployik
// container that asks Docker to start a sibling container with bind source
// "/data/screenshots" (a path inside deployik backed by a named volume)
// fails with "bind source path does not exist": the host has no such path.
func (d *DockerClient) ResolveHostPath(ctx context.Context, containerPath string) string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return containerPath
	}
	self, err := d.cli.ContainerInspect(ctx, hostname)
	if err != nil {
		return containerPath
	}
	// Pick the longest-matching mount destination so /data/screenshots wins
	// over /data when both could apply, and require a path-component boundary
	// so /datasets doesn't match a /data mount.
	var bestSource, bestDest string
	for _, m := range self.Mounts {
		if !strings.HasPrefix(containerPath, m.Destination) {
			continue
		}
		if len(containerPath) > len(m.Destination) && containerPath[len(m.Destination)] != '/' {
			continue
		}
		if len(m.Destination) > len(bestDest) {
			bestDest = m.Destination
			bestSource = m.Source
		}
	}
	if bestDest == "" {
		return containerPath
	}
	return bestSource + strings.TrimPrefix(containerPath, bestDest)
}

// BuildxBuilderName is the named buildx builder Deployik uses for all builds.
// A persistent docker-container driver builder gives us:
//   - cross-build persistence of `RUN --mount=type=cache` (package-manager caches,
//     Next.js incremental build cache, etc.),
//   - a single place to prune cache (`docker buildx prune --builder=...`),
//   - isolation from whatever the host's default buildx config is.
const BuildxBuilderName = "deployik-builder"

// BuildxBuilderMemory caps the BuildKit container itself so a single runaway
// build cannot consume more than this much RAM regardless of per-build
// --memory flags. This is the platform-wide ceiling: per-build caps from the
// ResourceTier table (Nano 1.5 GB → Large 4 GB) must fit inside it.
const (
	BuildxBuilderMemory = "6g"
	BuildxBuilderCPUs   = "4.0"
)

// EnsureBuildxBuilder makes sure the `deployik-builder` named buildx builder
// exists and is started, then caps its container at BuildxBuilderMemory /
// BuildxBuilderCPUs so it cannot OOM-kill neighbor stacks on the VPS. Safe to
// call repeatedly (idempotent).
//
// Lifecycle:
//   - On first call, creates the builder with the docker-container driver and
//     bootstraps the BuildKit container.
//   - On subsequent calls, `buildx inspect --bootstrap` is a no-op when the
//     builder is already running, and will start it if it was stopped.
//   - After bootstrap, `docker update` re-applies the memory/CPU caps on the
//     underlying container (the container name is `buildx_buildkit_<name>0`).
//     This is the only way to bound BuildKit itself — `buildx create --driver-opt`
//     does not expose memory/CPU limits.
//
// Non-fatal on failure: the caller logs a warning and continues; BuildImage
// will surface a clearer error on the next build if the builder truly isn't
// available. The `docker update` cap step is best-effort too — a missing cap
// is preferable to a missing builder, so we never fail boot just because
// resource pinning didn't take.
func (d *DockerClient) EnsureBuildxBuilder(ctx context.Context) error {
	// Fast path: builder already exists → bootstrap starts it if stopped.
	if err := exec.CommandContext(ctx, "docker", "buildx", "inspect", "--bootstrap", BuildxBuilderName).Run(); err != nil {
		create := exec.CommandContext(ctx, "docker", "buildx", "create",
			"--name", BuildxBuilderName,
			"--driver", "docker-container",
			"--bootstrap",
		)
		out, err := create.CombinedOutput()
		if err != nil {
			return fmt.Errorf("create buildx builder %s: %w: %s", BuildxBuilderName, err, strings.TrimSpace(string(out)))
		}
	}

	// Cap the BuildKit container's resource ceiling. Buildx names the
	// underlying container `buildx_buildkit_<builder-name>0`.
	builderContainer := "buildx_buildkit_" + BuildxBuilderName + "0"
	if out, err := exec.CommandContext(ctx, "docker", "update",
		"--memory="+BuildxBuilderMemory,
		"--memory-swap="+BuildxBuilderMemory,
		"--cpus="+BuildxBuilderCPUs,
		builderContainer,
	).CombinedOutput(); err != nil {
		log.Printf("Warning: could not cap buildx builder %s at %s / %s CPUs: %v: %s",
			builderContainer, BuildxBuilderMemory, BuildxBuilderCPUs, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BuildImageOptions carries optional knobs for BuildImage. All fields are optional.
type BuildImageOptions struct {
	// Builder is the buildx builder name. Defaults to BuildxBuilderName.
	Builder string
	// SecretsFile is the path to a KEY=VALUE env-style file whose contents are
	// mounted into the build via `--secret id=deployik-secrets,src=<path>`.
	// The Dockerfile can source it in a RUN step that uses
	// `--mount=type=secret,id=deployik-secrets`. Empty → no secret mount.
	SecretsFile string
	// Tier governs the per-build --memory / --memory-swap / --cpus flags so
	// one project's webpack/turbopack pass cannot exhaust the host. Zero
	// values fall back to the Small tier (2 GB / 2.0 CPUs).
	Tier ResourceTier
	// OnLog receives one build-output line at a time (merged stdout+stderr).
	OnLog func(line string)
}

// BuildImage builds a Docker image via `docker buildx build` and returns the image ID.
// Uses BuildKit unconditionally so user Dockerfiles relying on BuildKit features
// (automatic ARGs like $BUILDPLATFORM, RUN --mount, HEREDOCs, # syntax=, etc.) build
// correctly.
func (d *DockerClient) BuildImage(ctx context.Context, contextDir, dockerfilePath, imageTag string, opts BuildImageOptions) (string, error) {
	dockerfileArg := dockerfilePath
	if dockerfileArg == "" {
		dockerfileArg = "Dockerfile"
	}

	builder := opts.Builder
	if builder == "" {
		builder = BuildxBuilderName
	}

	tier := opts.Tier
	if tier.BuildMemoryMB == 0 {
		tier = Tiers[SmallTier]
	}

	args := []string{
		"buildx", "build",
		"--builder", builder,
		"--load",
		"--progress=plain",
		fmt.Sprintf("--memory=%dm", tier.BuildMemoryMB),
		fmt.Sprintf("--memory-swap=%dm", tier.BuildMemoryMB),
		fmt.Sprintf("--cpus=%.2f", tier.BuildCPUCores),
		"-t", imageTag,
		"-f", dockerfileArg,
	}
	if opts.SecretsFile != "" {
		args = append(args, "--secret", "id=deployik-secrets,src="+opts.SecretsFile)
	}
	args = append(args, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	// Make buildx deterministic about its output: plain progress, no truncation, no colors.
	cmd.Env = append(os.Environ(),
		"DOCKER_BUILDKIT=1",
		"BUILDKIT_PROGRESS=plain",
		"BUILDX_NO_DEFAULT_ATTESTATIONS=1",
		"NO_COLOR=1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start docker buildx: %w", err)
	}

	tail := newLineTail(40)

	var wg sync.WaitGroup
	consume := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			tail.push(line)
			if opts.OnLog != nil {
				opts.OnLog(line)
			}
		}
	}
	wg.Add(2)
	go consume(stdout)
	go consume(stderr)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("docker buildx build failed: %w\n%s", err, tail.join())
	}

	inspect, _, err := d.cli.ImageInspectWithRaw(ctx, imageTag)
	if err != nil {
		return "", fmt.Errorf("inspect built image %s: %w", imageTag, err)
	}
	return inspect.ID, nil
}

// lineTail keeps a bounded ring of the most recent N lines, used to attach
// trailing build output to error messages without unbounded memory growth.
type lineTail struct {
	capacity int
	lines    []string
}

func newLineTail(n int) *lineTail { return &lineTail{capacity: n} }

func (t *lineTail) push(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > t.capacity {
		t.lines = t.lines[len(t.lines)-t.capacity:]
	}
}

func (t *lineTail) join() string { return strings.Join(t.lines, "\n") }

// RunContainerOptions holds optional settings for container creation.
type RunContainerOptions struct {
	ExtraHosts   []string // e.g. []string{"host.docker.internal:host-gateway"}
	BindHostPort bool     // if true, binds the container port to a random localhost port
	VolumeBinds  []string // e.g. []string{"deployik-myapp-preview-data:/app/data"}
	// Port is the TCP port the container listens on (the port Deployik declares
	// as ExposedPorts and, when BindHostPort is set, maps to a random host port).
	// Zero defaults to 3000 — the port our generated Dockerfiles bind to.
	Port int
	// Tier carries the per-project runtime caps (memory, CPU, OOM score).
	// Zero value falls back to the Small tier so a partially-populated caller
	// never accidentally launches an uncapped container.
	Tier ResourceTier
}

// ptrInt64 returns a pointer to v. Used for *int64 fields like PidsLimit.
func ptrInt64(v int64) *int64 { return &v }

// RunContainer starts a container from an image.
// Returns the container ID.
func (d *DockerClient) RunContainer(ctx context.Context, name, imageTag string, envVars []string, networkName string, opts RunContainerOptions) (string, error) {
	port := opts.Port
	if port <= 0 {
		port = 3000
	}
	containerPort := nat.Port(fmt.Sprintf("%d/tcp", port))

	tier := opts.Tier
	if tier.MemoryMB == 0 {
		tier = Tiers[SmallTier]
	}
	memBytes := tier.MemoryMB * 1024 * 1024

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		Resources: container.Resources{
			Memory:            memBytes,
			MemorySwap:        memBytes, // disable swap — fail fast instead of thrashing the host
			MemoryReservation: memBytes / 2,
			CPUQuota:          int64(tier.CPUCores * 100000),
			CPUPeriod:         100000,
			PidsLimit:         ptrInt64(256),
		},
		OomScoreAdj: tier.OomScoreAdj,
		SecurityOpt: []string{"no-new-privileges=true"},
		Tmpfs: map[string]string{
			"/tmp":             "size=64m,noexec,nosuid,nodev",
			"/app/.next/cache": "size=128m,nosuid,nodev",
		},
		ExtraHosts: opts.ExtraHosts,
		Binds:      opts.VolumeBinds,
	}
	if opts.BindHostPort {
		hostConfig.PortBindings = nat.PortMap{
			containerPort: []nat.PortBinding{
				{HostIP: "127.0.0.1", HostPort: "0"},
			},
		}
	}

	// Create container
	resp, err := d.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        imageTag,
			Env:          envVars,
			ExposedPorts: nat.PortSet{containerPort: struct{}{}},
		},
		hostConfig,
		&network.NetworkingConfig{},
		nil,
		name,
	)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	// Connect to the proxy network
	if networkName != "" {
		if err := d.cli.NetworkConnect(ctx, networkName, resp.ID, nil); err != nil {
			log.Printf("Warning: failed to connect to network %s: %v", networkName, err)
		}
	}

	// Start container
	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up created container on start failure
		d.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("start container: %w", err)
	}

	return resp.ID, nil
}

// StopContainer stops and removes a container.
func (d *DockerClient) StopContainer(ctx context.Context, containerID string) error {
	timeout := 30
	stopOpts := container.StopOptions{Timeout: &timeout}
	if err := d.cli.ContainerStop(ctx, containerID, stopOpts); err != nil {
		log.Printf("Warning: stop container %s: %v", containerID[:12], err)
	}

	if err := d.cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}

// WaitForHealthy polls the container until it responds to HTTP or times out.
func (d *DockerClient) WaitForHealthy(ctx context.Context, containerID string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		inspect, err := d.cli.ContainerInspect(ctx, containerID)
		if err != nil {
			return fmt.Errorf("inspect container: %w", err)
		}

		if !inspect.State.Running {
			return fmt.Errorf("container stopped unexpectedly (exit code %d)", inspect.State.ExitCode)
		}

		// If container is running for at least 5 seconds, consider it healthy
		// (Next.js standalone server starts quickly)
		startedAt, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt)
		if err == nil && time.Since(startedAt) > 5*time.Second {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return fmt.Errorf("container did not become healthy within %s", maxWait)
}

// RemoveImage removes a Docker image.
func (d *DockerClient) RemoveImage(ctx context.Context, imageTag string) error {
	_, err := d.cli.ImageRemove(ctx, imageTag, image.RemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("remove image: %w", err)
	}
	return nil
}

// ContainerExists checks if a container with the given name exists.
func (d *DockerClient) ContainerExists(ctx context.Context, name string) (string, bool) {
	inspect, err := d.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", false
	}
	return inspect.ID, true
}

// GetHostPort returns the host-mapped port for the given container port/tcp.
// Only meaningful when the container was started with BindHostPort=true.
// Pass the same Port value that was given to RunContainerOptions; 0 defaults to 3000.
func (d *DockerClient) GetHostPort(ctx context.Context, containerID string, port int) (string, error) {
	if port <= 0 {
		port = 3000
	}
	key := nat.Port(fmt.Sprintf("%d/tcp", port))
	inspect, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container: %w", err)
	}
	bindings, ok := inspect.NetworkSettings.Ports[key]
	if !ok || len(bindings) == 0 {
		return "", fmt.Errorf("no host port binding for container port %d", port)
	}
	return bindings[0].HostPort, nil
}

// EnsureVolume creates a named Docker volume if it doesn't already exist.
// Docker's VolumeCreate is idempotent on name — if a volume with this name
// already exists, this returns its metadata without error. Callers relying on
// "created fresh" semantics must remove first and verify removal succeeded.
func (d *DockerClient) EnsureVolume(ctx context.Context, name string) error {
	_, err := d.cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	return err
}

// VolumesDiskUsage returns volumes keyed by name with UsageData populated.
// Hits /system/df scoped to volumes — this is the only Docker API that
// reports true on-disk size; VolumeInspect and VolumeList leave UsageData
// as nil. The daemon walks the volume directory to compute size, so on a
// busy host with many/large volumes this can take a second or two.
func (d *DockerClient) VolumesDiskUsage(ctx context.Context) (map[string]*volume.Volume, error) {
	du, err := d.cli.DiskUsage(ctx, types.DiskUsageOptions{Types: []types.DiskUsageObject{types.VolumeObject}})
	if err != nil {
		return nil, fmt.Errorf("system df (volumes): %w", err)
	}
	result := make(map[string]*volume.Volume, len(du.Volumes))
	for _, v := range du.Volumes {
		if v == nil {
			continue
		}
		result[v.Name] = v
	}
	return result, nil
}

// RemoveVolume removes a named Docker volume. The returned error can be
// classified with errdefs.IsNotFound (gone already) and errdefs.IsConflict
// (in use by a container). Force=false so we never yank a volume that is
// actively mounted — the caller gets a Conflict and can surface it to the user.
func (d *DockerClient) RemoveVolume(ctx context.Context, name string) error {
	return d.cli.VolumeRemove(ctx, name, false)
}

// Close closes the Docker client.
func (d *DockerClient) Close() error {
	return d.cli.Close()
}
