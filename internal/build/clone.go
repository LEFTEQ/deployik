package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CloneRepo clones a GitHub repository to a temporary directory.
// Uses the OAuth token for authentication (supports private repos).
// Returns the path to the cloned directory.
func CloneRepo(buildDir, owner, repo, branch, token string) (string, error) {
	cloneDir := filepath.Join(buildDir, fmt.Sprintf("%s-%s", owner, repo))

	// Clean up any previous clone
	os.RemoveAll(cloneDir)

	// Build clone URL with token for authentication
	var cloneURL string
	if token != "" {
		cloneURL = fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
	} else {
		cloneURL = fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	}

	cmd := exec.Command("git", "clone",
		"--depth", "1",
		"--branch", branch,
		"--single-branch",
		cloneURL,
		cloneDir,
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}

	return cloneDir, nil
}

// GetHeadCommit returns the HEAD commit SHA and message from a cloned repo.
func GetHeadCommit(repoDir string) (sha, message string, err error) {
	// Get SHA
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("get HEAD sha: %w", err)
	}
	sha = string(out[:len(out)-1]) // trim newline

	// Get message
	cmd = exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = repoDir
	out, err = cmd.Output()
	if err != nil {
		return sha, "", fmt.Errorf("get HEAD message: %w", err)
	}
	message = string(out[:len(out)-1])

	return sha, message, nil
}
