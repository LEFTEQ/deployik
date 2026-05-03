package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

const screenshotTimeout = 30 * time.Second

// screenshotSemaphore caps how many headless Chrome containers can run at
// once. Each capture spawns a Chrome container that briefly competes for CPU
// and memory; on a single VPS, two in flight is plenty and avoids fan-out
// pressure when many users open dashboards concurrently.
var screenshotSemaphore = NewSemaphore(2)

// AppendBypassToken returns rawURL with `?<param>=<token>` (or `&<param>=...`
// when a query string is already present) appended. When token is empty the
// URL is returned unchanged. Centralized here so both the post-deploy capture
// path and the on-demand capture handler compose URLs identically.
func AppendBypassToken(rawURL, param, token string) string {
	if token == "" {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + param + "=" + token
}

// CaptureScreenshot runs a headless Chrome container to take a screenshot of the given URL.
// Returns the path to the saved PNG file (in deployik-container terms — same
// path used for file IO inside this process). Honours a package-level
// concurrency cap (NewSemaphore(2)); concurrent callers queue on the caller's
// context.
//
// `screenshotDir` is the path inside the deployik container where the PNG
// will be readable after the chrome container exits. `screenshotHostDir` is
// the corresponding path on the Docker host — used as the bind-mount Source
// so the daemon can find it. Pass the same value for both in dev mode (no
// container) or any setup where deployik is NOT itself running in a
// container.
func CaptureScreenshot(ctx context.Context, docker *DockerClient, url, deploymentID, screenshotDir, screenshotHostDir, proxyNetwork string) (string, error) {
	if err := screenshotSemaphore.AcquireCtx(ctx); err != nil {
		return "", fmt.Errorf("queue screenshot: %w", err)
	}
	defer screenshotSemaphore.Release()

	os.MkdirAll(screenshotDir, 0755)

	ctx, cancel := context.WithTimeout(ctx, screenshotTimeout)
	defer cancel()

	containerName := fmt.Sprintf("deployik-screenshot-%s", deploymentID[:8])
	outputFile := deploymentID + ".png"

	bindSource := screenshotHostDir
	if bindSource == "" {
		bindSource = screenshotDir
	}

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
				{Type: mount.TypeBind, Source: bindSource, Target: "/screenshot"},
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
