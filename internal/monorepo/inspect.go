package monorepo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/sync/errgroup"
)

// RepoInspector is the narrow interface the inspector needs from a GitHub
// client. Defined here (not in internal/github) so this package stays
// dependency-free and easily mockable. internal/github.Client will satisfy
// it after Task 2 lands.
type RepoInspector interface {
	// GetTree returns a flat list of repo paths at the given ref. Each entry
	// is a forward-slash relative path without a leading slash. Directories
	// and files are both included; callers don't need to distinguish.
	// truncated is true when the provider returned a partial result.
	GetTree(ctx context.Context, owner, repo, ref string) (paths []string, truncated bool, err error)

	// GetFileContent returns the raw file bytes at the given ref, or
	// (nil, ErrFileNotFound) if the path doesn't exist. Other errors are
	// propagated.
	GetFileContent(ctx context.Context, owner, repo, ref, filePath string) ([]byte, error)
}

// ErrFileNotFound is returned by RepoInspector.GetFileContent when the
// requested path is absent at the given ref. Inspect treats this as a
// soft signal, not a hard failure.
var ErrFileNotFound = errors.New("monorepo: file not found")

// maxConcurrentAppFetches caps how many apps are inspected in parallel.
const maxConcurrentAppFetches = 5

// Inspect runs the full detection pipeline against a repo at a given ref.
// It always returns a non-nil Report on success, even for non-monorepo
// repos (in which case Apps has a single entry with Path="").
func Inspect(ctx context.Context, gh RepoInspector, owner, repo, ref string) (*Report, error) {
	// 1. Fetch the tree once.
	tree, treeTruncated, err := gh.GetTree(ctx, owner, repo, ref)
	if err != nil {
		return nil, err
	}

	// 2. Build rootFiles set from top-level entries (no "/" in path).
	rootFiles := make(map[string]bool)
	for _, p := range tree {
		if !strings.Contains(p, "/") {
			rootFiles[p] = true
		}
	}

	// 3. Detect package manager + tooling from rootFiles.
	packageManager := DetectPackageManager(rootFiles)
	tooling := DetectTooling(rootFiles)

	// 4. Fetch root package.json and pnpm-workspace.yaml — pre-check rootFiles
	// to skip round-trips for files that definitely aren't there.
	var rootPkg rootPackageJSON
	if rootFiles["package.json"] {
		if rootPkgContent, fetchErr := gh.GetFileContent(ctx, owner, repo, ref, "package.json"); fetchErr == nil {
			parsed, parseErr := ParseRootPackageJSON(rootPkgContent)
			if parseErr == nil {
				rootPkg = parsed
			}
			// On parse error: treat as empty root package (continue gracefully)
		}
	}

	var pnpmGlobs []string
	if rootFiles["pnpm-workspace.yaml"] {
		if pnpmContent, fetchErr := gh.GetFileContent(ctx, owner, repo, ref, "pnpm-workspace.yaml"); fetchErr == nil {
			globs, parseErr := ParsePnpmWorkspaceYAML(pnpmContent)
			if parseErr != nil {
				// Malformed pnpm-workspace.yaml → graceful fallback: treat as non-monorepo
				report := singleAppReport(packageManager, tooling, rootPkg)
				report.Truncated = treeTruncated
				return report, nil
			}
			pnpmGlobs = globs
		} else if !errors.Is(fetchErr, ErrFileNotFound) {
			return nil, fetchErr
		}
	}

	// 5. Compute workspace globs (pnpm + root package.json workspaces, deduped).
	allGlobs := dedupeGlobs(pnpmGlobs, rootPkg.Workspaces.Globs())

	// 6. ExpandGlobs against tree to get app paths.
	appPaths := ExpandGlobs(allGlobs, tree)

	// 7. IsMonorepo determination.
	if len(allGlobs) == 0 || len(appPaths) == 0 {
		report := singleAppReport(packageManager, tooling, rootPkg)
		report.Truncated = treeTruncated
		return report, nil
	}

	// 8. Build a per-app file presence set so we skip fetches for absent files.
	// Keys are the file name relative to each app dir (e.g. "vite.config.ts").
	perAppFiles := buildPerAppFileSets(tree, appPaths)

	// 9. Fetch each app's package.json + optional vite config in parallel.
	apps := make([]App, len(appPaths))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentAppFetches)
	for i, appPath := range appPaths {
		i, appPath := i, appPath
		g.Go(func() error {
			app, err := inspectApp(gctx, gh, owner, repo, ref, appPath, packageManager, tooling, perAppFiles[appPath])
			if err != nil {
				return err
			}
			apps[i] = app
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Filter out zero-value slots (apps whose package.json was missing/unparseable).
	validApps := apps[:0]
	for _, app := range apps {
		if app.Name != "" || app.Path != "" {
			validApps = append(validApps, app)
		}
	}

	// Apps are already in sorted order (appPaths is sorted by ExpandGlobs).
	return &Report{
		IsMonorepo:     true,
		PackageManager: packageManager,
		Tooling:        tooling,
		Apps:           validApps,
		Truncated:      treeTruncated,
	}, nil
}

// singleAppReport builds a Report for a non-monorepo repo.
func singleAppReport(packageManager string, tooling []Tooling, rootPkg rootPackageJSON) *Report {
	appPkg := AppPackageJSON{
		Name:            rootPkg.Name,
		Scripts:         rootPkg.Scripts,
		Dependencies:    rootPkg.Dependencies,
		DevDependencies: rootPkg.DevDependencies,
	}
	app := DeriveAppProfile("", appPkg, nil, packageManager, tooling)
	return &Report{
		IsMonorepo:     false,
		PackageManager: packageManager,
		Tooling:        tooling,
		Apps:           []App{app},
	}
}

// dedupeGlobs merges two glob slices, preserving order and removing duplicates.
func dedupeGlobs(a, b []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(a)+len(b))
	for _, g := range append(a, b...) {
		if !seen[g] {
			seen[g] = true
			result = append(result, g)
		}
	}
	return result
}

// buildPerAppFileSets builds a map from app path → set of filenames present
// directly inside that app directory (one level deep, no subdirectories).
func buildPerAppFileSets(tree []string, appPaths []string) map[string]map[string]bool {
	result := make(map[string]map[string]bool, len(appPaths))
	for _, ap := range appPaths {
		result[ap] = make(map[string]bool)
	}
	for _, p := range tree {
		slash := strings.LastIndex(p, "/")
		if slash < 0 {
			continue
		}
		dir := p[:slash]
		file := p[slash+1:]
		if files, ok := result[dir]; ok {
			files[file] = true
		}
	}
	return result
}

// inspectApp fetches and profiles a single monorepo app. On missing/unparseable
// package.json it returns a zero-value App so the caller can filter it out.
func inspectApp(ctx context.Context, gh RepoInspector, owner, repo, ref, appPath, packageManager string, tooling []Tooling, appFiles map[string]bool) (App, error) {
	pkgContent, fetchErr := gh.GetFileContent(ctx, owner, repo, ref, appPath+"/package.json")
	if fetchErr != nil {
		// Missing package.json: return zero-value; caller will filter.
		if errors.Is(fetchErr, ErrFileNotFound) {
			return App{}, nil
		}
		return App{}, fetchErr
	}
	var appPkg AppPackageJSON
	if jsonErr := json.Unmarshal(pkgContent, &appPkg); jsonErr != nil {
		return App{}, nil
	}

	// Fetch vite config only when vite is present AND the file exists in the tree.
	var viteConfigContent []byte
	if hasViteDep(appPkg) {
		viteConfigContent = fetchFirstViteConfigPreChecked(ctx, gh, owner, repo, ref, appPath, appFiles)
	}

	app := DeriveAppProfile(appPath, appPkg, viteConfigContent, packageManager, tooling)
	return app, nil
}

// hasViteDep checks if an app package.json has vite as a dep or devDep.
func hasViteDep(pkg AppPackageJSON) bool {
	if _, ok := pkg.Dependencies["vite"]; ok {
		return true
	}
	if _, ok := pkg.DevDependencies["vite"]; ok {
		return true
	}
	return false
}

// viteConfigExtensions lists the vite config file names to probe, in priority order.
var viteConfigExtensions = []string{
	"vite.config.ts",
	"vite.config.js",
	"vite.config.mjs",
	"vite.config.cjs",
}

// fetchFirstViteConfigPreChecked fetches the first vite config that actually
// exists in appFiles (as determined from the tree), skipping extensions that
// aren't present. Falls back to fetchFirstViteConfig when appFiles is nil.
func fetchFirstViteConfigPreChecked(ctx context.Context, gh RepoInspector, owner, repo, ref, appPath string, appFiles map[string]bool) []byte {
	if appFiles == nil {
		return fetchFirstViteConfig(ctx, gh, owner, repo, ref, appPath)
	}
	for _, ext := range viteConfigExtensions {
		if !appFiles[ext] {
			continue
		}
		content, fetchErr := gh.GetFileContent(ctx, owner, repo, ref, appPath+"/"+ext)
		if fetchErr == nil {
			return content
		}
	}
	return nil
}

// fetchFirstViteConfig tries to fetch the first existing vite config file for an app.
func fetchFirstViteConfig(ctx context.Context, gh RepoInspector, owner, repo, ref, appPath string) []byte {
	for _, ext := range viteConfigExtensions {
		filePath := appPath + "/" + ext
		content, fetchErr := gh.GetFileContent(ctx, owner, repo, ref, filePath)
		if fetchErr == nil {
			return content
		}
		if !errors.Is(fetchErr, ErrFileNotFound) {
			// Transient error on this extension; try remaining extensions.
			continue
		}
	}
	return nil
}
