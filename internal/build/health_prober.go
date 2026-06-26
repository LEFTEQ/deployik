package build

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

// MemberProbe is the result of a live container probe for one member.
type MemberProbe struct {
	Probed  bool // false => could not determine (docker error) => caller treats as "unknown"
	Running bool // container exists and is in the running state
	OK      bool // health endpoint responded with an up status (200/204/3xx/401/403)
}

// HealthProber probes a member project's canonical container for an environment.
type HealthProber interface {
	Probe(ctx context.Context, project db.Project, environment string) MemberProbe
}

// DockerHealthProber probes a member's canonical container over the Docker
// network (PROXY_TYPE=docker) or via its host port (PROXY_TYPE=host-port).
type DockerHealthProber struct {
	docker    *DockerClient
	proxyType string
	client    *http.Client
}

// NewDockerHealthProber builds a prober with a short per-probe HTTP timeout.
func NewDockerHealthProber(docker *DockerClient, proxyType string) *DockerHealthProber {
	return &DockerHealthProber{
		docker:    docker,
		proxyType: proxyType,
		client:    &http.Client{Timeout: 2 * time.Second},
	}
}

func (p *DockerHealthProber) Probe(ctx context.Context, project db.Project, environment string) MemberProbe {
	name := db.DeploymentContainerName(project.Name, environment, nil)
	id, exists := p.docker.ContainerExists(ctx, name)
	if !exists {
		return MemberProbe{Probed: true, Running: false}
	}
	running, err := p.docker.IsContainerRunning(ctx, id)
	if err != nil {
		return MemberProbe{Probed: false}
	}
	if !running {
		return MemberProbe{Probed: true, Running: false}
	}

	port := project.Port
	if port <= 0 {
		port = 3000
	}
	// NOTE: empty HealthPath defaults to "/". node-api runtimes serve health at
	// "/health"; ensure such members store HealthPath (set at create time) or
	// extend this to projectconfig.DefaultHealthPath.
	healthPath := project.HealthPath
	if healthPath == "" {
		healthPath = "/"
	}

	var target string
	if p.proxyType == "host-port" {
		hostPort, err := p.docker.GetHostPort(ctx, id, port)
		if err != nil || hostPort == "" {
			// Running but we can't resolve the port — count running as up rather
			// than falsely degraded.
			return MemberProbe{Probed: true, Running: true, OK: true}
		}
		target = fmt.Sprintf("http://127.0.0.1:%s%s", hostPort, healthPath)
	} else {
		target = fmt.Sprintf("http://%s:%d%s", name, port, healthPath)
	}
	return MemberProbe{Probed: true, Running: true, OK: p.httpUp(ctx, target)}
}

// httpUp treats 200/204, any 3xx, and 401/403 as up (mirrors the devops blackbox
// http_app_up module — password-protected 401 is healthy). 5xx / errors are down.
func (p *DockerHealthProber) httpUp(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	s := resp.StatusCode
	return s == 200 || s == 204 || (s >= 300 && s < 400) || s == 401 || s == 403
}
