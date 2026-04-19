package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ErrNotFound is returned by content/tree fetchers when the requested path
// or ref does not exist (HTTP 404). Soft signal that callers can use to
// distinguish "absent" from "fetch failed".
var ErrNotFound = errors.New("github: not found")

// isSHA reports whether s looks like a 40-char hex SHA.
var shaRe = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func isSHA(s string) bool { return shaRe.MatchString(s) }

type contentsResponse struct {
	Type     string `json:"type"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

// GetFileContent fetches the raw bytes of a file at the given ref. Returns
// (nil, ErrNotFound) on HTTP 404. Other non-200 responses are wrapped with
// the API's response body for diagnostics. Handles GitHub's base64-encoded
// `contents` API response transparently — callers receive decoded bytes.
func (c *Client) GetFileContent(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	// URL-encode each path segment so paths with spaces or special chars are safe.
	segments := strings.Split(path, "/")
	escaped := make([]string, len(segments))
	for i, s := range segments {
		escaped[i] = url.PathEscape(s)
	}
	escapedPath := strings.Join(escaped, "/")

	reqURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s",
		apiBase, owner, repo, escapedPath, url.QueryEscape(ref))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
	}

	var cr contentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("decode contents response: %w", err)
	}

	if cr.Type != "file" {
		return nil, fmt.Errorf("github: path is not a file (type=%q)", cr.Type)
	}
	if cr.Content == "" {
		return []byte{}, nil
	}
	if cr.Encoding != "base64" {
		return nil, fmt.Errorf("github: unsupported content encoding %q", cr.Encoding)
	}

	// GitHub wraps base64 at 60 chars with newlines; strip them before decoding.
	clean := strings.ReplaceAll(cr.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("github: decode base64 content: %w", err)
	}
	return decoded, nil
}

type treeResponse struct {
	SHA       string      `json:"sha"`
	Tree      []treeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

type treeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// GetTree returns a flat list of every path in the repo at the given ref,
// recursively. Each entry is a forward-slash relative path with no leading
// slash. Both blobs and trees are included. The ref may be a branch name,
// a tag, or a commit/tree SHA — branch names and tags are resolved to their
// underlying commit SHA first. If GitHub returns truncated:true (very large
// repos), the truncated return value is true and the paths are what GitHub
// returned (some may be missing).
func (c *Client) GetTree(ctx context.Context, owner, repo, ref string) ([]string, bool, error) {
	commitSHA := ref
	if !isSHA(ref) {
		// Resolve branch/tag name to commit SHA using the existing (ctx-less) method.
		commit, err := c.GetLatestCommit(owner, repo, ref)
		if err != nil {
			// Treat a 404-style error as ErrNotFound; wrap others.
			if strings.Contains(err.Error(), "404") {
				return nil, false, ErrNotFound
			}
			return nil, false, fmt.Errorf("resolve ref %q: %w", ref, err)
		}
		commitSHA = commit.SHA
	}

	reqURL := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1",
		apiBase, owner, repo, commitSHA)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
	}

	var tr treeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, false, fmt.Errorf("decode tree response: %w", err)
	}

	paths := make([]string, 0, len(tr.Tree))
	for _, e := range tr.Tree {
		paths = append(paths, e.Path)
	}
	return paths, tr.Truncated, nil
}
