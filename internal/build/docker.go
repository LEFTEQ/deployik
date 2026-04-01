package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
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
func (d *DockerClient) BuildImage(ctx context.Context, contextDir, imageTag string, onLog func(line string)) (string, error) {
	// Create tar archive of the build context
	tar, err := archive.TarWithOptions(contextDir, &archive.TarOptions{})
	if err != nil {
		return "", fmt.Errorf("create tar: %w", err)
	}
	defer tar.Close()

	resp, err := d.cli.ImageBuild(ctx, tar, types.ImageBuildOptions{
		Tags:       []string{imageTag},
		Dockerfile: "Dockerfile",
		Remove:     true,
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

// RunContainer starts a container from an image.
// Returns the container ID.
func (d *DockerClient) RunContainer(ctx context.Context, name, imageTag string, envVars []string, networkName string) (string, error) {
	// Create container
	resp, err := d.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        imageTag,
			Env:          envVars,
			ExposedPorts: nat.PortSet{"3000/tcp": struct{}{}},
		},
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		},
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

// Close closes the Docker client.
func (d *DockerClient) Close() error {
	return d.cli.Close()
}
