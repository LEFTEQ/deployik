package monorepo

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lefteq/lovinka-deployik/internal/projectconfig"
)

// DetectPackageManager determines the package manager from root-level file presence.
// Priority: pnpm-workspace.yaml/pnpm-lock.yaml > bun.lockb/bun.lock > yarn.lock >
// package-lock.json > "auto".
func DetectPackageManager(rootFiles map[string]bool) string {
	if rootFiles["pnpm-workspace.yaml"] || rootFiles["pnpm-lock.yaml"] {
		return "pnpm"
	}
	if rootFiles["bun.lockb"] || rootFiles["bun.lock"] {
		return "bun"
	}
	if rootFiles["yarn.lock"] {
		return "yarn"
	}
	if rootFiles["package-lock.json"] {
		return "npm"
	}
	return "auto"
}

// DetectTooling returns the build orchestrators present at the repo root.
// Returns an empty (non-nil) slice when none are detected.
// Order is deterministic: turborepo first, then nx.
func DetectTooling(rootFiles map[string]bool) []Tooling {
	result := make([]Tooling, 0)
	if rootFiles["turbo.json"] {
		result = append(result, ToolingTurborepo)
	}
	if rootFiles["nx.json"] {
		result = append(result, ToolingNx)
	}
	return result
}

// pnpmWorkspaceYAML is the structure of pnpm-workspace.yaml.
type pnpmWorkspaceYAML struct {
	Packages []string `yaml:"packages"`
}

// ParsePnpmWorkspaceYAML parses pnpm-workspace.yaml content and returns the
// packages glob list. Returns (nil, nil) for empty/missing content or missing
// packages key. Returns an error only on actual YAML parse failure.
func ParsePnpmWorkspaceYAML(content []byte) ([]string, error) {
	if len(content) == 0 {
		return nil, nil
	}
	var ws pnpmWorkspaceYAML
	if err := yaml.Unmarshal(content, &ws); err != nil {
		return nil, err
	}
	if len(ws.Packages) == 0 {
		return nil, nil
	}
	return ws.Packages, nil
}

// rawWorkspaces handles both JSON shapes of the workspaces field:
//   - []string: ["apps/*", "packages/*"]
//   - object: { "packages": ["apps/*"] }
type rawWorkspaces struct {
	globs []string
}

func (rw *rawWorkspaces) UnmarshalJSON(data []byte) error {
	// Try array first
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		rw.globs = arr
		return nil
	}
	// Try object shape: { "packages": [...] }
	var obj struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		rw.globs = obj.Packages
		return nil
	}
	// Ignore unrecognized shapes
	return nil
}

// Globs returns the workspace glob patterns, or nil if none were present.
func (rw rawWorkspaces) Globs() []string {
	return rw.globs
}

// RootPackageJSON represents the relevant fields of the root package.json.
type rootPackageJSON struct {
	Name            string            `json:"name"`
	PackageManager  string            `json:"packageManager"` // e.g. "pnpm@9.15.4"
	Workspaces      rawWorkspaces     `json:"workspaces"`
	Scripts         map[string]string `json:"scripts"`
	DevDependencies map[string]string `json:"devDependencies"`
	Dependencies    map[string]string `json:"dependencies"`
}

// ParseRootPackageJSON parses the root package.json into a rootPackageJSON struct.
func ParseRootPackageJSON(content []byte) (rootPackageJSON, error) {
	var pkg rootPackageJSON
	if err := json.Unmarshal(content, &pkg); err != nil {
		return rootPackageJSON{}, err
	}
	return pkg, nil
}

// ExpandGlobs expands workspace glob patterns against a flat list of repo
// paths, returning sorted unique app directory paths.
// Supports: apps/* (single level), apps/** (recursive), and literal paths.
// Skips any path segment starting with '.' (e.g. .git, .next, .turbo).
func ExpandGlobs(globs []string, treePaths []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, glob := range globs {
		glob = strings.TrimSpace(glob)
		if glob == "" {
			continue
		}

		// Normalize: remove trailing slash and /**
		recursive := false
		if strings.HasSuffix(glob, "/**") {
			recursive = true
			glob = strings.TrimSuffix(glob, "/**")
		} else if strings.HasSuffix(glob, "/*") {
			glob = strings.TrimSuffix(glob, "/*")
			// single-level wildcard: apps/* → find apps/<X>/package.json
		} else {
			// Literal path: treat as exact app dir
			// Check if <glob>/package.json exists in treePaths
			target := glob + "/package.json"
			for _, tp := range treePaths {
				if tp == target && !hasHiddenSegment(glob) {
					if !seen[glob] {
						seen[glob] = true
						result = append(result, glob)
					}
				}
			}
			continue
		}

		prefix := glob + "/"
		for _, tp := range treePaths {
			if !strings.HasPrefix(tp, prefix) {
				continue
			}
			// tp is something like: apps/web/package.json or apps/web/src/index.ts
			rest := tp[len(prefix):]
			if rest == "" {
				continue
			}

			var appDir string
			if recursive {
				// For **, find any package.json under the prefix
				if !strings.HasSuffix(tp, "/package.json") {
					continue
				}
				// The app dir is everything except the trailing /package.json
				appDir = tp[:len(tp)-len("/package.json")]
			} else {
				// Single level: apps/web/package.json → apps/web
				// rest should be exactly "<name>/package.json"
				parts := strings.SplitN(rest, "/", 2)
				if len(parts) != 2 || parts[1] != "package.json" {
					continue
				}
				appDir = glob + "/" + parts[0]
			}

			if hasHiddenSegment(appDir) {
				continue
			}
			if !seen[appDir] {
				seen[appDir] = true
				result = append(result, appDir)
			}
		}
	}

	sort.Strings(result)
	return result
}

// hasHiddenSegment returns true if any path segment starts with '.'.
func hasHiddenSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// AppPackageJSON represents the relevant fields of a per-app package.json.
type AppPackageJSON struct {
	Name            string            `json:"name"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

var viteOutDirRe = regexp.MustCompile(`\boutDir\s*:\s*["']([^"']+)["']`)

// ExtractViteOutDir performs a best-effort regex extraction of the outDir
// string literal from a vite config file. Returns "" when not found or when
// the value is not a string literal.
func ExtractViteOutDir(viteConfigContent []byte) string {
	m := viteOutDirRe.FindSubmatch(viteConfigContent)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// DeriveAppProfile produces an App from a per-app package.json and optional
// vite config content.
func DeriveAppProfile(pkgPath string, pkg AppPackageJSON, viteConfigContent []byte, manager string, tooling []Tooling) App {
	// 1. Name
	name := pkg.Name
	if name == "" {
		if pkgPath == "" {
			name = "root"
		} else {
			name = path.Base(pkgPath)
		}
	}

	// 2. Buildable
	buildable := pkg.Scripts["build"] != ""

	// 3. Framework detection
	allDeps := make(map[string]bool)
	for k := range pkg.Dependencies {
		allDeps[k] = true
	}
	for k := range pkg.DevDependencies {
		allDeps[k] = true
	}

	var framework string
	switch {
	case allDeps["next"]:
		framework = "nextjs"
	case allDeps["vite"]:
		framework = "vite"
	case allDeps["astro"]:
		framework = "astro"
	default:
		framework = "static"
	}
	outputDirectory := projectconfig.DefaultOutputDirectory(framework)

	// 4. Vite outDir override
	if framework == "vite" && viteConfigContent != nil {
		if outDir := ExtractViteOutDir(viteConfigContent); outDir != "" {
			outputDirectory = outDir
		}
	}

	// 5. Suggested build command
	var suggestedBuildCommand string
	if buildable {
		hasTurbo := false
		hasNx := false
		for _, t := range tooling {
			if t == ToolingTurborepo {
				hasTurbo = true
			}
			if t == ToolingNx {
				hasNx = true
			}
		}

		effectiveManager := manager
		if effectiveManager == "auto" {
			effectiveManager = "pnpm"
		}

		switch {
		case hasTurbo && pkg.Name != "":
			suggestedBuildCommand = effectiveManager + " turbo run build --filter=" + pkg.Name
		case hasNx && pkg.Name != "":
			suggestedBuildCommand = effectiveManager + " nx build " + pkg.Name
		default:
			// Delegate to projectconfig for per-manager default build commands.
			suggestedBuildCommand = projectconfig.DefaultBuildCommand(manager)
		}
	}

	return App{
		Name:                  name,
		Path:                  pkgPath,
		Framework:             framework,
		OutputDirectory:       outputDirectory,
		SuggestedBuildCommand: suggestedBuildCommand,
		Buildable:             buildable,
	}
}
