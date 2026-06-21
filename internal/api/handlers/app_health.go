package handlers

import "github.com/LEFTEQ/lovinka-deployik/internal/db"

// Member live-status vocabulary (worst-of contributes to the combined status).
const (
	memberStatusHealthy   = "healthy"
	memberStatusDeploying = "deploying"
	memberStatusDegraded  = "degraded"
	memberStatusFailed    = "failed"
	memberStatusDown      = "down"
	memberStatusNone      = "none"
	memberStatusUnknown   = "unknown"
)

// deriveMemberLiveStatusFromDeployment is the P1 (DB-only) status source: it
// maps a member's latest deployment to a coarse live status. P2 refines this
// with a real container probe.
func deriveMemberLiveStatusFromDeployment(latest *db.Deployment) string {
	if latest == nil {
		return memberStatusNone
	}
	switch latest.Status {
	case "live":
		return memberStatusHealthy
	case "queued", "building", "deploying":
		return memberStatusDeploying
	case "failed":
		return memberStatusFailed
	default: // rolled_back, replaced, or anything unexpected
		return memberStatusDegraded
	}
}

// statusSeverity ranks a member status for the worst-of roll-up and maps it to
// the combined-status vocabulary (healthy|deploying|degraded|down|none).
func statusSeverity(s string) (int, string) {
	switch s {
	case memberStatusDown:
		return 4, "down"
	case memberStatusDegraded, memberStatusFailed, memberStatusUnknown:
		return 3, "degraded"
	case memberStatusDeploying:
		return 2, "deploying"
	case memberStatusHealthy:
		return 1, "healthy"
	default: // none / unrecognized
		return 0, "none"
	}
}

// combinedAppStatus returns the worst-of member statuses as a combined app
// status. Empty input -> "none".
func combinedAppStatus(memberStatuses []string) string {
	best := -1
	combined := "none"
	for _, s := range memberStatuses {
		sev, label := statusSeverity(s)
		if sev > best {
			best = sev
			combined = label
		}
	}
	return combined
}
