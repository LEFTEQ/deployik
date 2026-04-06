package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

const screenshotTimeout = 30 * time.Second

// CaptureScreenshot runs a headless Chrome container to take a screenshot of the given URL.
// Returns the path to the saved PNG file.
func CaptureScreenshot(ctx context.Context, docker *DockerClient, url, deploymentID, screenshotDir, proxyNetwork string) (string, error) {
	os.MkdirAll(screenshotDir, 0755)

	ctx, cancel := context.WithTimeout(ctx, screenshotTimeout)
	defer cancel()

	containerName := fmt.Sprintf("deployik-screenshot-%s", deploymentID[:8])
	outputFile := deploymentID + ".png"

	resp, err := docker.cli.ContainerCreate(ctx,
		&container.Config{
			Image: "zenika/alpine-chrome:latest",
			Cmd: []string{
				"--no-sandbox",
				"--disable-gpu",
				"--screenshot=/screenshot/" + outputFile,
				"--window-size=1280,800",
				"--hide-scrollbars",
				url,
			},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{Type: mount.TypeBind, Source: screenshotDir, Target: "/screenshot"},
			},
			AutoRemove: true,
		},
		nil, nil, containerName,
	)
	if err != nil {
		return "", fmt.Errorf("create screenshot container: %w", err)
	}

	if proxyNetwork != "" {
		docker.cli.NetworkConnect(ctx, proxyNetwork, resp.ID, &network.EndpointSettings{})
	}

	if err := docker.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start screenshot container: %w", err)
	}

	statusCh, errCh := docker.cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return "", fmt.Errorf("wait screenshot container: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return "", fmt.Errorf("screenshot container exited with code %d", status.StatusCode)
		}
	}

	finalPath := filepath.Join(screenshotDir, outputFile)
	if _, err := os.Stat(finalPath); err != nil {
		return "", fmt.Errorf("screenshot file not found: %w", err)
	}
	return finalPath, nil
}
