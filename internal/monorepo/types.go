package monorepo

// Tooling identifies a build orchestrator detected at the repo root.
type Tooling string

const (
	ToolingTurborepo Tooling = "turborepo"
	ToolingNx        Tooling = "nx"
)

// Report is the result of inspecting a repository for monorepo structure
// and per-app build profiles.
type Report struct {
	IsMonorepo     bool      `json:"is_monorepo"`
	PackageManager string    `json:"package_manager"` // "auto", "bun", "pnpm", "npm", "yarn"
	Tooling        []Tooling `json:"tooling"`         // empty slice when none detected
	Apps           []App     `json:"apps"`            // always non-empty; single entry with path="" for non-monorepo
	Truncated      bool      `json:"truncated"`       // true when GitHub's tree API returned truncated=true (very large repos)
}

// App is a buildable package inside the repo. For non-monorepo repos a single
// App with Path="" is returned representing the root.
type App struct {
	Name                  string `json:"name"`                    // package.json "name" or path basename if missing
	Path                  string `json:"path"`                    // relative to repo root, forward slashes, no leading slash
	Framework             string `json:"framework"`               // "nextjs" | "vite" | "astro" | "static"
	OutputDirectory       string `json:"output_directory"`        // e.g. "dist", ".next", or vite.config build.outDir
	SuggestedBuildCommand string `json:"suggested_build_command"` // package-manager-aware, Turborepo/Nx-aware
	Buildable             bool   `json:"buildable"`               // false when no "build" script
}
