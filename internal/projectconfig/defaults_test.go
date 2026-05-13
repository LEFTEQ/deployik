package projectconfig

import (
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestResolveDefaultsNextJSProject(t *testing.T) {
	t.Parallel()

	project := &db.Project{Framework: "nextjs"}

	settings, err := Resolve(project)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if settings.Framework != FrameworkNextJS {
		t.Fatalf("framework = %q, want %q", settings.Framework, FrameworkNextJS)
	}
	if settings.Runtime != RuntimeNextJSStandalone {
		t.Fatalf("runtime = %q, want %q", settings.Runtime, RuntimeNextJSStandalone)
	}
	if settings.OutputDirectory != ".next" {
		t.Fatalf("output_directory = %q, want .next", settings.OutputDirectory)
	}
	if settings.BuildCommand != "bun run build" {
		t.Fatalf("build_command = %q, want bun run build", settings.BuildCommand)
	}
	if settings.InstallCommand != "bun install --frozen-lockfile" {
		t.Fatalf("install_command = %q, want bun install --frozen-lockfile", settings.InstallCommand)
	}
	if settings.NodeVersion != "22" {
		t.Fatalf("node_version = %q, want 22", settings.NodeVersion)
	}
	if settings.PackageManager != PackageManagerAuto {
		t.Fatalf("package_manager = %q, want %q", settings.PackageManager, PackageManagerAuto)
	}
}

func TestResolveDefaultsStaticProject(t *testing.T) {
	t.Parallel()

	project := &db.Project{
		Framework:       "vite",
		RootDirectory:   "apps/web",
		OutputDirectory: "",
	}

	settings, err := Resolve(project)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if settings.Framework != FrameworkVite {
		t.Fatalf("framework = %q, want %q", settings.Framework, FrameworkVite)
	}
	if settings.Runtime != RuntimeStatic {
		t.Fatalf("runtime = %q, want %q", settings.Runtime, RuntimeStatic)
	}
	if settings.RootDirectory != "apps/web" {
		t.Fatalf("root_directory = %q, want apps/web", settings.RootDirectory)
	}
	if settings.OutputDirectory != "dist" {
		t.Fatalf("output_directory = %q, want dist", settings.OutputDirectory)
	}
}

func TestResolveDefaultsUsesSelectedPackageManager(t *testing.T) {
	t.Parallel()

	project := &db.Project{
		Framework:      "nextjs",
		PackageManager: "pnpm",
	}

	settings, err := Resolve(project)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if settings.PackageManager != PackageManagerPnpm {
		t.Fatalf("package_manager = %q, want %q", settings.PackageManager, PackageManagerPnpm)
	}
	if settings.InstallCommand != "pnpm install --frozen-lockfile" {
		t.Fatalf("install_command = %q, want pnpm install --frozen-lockfile", settings.InstallCommand)
	}
	if settings.BuildCommand != "pnpm run build" {
		t.Fatalf("build_command = %q, want pnpm run build", settings.BuildCommand)
	}
}

func TestResolveSyncsCommandsWhenPackageManagerChanges(t *testing.T) {
	t.Parallel()

	// Simulate switching from bun/auto to npm: old bun commands should be
	// replaced with npm defaults.
	project := &db.Project{
		Framework:      "nextjs",
		PackageManager: "npm",
		InstallCommand: "bun install --frozen-lockfile",
		BuildCommand:   "bun run build",
	}

	settings, err := Resolve(project)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if settings.InstallCommand != "npm ci" {
		t.Fatalf("install_command = %q, want %q", settings.InstallCommand, "npm ci")
	}
	if settings.BuildCommand != "npm run build" {
		t.Fatalf("build_command = %q, want %q", settings.BuildCommand, "npm run build")
	}
}

func TestResolvePreservesCustomCommands(t *testing.T) {
	t.Parallel()

	project := &db.Project{
		Framework:      "nextjs",
		PackageManager: "npm",
		InstallCommand: "npm ci --legacy-peer-deps",
		BuildCommand:   "npm run build -- --profile",
	}

	settings, err := Resolve(project)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if settings.InstallCommand != "npm ci --legacy-peer-deps" {
		t.Fatalf("install_command = %q, want %q", settings.InstallCommand, "npm ci --legacy-peer-deps")
	}
	if settings.BuildCommand != "npm run build -- --profile" {
		t.Fatalf("build_command = %q, want %q", settings.BuildCommand, "npm run build -- --profile")
	}
}

func TestResolveRejectsEscapingProjectPaths(t *testing.T) {
	t.Parallel()

	project := &db.Project{
		Framework:       "nextjs",
		RootDirectory:   "../apps/web",
		OutputDirectory: ".next",
	}

	if _, err := Resolve(project); err == nil {
		t.Fatal("Resolve should reject escaping paths")
	}
}

func TestNormalizeFrameworkNodeAPI(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"node-api":  FrameworkNodeAPI,
		"NODE-API":  FrameworkNodeAPI,
		" node-api": FrameworkNodeAPI,
	}
	for input, want := range cases {
		if got := NormalizeFramework(input); got != want {
			t.Errorf("NormalizeFramework(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRuntimeForFrameworkNodeAPI(t *testing.T) {
	t.Parallel()

	if got := RuntimeForFramework(FrameworkNodeAPI); got != RuntimeNodeAPI {
		t.Errorf("RuntimeForFramework(node-api) = %q, want %q", got, RuntimeNodeAPI)
	}
	if got := RuntimeForFramework(FrameworkVite); got != RuntimeStatic {
		t.Errorf("RuntimeForFramework(vite) = %q, want %q (regression check)", got, RuntimeStatic)
	}
}
