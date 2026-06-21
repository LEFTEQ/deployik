package build

import (
	"context"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
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
