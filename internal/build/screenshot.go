package build

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

const (
	screenshotTimeout = 30 * time.Second
	screenshotImage   = "zenika/alpine-chrome:latest"
	imagePullTimeout  = 90 * time.Second
)

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

	os.MkdirAll(screenshotDir, 0o755)
	// zenika/alpine-chrome runs as uid 1000 (chrome), but deployik runs as
	// root so the bind-mounted screenshot dir is created 0755-root. Chrome
	// can read but not write, and chrome's `--screenshot=` failure surfaces
	// as container exit code 2. Chmod after MkdirAll handles both fresh
	// installs (where MkdirAll creates the dir) and existing installs (where
	// the dir already exists with the old 0755 mode). Errors are ignored on
	// purpose: a Chmod failure is not fatal here, and the chrome run will
	// still exit cleanly with a clear error if perms remain wrong.
	_ = os.Chmod(screenshotDir, 0o777)

	ctx, cancel := context.WithTimeout(ctx, screenshotTimeout)
	defer cancel()

	containerName := fmt.Sprintf("deployik-screenshot-%s", deploymentID[:8])
	outputFile := deploymentID + ".png"

	bindSource := screenshotHostDir
	if bindSource == "" {
		bindSource = screenshotDir
	}

	createContainer := func() (string, error) {
		resp, err := docker.cli.ContainerCreate(ctx,
			&container.Config{
				Image: screenshotImage,
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
			return "", err
		}
		return resp.ID, nil
	}

	containerID, err := createContainer()
	if err != nil && isImageNotFoundError(err) {
		// First-run on a fresh VPS: the chrome image hasn't been pulled. Pull
		// once and retry. Subsequent captures hit the warm cache.
		log.Printf("Screenshot: pulling %s (first use)", screenshotImage)
		if pullErr := pullScreenshotImage(ctx, docker); pullErr != nil {
			return "", fmt.Errorf("pull screenshot image: %w", pullErr)
		}
		containerID, err = createContainer()
	}
	if err != nil {
		return "", fmt.Errorf("create screenshot container: %w", err)
	}

	if proxyNetwork != "" {
		docker.cli.NetworkConnect(ctx, proxyNetwork, containerID, &network.EndpointSettings{})
	}

	if err := docker.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start screenshot container: %w", err)
	}

	statusCh, errCh := docker.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
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

// isImageNotFoundError detects "No such image" failures from ContainerCreate
// in a way that survives error wrapping (errdefs.IsNotFound trips on the
// concrete daemon error, but the SDK can return wrapped variants too).
func isImageNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "No such image") || strings.Contains(msg, "no such image")
}

// pullScreenshotImage pulls the headless-chrome image. The pull stream must
// be drained for Docker to actually complete the operation, even if we don't
// surface progress to the caller.
func pullScreenshotImage(ctx context.Context, docker *DockerClient) error {
	pullCtx, cancel := context.WithTimeout(ctx, imagePullTimeout)
	defer cancel()
	rc, err := docker.cli.ImagePull(pullCtx, screenshotImage, image.PullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("drain image pull: %w", err)
	}
	return nil
}
