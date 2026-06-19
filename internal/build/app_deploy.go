package build

import (
	"sort"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// OrderAppMembers groups an app's member projects into the batches a coordinated
// deploy rolls out in sequence. Members within a batch deploy in parallel;
// batches run one after another, each gated on the previous batch's health.
//
//   - deployOrdered = false → one batch with every member (fully parallel,
//     today's per-project behavior, just fanned out).
//   - deployOrdered = true  → batches grouped by ascending DeployOrder; members
//     sharing an order land in the same (parallel) batch. A DB/api member with a
//     lower order thus comes up before the web member that depends on it.
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
