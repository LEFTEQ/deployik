package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var standaloneOutputPattern = regexp.MustCompile(`(?m)\boutput\s*:\s*['"]standalone['"]`)

var nextConfigObjectPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)\b(?:const|let|var)\s+nextConfig\s*(?::[^=\n]+)?=\s*{`),
	regexp.MustCompile(`(?m)\b(?:const|let|var)\s+nextConfig\s*(?::[^=\n]+)?=\s*(?:[A-Za-z_$][\w$.]*\(\s*)+{`),
	regexp.MustCompile(`(?m)module\.exports\s*=\s*{`),
	regexp.MustCompile(`(?m)module\.exports\s*=\s*(?:[A-Za-z_$][\w$.]*\(\s*)+{`),
	regexp.MustCompile(`(?m)export\s+default\s*{`),
	regexp.MustCompile(`(?m)export\s+default\s+(?:[A-Za-z_$][\w$.]*\(\s*)+{`),
}

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
	if standaloneOutputPattern.MatchString(content) {
		return nil
	}

	// Inject output: 'standalone' into the config object
	// Strategy: find the config object pattern and add the property.
	patched, ok := injectStandaloneOutput(content)
	if !ok {
		return fmt.Errorf("could not locate Next.js config object in %s", filepath.Base(configPath))
	}

	if err := os.WriteFile(configPath, []byte(patched), 0644); err != nil {
		return fmt.Errorf("write patched next.config: %w", err)
	}

	return nil
}

func injectStandaloneOutput(content string) (string, bool) {
	for _, pattern := range nextConfigObjectPatterns {
		loc := pattern.FindStringIndex(content)
		if loc == nil {
			continue
		}

		match := content[loc[0]:loc[1]]
		braceIdx := strings.LastIndex(match, "{")
		if braceIdx == -1 {
			continue
		}

		insertAt := loc[0] + braceIdx + 1
		return content[:insertAt] + "\n  output: 'standalone'," + content[insertAt:], true
	}

	return content, false
}
