package buildfilter

import "testing"

func TestShouldBuild(t *testing.T) {
	tests := []struct {
		name          string
		filterEnabled bool
		rootDir       string
		watchPaths    []string
		changed       []string
		listAvailable bool
		wantBuild     bool
	}{
		{
			name:          "filter disabled always builds",
			filterEnabled: false,
			rootDir:       "apps/web",
			changed:       []string{"apps/api/main.go"},
			listAvailable: true,
			wantBuild:     true,
		},
		{
			name:          "file list unavailable fails safe to build",
			filterEnabled: true,
			rootDir:       "apps/web",
			changed:       nil,
			listAvailable: false,
			wantBuild:     true,
		},
		{
			name:          "changed path under root builds",
			filterEnabled: true,
			rootDir:       "apps/web",
			changed:       []string{"apps/web/src/page.tsx"},
			listAvailable: true,
			wantBuild:     true,
		},
		{
			name:          "changed path outside root and no globs skips",
			filterEnabled: true,
			rootDir:       "apps/web",
			changed:       []string{"apps/api/main.go"},
			listAvailable: true,
			wantBuild:     false,
		},
		{
			name:          "watch glob double-star matches shared package",
			filterEnabled: true,
			rootDir:       "apps/web",
			watchPaths:    []string{"packages/shared/**"},
			changed:       []string{"packages/shared/util.ts"},
			listAvailable: true,
			wantBuild:     true,
		},
		{
			name:          "watch glob exact file matches lockfile",
			filterEnabled: true,
			rootDir:       "apps/web",
			watchPaths:    []string{"bun.lock"},
			changed:       []string{"bun.lock"},
			listAvailable: true,
			wantBuild:     true,
		},
		{
			name:          "empty root dir matches any changed path",
			filterEnabled: true,
			rootDir:       "",
			changed:       []string{"anything/here.go"},
			listAvailable: true,
			wantBuild:     true,
		},
		{
			name:          "root dir exact file change builds",
			filterEnabled: true,
			rootDir:       "apps/web",
			changed:       []string{"apps/web"},
			listAvailable: true,
			wantBuild:     true,
		},
		{
			name:          "root prefix is segment-aware (apps/web does not match apps/webextra)",
			filterEnabled: true,
			rootDir:       "apps/web",
			changed:       []string{"apps/webextra/x.ts"},
			listAvailable: true,
			wantBuild:     false,
		},
		{
			name:          "no changed paths at all skips when filtering",
			filterEnabled: true,
			rootDir:       "apps/web",
			changed:       []string{},
			listAvailable: true,
			wantBuild:     false,
		},
		{
			name:          "single-star glob stays within a segment",
			filterEnabled: true,
			rootDir:       "apps/web",
			watchPaths:    []string{"config/*.yml"},
			changed:       []string{"config/db/prod.yml"},
			listAvailable: true,
			wantBuild:     false,
		},
		{
			name:          "leading-slash root dir is normalized",
			filterEnabled: true,
			rootDir:       "/apps/web/",
			changed:       []string{"apps/web/src/page.tsx"},
			listAvailable: true,
			wantBuild:     true,
		},
		{
			name:          "leading-slash watch glob is normalized to match changed paths",
			filterEnabled: true,
			rootDir:       "apps/web",
			watchPaths:    []string{"/packages/shared/**"},
			changed:       []string{"packages/shared/util.ts"},
			listAvailable: true,
			wantBuild:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			build, reason := ShouldBuild(tc.filterEnabled, tc.rootDir, tc.watchPaths, tc.changed, tc.listAvailable)
			if build != tc.wantBuild {
				t.Fatalf("ShouldBuild = %v (%q), want %v", build, reason, tc.wantBuild)
			}
			if reason == "" {
				t.Fatal("expected a non-empty reason")
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"packages/shared/**", "packages/shared/a/b.ts", true},
		{"packages/shared/**", "packages/shared", false},
		{"packages/shared/**", "packages/other/a.ts", false},
		{"**/*.lock", "a/b/c.lock", true},
		{"*.lock", "bun.lock", true},
		{"*.lock", "a/bun.lock", false},
		{"bun.lock", "bun.lock", true},
		{"config/*.yml", "config/app.yml", true},
		{"config/*.yml", "config/db/app.yml", false},
	}
	for _, c := range cases {
		if got := matchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}
