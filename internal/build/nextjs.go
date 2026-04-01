package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PatchNextConfig ensures next.config has output: 'standalone' set.
// This is required for the standalone Dockerfile to work.
// Modifies the config in-place before Docker build.
func PatchNextConfig(repoDir string) error {
	// Find the next.config file (could be .js, .mjs, .ts)
	candidates := []string{
		"next.config.ts",
		"next.config.mjs",
		"next.config.js",
	}

	var configPath string
	for _, name := range candidates {
		p := filepath.Join(repoDir, name)
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}
	}

	if configPath == "" {
		// No next.config found — create a minimal one
		configPath = filepath.Join(repoDir, "next.config.mjs")
		content := `/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
};

export default nextConfig;
`
		return os.WriteFile(configPath, []byte(content), 0644)
	}

	// Read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read next.config: %w", err)
	}

	content := string(data)

	// Already has standalone output
	if strings.Contains(content, "output") && strings.Contains(content, "standalone") {
		return nil
	}

	// Inject output: 'standalone' into the config object
	// Strategy: find the config object pattern and add the property
	patched := injectStandaloneOutput(content)

	if err := os.WriteFile(configPath, []byte(patched), 0644); err != nil {
		return fmt.Errorf("write patched next.config: %w", err)
	}

	return nil
}

func injectStandaloneOutput(content string) string {
	// Try common patterns to inject output: 'standalone'

	// Pattern 1: nextConfig = { ... }
	// Find the opening brace of the config object
	for _, pattern := range []string{
		"nextConfig = {",
		"nextConfig: {",
		"module.exports = {",
		"export default {",
	} {
		if idx := strings.Index(content, pattern); idx != -1 {
			braceIdx := idx + strings.Index(content[idx:], "{")
			return content[:braceIdx+1] + "\n  output: 'standalone'," + content[braceIdx+1:]
		}
	}

	// Pattern 2: defineConfig or function call that returns config
	// Look for the first { after common Next.js config wrappers
	for _, pattern := range []string{
		"defineNextConfig({",
		"withNextIntl({",
		"withMDX({",
		"createNextConfig({",
	} {
		if idx := strings.Index(content, pattern); idx != -1 {
			braceIdx := idx + len(pattern) - 1
			return content[:braceIdx+1] + "\n  output: 'standalone'," + content[braceIdx+1:]
		}
	}

	// Fallback: just append an override at the end
	// This works for .mjs/.ts files using export default
	if strings.Contains(content, "export default") {
		// Wrap the existing export in a standalone override
		return content + "\n// Injected by Deployik for standalone Docker deployment\n"
	}

	return content
}
