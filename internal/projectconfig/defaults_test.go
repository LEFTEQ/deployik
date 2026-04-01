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
	if settings.InstallCommand != "bun install" {
		t.Fatalf("install_command = %q, want bun install", settings.InstallCommand)
	}
	if settings.NodeVersion != "22" {
		t.Fatalf("node_version = %q, want 22", settings.NodeVersion)
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
