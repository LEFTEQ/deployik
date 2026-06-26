package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

// AppSiblingEnv returns runtime env vars that expose each sibling member's
// internal base URL + host on the app's private network, so a member reaches
// its siblings by container name without hardcoding. For a sibling "acme-api"
// (port 4000) on production it emits:
//
//	ACME_API_URL=http://deployik-acme-api-production:4000
//	ACME_API_HOST=deployik-acme-api-production
//	ACME_API_PORT=4000
//
// selfID is excluded. Names are upper-snaked (hyphens → underscores). These are
// injected at the LOWEST precedence so a user-set app/project var of the same
// key always wins. Uses the canonical (production / default-preview) container
// name — cross-member discovery targets that naming, not per-branch preview
// instances.
func AppSiblingEnv(selfID string, members []db.Project, environment string) []string {
	var out []string
	for _, sibling := range members {
		if sibling.ID == selfID || sibling.Status == "deleted" {
			continue
		}
		prefix := envVarPrefix(sibling.Name)
		if prefix == "" {
			continue
		}
		host := db.DeploymentContainerName(sibling.Name, environment, nil)
		port := sibling.Port
		if port <= 0 {
			port = 3000
		}
		out = append(out,
			fmt.Sprintf("%s_URL=http://%s:%d", prefix, host, port),
			fmt.Sprintf("%s_HOST=%s", prefix, host),
			fmt.Sprintf("%s_PORT=%d", prefix, port),
		)
	}
	return out
}

// envVarPrefix upper-snakes a project name for use as an env var prefix.
func envVarPrefix(name string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(name)) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// OrderAppMembers groups an app's member projects into the batches a coordinated
// deploy rolls out. RunRollout deploys members within a batch one after another
// (the build semaphore serializes builds anyway), and batches act as health
// barriers — every member of batch N must come up healthy before batch N+1 starts.
//
//   - deployOrdered = false → one batch with every member (no inter-member
//     ordering, today's per-project behavior, just fanned out under one release).
//   - deployOrdered = true  → batches grouped by ascending DeployOrder; members
//     sharing an order land in the same batch. A db/api member with a lower order
//     thus comes up (and is health-gated) before the web member that depends on it.
//
// Stable within a batch (by name) so the rollout order is deterministic.
func OrderAppMembers(members []db.Project, deployOrdered bool) [][]db.Project {
	if len(members) == 0 {
		return nil
	}
	if !deployOrdered {
		batch := make([]db.Project, len(members))
		copy(batch, members)
		sort.SliceStable(batch, func(i, j int) bool { return batch[i].Name < batch[j].Name })
		return [][]db.Project{batch}
	}

	sorted := make([]db.Project, len(members))
	copy(sorted, members)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].DeployOrder != sorted[j].DeployOrder {
			return sorted[i].DeployOrder < sorted[j].DeployOrder
		}
		return sorted[i].Name < sorted[j].Name
	})

	var batches [][]db.Project
	var current []db.Project
	for i, m := range sorted {
		if i > 0 && m.DeployOrder != sorted[i-1].DeployOrder {
			batches = append(batches, current)
			current = nil
		}
		current = append(current, m)
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// MemberDeployResult is one member's outcome in a coordinated rollout.
type MemberDeployResult struct {
	ProjectID    string
	DeploymentID string
	Healthy      bool
}

// RolloutResult is the outcome of a coordinated app rollout.
type RolloutResult struct {
	Succeeded bool
	// Deployed lists every member that ran and reached healthy/live, in rollout
	// order — these are the ones swapped in (and the set to roll back on failure).
	Deployed []MemberDeployResult
	// FailedProjectID is the first member that failed (empty when Succeeded).
	FailedProjectID string
}

// RunRollout deploys batches in order, halting at the first unhealthy member.
// deployFn deploys one member and reports the deployment it created and whether
// that member reached healthy/live (blue-green only swaps on health, so an
// unhealthy member leaves the prior container serving). Members in a batch are
// deployed in slice order; a failure stops the remaining members and batches.
//
// The Docker-dependent work lives in deployFn; this coordination is pure and
// unit-tested with a fake deployFn (mirrors the codebase's fakeRunner pattern).
func RunRollout(batches [][]db.Project, deployFn func(p db.Project) (deploymentID string, healthy bool)) RolloutResult {
	var res RolloutResult
	for _, batch := range batches {
		for _, member := range batch {
			deploymentID, healthy := deployFn(member)
			if !healthy {
				res.FailedProjectID = member.ID
				res.Succeeded = false
				return res
			}
			res.Deployed = append(res.Deployed, MemberDeployResult{
				ProjectID:    member.ID,
				DeploymentID: deploymentID,
				Healthy:      true,
			})
		}
	}
	res.Succeeded = true
	return res
}
