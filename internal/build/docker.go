package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
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

// BuildStreamLine represents a single line from docker build output.
type BuildStreamLine struct {
	Stream string `json:"stream"`
	Error  string `json:"error"`
}

// BuildImage builds a Docker image from a directory containing a Dockerfile.
// Returns the image ID and calls onLog for each build output line.
func (d *DockerClient) BuildImage(ctx context.Context, contextDir, dockerfilePath, imageTag string, onLog func(line string)) (string, error) {
	// Create tar archive of the build context
	tar, err := archive.TarWithOptions(contextDir, &archive.TarOptions{})
	if err != nil {
		return "", fmt.Errorf("create tar: %w", err)
	}
	defer tar.Close()

	relDockerfile := "Dockerfile"
	if dockerfilePath != "" {
		relativePath, err := filepath.Rel(contextDir, dockerfilePath)
		if err == nil && !strings.HasPrefix(relativePath, "..") {
			relDockerfile = filepath.ToSlash(relativePath)
		}
		if relDockerfile == "" || relDockerfile == "." {
			relDockerfile = "Dockerfile"
		}
	}

	resp, err := d.cli.ImageBuild(ctx, tar, types.ImageBuildOptions{
		Tags:        []string{imageTag},
		Dockerfile:  relDockerfile,
		Remove:      true,
		ForceRemove: true,
	})
	if err != nil {
		return "", fmt.Errorf("image build: %w", err)
	}
	defer resp.Body.Close()

	var imageID string
	decoder := json.NewDecoder(resp.Body)
	for {
		var line BuildStreamLine
		if err := decoder.Decode(&line); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("decode build output: %w", err)
		}

		if line.Error != "" {
			return "", fmt.Errorf("build error: %s", line.Error)
		}

		if line.Stream != "" {
			trimmed := strings.TrimRight(line.Stream, "\n")
			if trimmed != "" {
				if onLog != nil {
					onLog(trimmed)
				}
				// Try to extract image ID from build output
				if strings.HasPrefix(trimmed, "Successfully built ") {
					imageID = strings.TrimPrefix(trimmed, "Successfully built ")
				}
			}
		}
	}

	// If we didn't get an ID from the stream, inspect the tag
	if imageID == "" {
		inspect, _, err := d.cli.ImageInspectWithRaw(ctx, imageTag)
		if err == nil {
			imageID = inspect.ID
		}
	}

	return imageID, nil
}

// RunContainerOptions holds optional settings for container creation.
type RunContainerOptions struct {
	ExtraHosts   []string // e.g. []string{"host.docker.internal:host-gateway"}
	BindHostPort bool     // if true, binds container port 3000 to a random localhost port
	VolumeBinds  []string // e.g. []string{"deployik-myapp-preview-data:/app/data"}
}

// RunContainer starts a container from an image.
// Returns the container ID.
func (d *DockerClient) RunContainer(ctx context.Context, name, imageTag string, envVars []string, networkName string, opts RunContainerOptions) (string, error) {
	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		Resources: container.Resources{
			Memory:    512 * 1024 * 1024, // 512MB
			CPUQuota:  100000,            // 1.0 CPU
			CPUPeriod: 100000,
		},
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
			"3000/tcp": []nat.PortBinding{
				{HostIP: "127.0.0.1", HostPort: "0"},
			},
		}
	}

	// Create container
	resp, err := d.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        imageTag,
			Env:          envVars,
			ExposedPorts: nat.PortSet{"3000/tcp": struct{}{}},
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

// GetHostPort returns the host-mapped port for container port 3000/tcp.
// Only meaningful when the container was started with BindHostPort=true.
func (d *DockerClient) GetHostPort(ctx context.Context, containerID string) (string, error) {
	inspect, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container: %w", err)
	}
	bindings, ok := inspect.NetworkSettings.Ports["3000/tcp"]
	if !ok || len(bindings) == 0 {
		return "", fmt.Errorf("no host port binding for container port 3000")
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
