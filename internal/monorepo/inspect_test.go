package monorepo

import (
	"context"
	"reflect"
	"testing"
)

// fakeInspector implements RepoInspector using in-memory data.
type fakeInspector struct {
	tree      []string
	truncated bool
	files     map[string][]byte
}

func (f *fakeInspector) GetTree(_ context.Context, _, _, _ string) ([]string, bool, error) {
	return f.tree, f.truncated, nil
}

func (f *fakeInspector) GetFileContent(_ context.Context, _, _, _, filePath string) ([]byte, error) {
	content, ok := f.files[filePath]
	if !ok {
		return nil, ErrFileNotFound
	}
	return content, nil
}

func TestInspect(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		gh       *fakeInspector
		expected Report
	}{
		{
			// Test 1: forge-shaped pnpm + Turborepo
			name: "pnpm turborepo monorepo",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"pnpm-workspace.yaml",
					"turbo.json",
					"pnpm-lock.yaml",
					"apps/web/package.json",
					"apps/web/src/index.ts",
					"apps/web/vite.config.ts",
					"packages/ui/package.json",
				},
				files: map[string][]byte{
					"package.json":          []byte(fixturePnpmTurboRootPkg),
					"pnpm-workspace.yaml":   []byte(fixturePnpmWorkspaceYAML),
					"apps/web/package.json": []byte(fixtureForgeWebPkg),
					"apps/web/vite.config.ts": []byte(fixtureViteConfigBasic),
				},
			},
			expected: Report{
				IsMonorepo:     true,
				PackageManager: "pnpm",
				Tooling:        []Tooling{ToolingTurborepo},
				Apps: []App{
					{
						Name:                  "@forge/web",
						Path:                  "apps/web",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "pnpm turbo run build --filter=@forge/web",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 2: Vite outDir override
			name: "vite outdir override",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"pnpm-workspace.yaml",
					"turbo.json",
					"pnpm-lock.yaml",
					"apps/web/package.json",
					"apps/web/vite.config.ts",
				},
				files: map[string][]byte{
					"package.json":          []byte(fixturePnpmTurboRootPkg),
					"pnpm-workspace.yaml":   []byte(fixturePnpmWorkspaceYAML),
					"apps/web/package.json": []byte(fixtureForgeWebPkg),
					"apps/web/vite.config.ts": []byte(fixtureViteConfigOutDir),
				},
			},
			expected: Report{
				IsMonorepo:     true,
				PackageManager: "pnpm",
				Tooling:        []Tooling{ToolingTurborepo},
				Apps: []App{
					{
						Name:                  "@forge/web",
						Path:                  "apps/web",
						Framework:             "vite",
						OutputDirectory:       "public-dist",
						SuggestedBuildCommand: "pnpm turbo run build --filter=@forge/web",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 3: plain pnpm workspace, no Turborepo, Next.js app
			name: "pnpm workspace no turborepo nextjs",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"pnpm-workspace.yaml",
					"pnpm-lock.yaml",
					"apps/site/package.json",
					"apps/site/pages/index.tsx",
				},
				files: map[string][]byte{
					"package.json":           []byte(fixturePnpmNoTurboRootPkg),
					"pnpm-workspace.yaml":    []byte(fixturePnpmWorkspaceYAML),
					"apps/site/package.json": []byte(fixtureNextjsAppPkg),
				},
			},
			expected: Report{
				IsMonorepo:     true,
				PackageManager: "pnpm",
				Tooling:        []Tooling{},
				Apps: []App{
					{
						Name:                  "site",
						Path:                  "apps/site",
						Framework:             "nextjs",
						OutputDirectory:       ".next",
						SuggestedBuildCommand: "pnpm run build",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 4: npm workspaces
			name: "npm workspaces",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"package-lock.json",
					"apps/admin/package.json",
					"apps/admin/src/index.ts",
				},
				files: map[string][]byte{
					"package.json":           []byte(fixtureNpmWorkspacesRootPkg),
					"apps/admin/package.json": []byte(fixtureViteAppPkg),
				},
			},
			expected: Report{
				IsMonorepo:     true,
				PackageManager: "npm",
				Tooling:        []Tooling{},
				Apps: []App{
					{
						Name:                  "admin",
						Path:                  "apps/admin",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "npm run build",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 5: Bun workspaces, object shape
			name: "bun workspaces object shape",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"bun.lockb",
					"apps/x/package.json",
					"apps/x/src/index.ts",
					"apps/x/vite.config.ts",
				},
				files: map[string][]byte{
					"package.json":       []byte(fixtureBunWorkspacesObjectPkg),
					"apps/x/package.json": []byte(fixtureBunAppVitePkg),
					"apps/x/vite.config.ts": []byte(fixtureViteConfigBasic),
				},
			},
			expected: Report{
				IsMonorepo:     true,
				PackageManager: "bun",
				Tooling:        []Tooling{},
				Apps: []App{
					{
						Name:                  "bun-app-x",
						Path:                  "apps/x",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "bun run build",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 6: Nx workspace
			name: "nx workspace",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"nx.json",
					"pnpm-lock.yaml",
					"apps/dashboard/package.json",
				},
				files: map[string][]byte{
					"package.json":                []byte(fixtureNxRootPkg),
					"apps/dashboard/package.json": []byte(fixtureNxAppPkg),
				},
			},
			expected: Report{
				IsMonorepo:     true,
				PackageManager: "pnpm",
				Tooling:        []Tooling{ToolingNx},
				Apps: []App{
					{
						Name:                  "@myorg/dashboard",
						Path:                  "apps/dashboard",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "pnpm nx build @myorg/dashboard",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 7: single-app no-monorepo
			name: "single app no monorepo",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"vite.config.ts",
					"src/index.ts",
				},
				files: map[string][]byte{
					"package.json": []byte(fixtureSingleViteAppPkg),
				},
			},
			expected: Report{
				IsMonorepo:     false,
				PackageManager: "auto",
				Tooling:        []Tooling{},
				Apps: []App{
					{
						Name:                  "my-vite-app",
						Path:                  "",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "bun run build",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 8: malformed pnpm-workspace.yaml → graceful fallback to single-app
			name: "malformed pnpm workspace yaml",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"pnpm-workspace.yaml",
					"pnpm-lock.yaml",
				},
				files: map[string][]byte{
					"package.json":        []byte(fixtureSingleViteAppPkg),
					"pnpm-workspace.yaml": []byte(fixtureMalformedYAML),
				},
			},
			expected: Report{
				IsMonorepo:     false,
				PackageManager: "pnpm",
				Tooling:        []Tooling{},
				Apps: []App{
					{
						Name:                  "my-vite-app",
						Path:                  "",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "pnpm run build",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 9: no buildable script
			name: "no build script",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"src/index.ts",
				},
				files: map[string][]byte{
					"package.json": []byte(fixtureNoBuildScriptPkg),
				},
			},
			expected: Report{
				IsMonorepo:     false,
				PackageManager: "auto",
				Tooling:        []Tooling{},
				Apps: []App{
					{
						Name:                  "no-build",
						Path:                  "",
						Framework:             "static",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "",
						Buildable:             false,
					},
				},
			},
		},
		{
			// Test 10: tree truncation flag is surfaced in Report
			name: "tree truncation surfaced in report",
			gh: &fakeInspector{
				truncated: true,
				tree: []string{
					"package.json",
					"src/index.ts",
				},
				files: map[string][]byte{
					"package.json": []byte(fixtureSingleViteAppPkg),
				},
			},
			expected: Report{
				IsMonorepo:     false,
				PackageManager: "auto",
				Tooling:        []Tooling{},
				Truncated:      true,
				Apps: []App{
					{
						Name:                  "my-vite-app",
						Path:                  "",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "bun run build",
						Buildable:             true,
					},
				},
			},
		},
		{
			// Test 11: hidden directories ignored
			name: "hidden directories not matched by globs",
			gh: &fakeInspector{
				tree: []string{
					"package.json",
					"pnpm-workspace.yaml",
					"pnpm-lock.yaml",
					"apps/web/package.json",
					// These should be ignored:
					".next/cache/package.json",
					".turbo/runs/package.json",
					"apps/.hidden/package.json",
				},
				files: map[string][]byte{
					"package.json":        []byte(fixturePnpmNoTurboRootPkg),
					"pnpm-workspace.yaml": []byte(fixturePnpmWorkspaceYAML),
					"apps/web/package.json": []byte(fixtureUnnamedViteAppPkg),
					// Hidden dirs exist in file map but should not be fetched
					".next/cache/package.json":  []byte(`{"name":"should-not-appear"}`),
					".turbo/runs/package.json":  []byte(`{"name":"should-not-appear"}`),
					"apps/.hidden/package.json": []byte(`{"name":"should-not-appear"}`),
				},
			},
			expected: Report{
				IsMonorepo:     true,
				PackageManager: "pnpm",
				Tooling:        []Tooling{},
				Apps: []App{
					{
						Name:                  "web",
						Path:                  "apps/web",
						Framework:             "vite",
						OutputDirectory:       "dist",
						SuggestedBuildCommand: "pnpm run build",
						Buildable:             true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := Inspect(ctx, tt.gh, "owner", "repo", "main")
			if err != nil {
				t.Fatalf("Inspect returned error: %v", err)
			}
			if report == nil {
				t.Fatal("Inspect returned nil report")
			}

			if report.IsMonorepo != tt.expected.IsMonorepo {
				t.Errorf("IsMonorepo: got %v, want %v", report.IsMonorepo, tt.expected.IsMonorepo)
			}
			if report.PackageManager != tt.expected.PackageManager {
				t.Errorf("PackageManager: got %q, want %q", report.PackageManager, tt.expected.PackageManager)
			}
			if !reflect.DeepEqual(report.Tooling, tt.expected.Tooling) {
				t.Errorf("Tooling: got %v, want %v", report.Tooling, tt.expected.Tooling)
			}
			if report.Truncated != tt.expected.Truncated {
				t.Errorf("Truncated: got %v, want %v", report.Truncated, tt.expected.Truncated)
			}
			if len(report.Apps) != len(tt.expected.Apps) {
				t.Errorf("Apps length: got %d, want %d\ngot apps: %+v", len(report.Apps), len(tt.expected.Apps), report.Apps)
				return
			}
			for i, app := range report.Apps {
				want := tt.expected.Apps[i]
				if app.Name != want.Name {
					t.Errorf("Apps[%d].Name: got %q, want %q", i, app.Name, want.Name)
				}
				if app.Path != want.Path {
					t.Errorf("Apps[%d].Path: got %q, want %q", i, app.Path, want.Path)
				}
				if app.Framework != want.Framework {
					t.Errorf("Apps[%d].Framework: got %q, want %q", i, app.Framework, want.Framework)
				}
				if app.OutputDirectory != want.OutputDirectory {
					t.Errorf("Apps[%d].OutputDirectory: got %q, want %q", i, app.OutputDirectory, want.OutputDirectory)
				}
				if app.SuggestedBuildCommand != want.SuggestedBuildCommand {
					t.Errorf("Apps[%d].SuggestedBuildCommand: got %q, want %q", i, app.SuggestedBuildCommand, want.SuggestedBuildCommand)
				}
				if app.Buildable != want.Buildable {
					t.Errorf("Apps[%d].Buildable: got %v, want %v", i, app.Buildable, want.Buildable)
				}
			}
		})
	}
}

// --- Embedded fixtures ---

const fixturePnpmWorkspaceYAML = `packages:
  - "apps/*"
  - "packages/*"
`

const fixturePnpmTurboRootPkg = `{
  "name": "forge-root",
  "packageManager": "pnpm@9.15.4",
  "devDependencies": {
    "turbo": "^2.0.0"
  }
}`

const fixturePnpmNoTurboRootPkg = `{
  "name": "myrepo-root",
  "packageManager": "pnpm@9.0.0"
}`

const fixtureForgeWebPkg = `{
  "name": "@forge/web",
  "scripts": {
    "build": "tsc -b && vite build",
    "dev": "vite"
  },
  "dependencies": {
    "react": "^19.0.0"
  },
  "devDependencies": {
    "vite": "^6.0.0",
    "@vitejs/plugin-react": "^4.0.0"
  }
}`

const fixtureViteConfigBasic = `import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    sourcemap: true,
  },
})
`

const fixtureViteConfigOutDir = `import { defineConfig } from 'vite'

export default defineConfig({
  build: {
    outDir: 'public-dist',
  },
})
`

const fixtureNextjsAppPkg = `{
  "name": "site",
  "scripts": {
    "build": "next build",
    "dev": "next dev"
  },
  "dependencies": {
    "next": "^15.0.0",
    "react": "^19.0.0"
  }
}`

const fixtureNpmWorkspacesRootPkg = `{
  "name": "npm-monorepo",
  "workspaces": ["apps/*"]
}`

const fixtureViteAppPkg = `{
  "name": "admin",
  "scripts": {
    "build": "vite build"
  },
  "devDependencies": {
    "vite": "^5.0.0"
  }
}`

const fixtureBunWorkspacesObjectPkg = `{
  "name": "bun-monorepo",
  "workspaces": {
    "packages": ["apps/*"]
  }
}`

const fixtureBunAppVitePkg = `{
  "name": "bun-app-x",
  "scripts": {
    "build": "vite build"
  },
  "devDependencies": {
    "vite": "^5.0.0"
  }
}`

const fixtureNxRootPkg = `{
  "name": "nx-monorepo",
  "workspaces": ["apps/*"]
}`

const fixtureNxAppPkg = `{
  "name": "@myorg/dashboard",
  "scripts": {
    "build": "nx build @myorg/dashboard"
  },
  "devDependencies": {
    "vite": "^5.0.0"
  }
}`

const fixtureSingleViteAppPkg = `{
  "name": "my-vite-app",
  "scripts": {
    "build": "vite build",
    "dev": "vite"
  },
  "devDependencies": {
    "vite": "^5.0.0"
  }
}`

const fixtureMalformedYAML = `packages:
  - "apps/*"
  bad yaml: [unclosed bracket
  : invalid: : key
`

const fixtureNoBuildScriptPkg = `{
  "name": "no-build",
  "scripts": {
    "dev": "node index.js"
  }
}`

// fixtureUnnamedViteAppPkg is a vite app with no name field (name falls back to path.Base).
const fixtureUnnamedViteAppPkg = `{
  "scripts": {
    "build": "vite build"
  },
  "devDependencies": {
    "vite": "^5.0.0"
  }
}`
