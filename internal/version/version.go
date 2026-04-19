// Package version exposes build-time version metadata baked into the binary
// via -ldflags. Constructed in main.go from package-level vars and passed to
// the API layer through RouterConfig.
package version

import "fmt"

// Info is the resolved, presentation-ready build metadata. It is safe to
// marshal directly into JSON responses.
type Info struct {
	GitSHA     string `json:"git_sha"`      // short (7-char) SHA for display; falls back to whatever was injected if shorter
	GitSHAFull string `json:"git_sha_full"` // full SHA, used for the commit URL and as a stable identifier
	BuildTime  string `json:"build_time"`   // RFC3339-ish timestamp of the commit (or build start) — display only
	GHRepo     string `json:"gh_repo"`      // "owner/name", e.g. "lefteq/lovinka-deployik"
	GHRunID    string `json:"gh_run_id"`    // GitHub Actions run id; empty for local/PR builds
	CommitURL  string `json:"commit_url"`   // built server-side; empty when sha or repo is missing
	RunURL     string `json:"run_url"`      // built server-side; empty when run id or repo is missing
}

// New constructs an Info from raw build-injected strings. URL fields are
// derived here so the SPA never has to know GitHub URL templates.
func New(gitSHA, buildTime, ghRunID, ghRepo string) *Info {
	short := gitSHA
	if len(short) > 7 {
		short = short[:7]
	}

	info := &Info{
		GitSHA:     short,
		GitSHAFull: gitSHA,
		BuildTime:  buildTime,
		GHRepo:     ghRepo,
		GHRunID:    ghRunID,
	}

	if ghRepo != "" && len(gitSHA) >= 7 && gitSHA != "dev" {
		info.CommitURL = fmt.Sprintf("https://github.com/%s/commit/%s", ghRepo, gitSHA)
	}
	if ghRepo != "" && ghRunID != "" {
		info.RunURL = fmt.Sprintf("https://github.com/%s/actions/runs/%s", ghRepo, ghRunID)
	}

	return info
}

// IsDev reports whether this binary was built without a real git SHA
// (typically a local `make dev-api` run). Callers can use this to suppress
// links that would 404 on github.com.
func (i *Info) IsDev() bool {
	return i.GitSHAFull == "" || i.GitSHAFull == "dev"
}
