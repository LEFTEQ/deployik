package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchNextConfigInjectsStandaloneIntoTypedConfig(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	configPath := filepath.Join(repoDir, "next.config.ts")
	content := `import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "placehold.co",
      },
    ],
  },
};

export default nextConfig;
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := PatchNextConfig(repoDir); err != nil {
		t.Fatalf("PatchNextConfig returned error: %v", err)
	}

	patched, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	got := string(patched)
	if !strings.Contains(got, "output: 'standalone'") {
		t.Fatalf("expected standalone output to be injected, got:\n%s", got)
	}
	if strings.Count(got, "output: 'standalone'") != 1 {
		t.Fatalf("expected standalone output to be injected once, got:\n%s", got)
	}
}

func TestPatchNextConfigInjectsStandaloneIntoWrappedConfig(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	configPath := filepath.Join(repoDir, "next.config.mjs")
	content := `const nextConfig = withMDX({
  pageExtensions: ["ts", "tsx", "mdx"],
});

export default nextConfig;
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := PatchNextConfig(repoDir); err != nil {
		t.Fatalf("PatchNextConfig returned error: %v", err)
	}

	patched, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	got := string(patched)
	if !strings.Contains(got, "output: 'standalone'") {
		t.Fatalf("expected standalone output to be injected, got:\n%s", got)
	}
}

func TestPatchNextConfigLeavesExistingStandaloneUntouched(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	configPath := filepath.Join(repoDir, "next.config.js")
	content := `module.exports = {
  output: 'standalone',
};
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := PatchNextConfig(repoDir); err != nil {
		t.Fatalf("PatchNextConfig returned error: %v", err)
	}

	patched, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	got := string(patched)
	if got != content {
		t.Fatalf("expected config to remain unchanged, got:\n%s", got)
	}
}
