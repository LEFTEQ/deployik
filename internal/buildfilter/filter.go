// Package buildfilter decides, at GitHub-push time, whether a project should
// rebuild given the set of changed paths. It is the trigger-time half of the
// "fan-out fix": a monorepo push that touches only one app's files should not
// rebuild every project bound to the repo.
//
// Fail-safe = build. If filtering is disabled, or the changed-file set is
// unavailable (GitHub truncates pushes > 2000 files; non-push/synthetic
// events), the project always builds — the filter only ever *skips* a build
// when it is certain no relevant path changed.
package buildfilter

import (
	"regexp"
	"strings"
)

// ShouldBuild reports whether a project should rebuild for a push, plus a short
// human reason logged on the trigger record.
//
//	filterEnabled=false            → build ("filter disabled")
//	listAvailable=false            → build ("file list unavailable")
//	a changed path under rootDir   → build ("changed path under root")
//	a changed path matches a glob  → build ("changed path matches watch glob")
//	otherwise                      → skip  ("no changed paths under root or watch globs")
func ShouldBuild(filterEnabled bool, rootDir string, watchPaths, changedPaths []string, listAvailable bool) (bool, string) {
	if !filterEnabled {
		return true, "filter disabled"
	}
	if !listAvailable {
		return true, "file list unavailable"
	}
	root := normalizeRoot(rootDir)
	for _, changed := range changedPaths {
		changed = strings.TrimSpace(strings.TrimPrefix(changed, "/"))
		if changed == "" {
			continue
		}
		if underRoot(root, changed) {
			return true, "changed path under root"
		}
		for _, glob := range watchPaths {
			if matchGlob(strings.TrimSpace(glob), changed) {
				return true, "changed path matches watch glob"
			}
		}
	}
	return false, "no changed paths under root or watch globs"
}

// normalizeRoot strips leading/trailing slashes. "" (repo root) matches every
// path.
func normalizeRoot(rootDir string) string {
	return strings.Trim(strings.TrimSpace(rootDir), "/")
}

// underRoot is segment-aware: root "apps/web" matches "apps/web" and
// "apps/web/x" but NOT "apps/webextra". Empty root matches everything.
func underRoot(root, changed string) bool {
	if root == "" {
		return true
	}
	return changed == root || strings.HasPrefix(changed, root+"/")
}

// matchGlob matches a changed path against a glob supporting:
//   - "**" — any number of path segments (crosses "/")
//   - "*"  — any run of characters within a single segment (no "/")
//   - "?"  — a single non-"/" character
//
// All other characters match literally. Patterns are fully anchored.
func matchGlob(pattern, path string) bool {
	re, err := regexp.Compile("^" + globToRegexp(pattern) + "$")
	if err != nil {
		return false
	}
	return re.MatchString(path)
}

func globToRegexp(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*") // ** crosses path separators
				i++
			} else {
				b.WriteString("[^/]*") // * stays within a segment
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}
