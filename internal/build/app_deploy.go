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
