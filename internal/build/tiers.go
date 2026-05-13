package build

// ResourceTier carries the runtime + build-phase resource caps a project picks
// in its Settings → Resources page. Both the buildx --memory / --cpus flags
// and the Docker HostConfig.Resources on the deployed container read from a
// single tier per deploy so build and runtime never drift.
type ResourceTier struct {
	Name          string  // canonical identifier persisted in projects.resource_tier
	Label         string  // human label shown in the UI ("Nano", "Small", ...)
	Description   string  // one-line UI hint
	MemoryMB      int64   // runtime container memory ceiling (hard limit, no swap)
	CPUCores      float64 // runtime CPU quota (CPUQuota = CPUCores * 100000)
	BuildMemoryMB int64   // --memory and --memory-swap passed to `docker buildx build`
	BuildCPUCores float64 // --cpus passed to `docker buildx build`
	OomScoreAdj   int     // kernel OOM kill priority: positive = killed sooner
}

// SmallTier is the default for new projects and the backfill value for
// migration 021. 512 MB / 1.0 CPU exactly matches the previous hardcoded
// values in RunContainer, so existing containers continue running unchanged
// until their next deploy.
const SmallTier = "small"

// Tiers is the source of truth for tier metadata. The frontend mirrors this
// table in web/src/lib/deployment-helpers.ts; a tier_metadata_test cross-checks
// the two so they cannot drift silently.
var Tiers = map[string]ResourceTier{
	"nano": {
		Name:          "nano",
		Label:         "Nano",
		Description:   "Static sites and very light pages.",
		MemoryMB:      256,
		CPUCores:      0.5,
		BuildMemoryMB: 1536,
		BuildCPUCores: 1.0,
		OomScoreAdj:   200,
	},
	"small": {
		Name:          "small",
		Label:         "Small",
		Description:   "Default. Most marketing sites and small apps.",
		MemoryMB:      512,
		CPUCores:      1.0,
		BuildMemoryMB: 2048,
		BuildCPUCores: 2.0,
		OomScoreAdj:   0,
	},
	"medium": {
		Name:          "medium",
		Label:         "Medium",
		Description:   "Real Next.js apps with moderate traffic.",
		MemoryMB:      1024,
		CPUCores:      2.0,
		BuildMemoryMB: 3072,
		BuildCPUCores: 2.0,
		OomScoreAdj:   0,
	},
	"large": {
		Name:          "large",
		Label:         "Large",
		Description:   "Heavy workloads with bigger Node heaps.",
		MemoryMB:      2048,
		CPUCores:      2.0,
		BuildMemoryMB: 4096,
		BuildCPUCores: 2.0,
		OomScoreAdj:   0,
	},
}

// TierOrDefault returns the named tier or falls back to Small. Used at the
// edge where a project row might carry an empty string (defensive — the DB
// CHECK constraint + DEFAULT 'small' should make this unreachable in
// production, but we never want a deploy to crash because of a missing tier).
func TierOrDefault(name string) ResourceTier {
	if t, ok := Tiers[name]; ok {
		return t
	}
	return Tiers[SmallTier]
}

// IsValidTier reports whether name matches a known tier. The API handler
// rejects invalid values with HTTP 400 — the DB CHECK is the final backstop.
func IsValidTier(name string) bool {
	_, ok := Tiers[name]
	return ok
}
