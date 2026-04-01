package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const apiBase = "https://api.github.com"

// Repo represents a GitHub repository.
type Repo struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Owner         Owner  `json:"owner"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	Language      string `json:"language"`
	UpdatedAt     string `json:"updated_at"`
}

type Owner struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

// Branch represents a GitHub branch.
type Branch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// Commit represents a GitHub commit.
type Commit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

// Client is a GitHub API client authenticated with a user's token.
type Client struct {
	token string
}

// NewClient creates a new GitHub API client.
func NewClient(token string) *Client {
	return &Client{token: token}
}

// ListRepos returns repositories accessible to the authenticated user.
func (c *Client) ListRepos(page, perPage int) ([]Repo, error) {
	if perPage <= 0 {
		perPage = 30
	}
	if page <= 0 {
		page = 1
	}

	url := fmt.Sprintf("%s/user/repos?sort=updated&per_page=%d&page=%d&type=all", apiBase, perPage, page)
	var repos []Repo
	if err := c.get(url, &repos); err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}
	return repos, nil
}

// ListBranches returns branches for a repository.
func (c *Client) ListBranches(owner, repo string) ([]Branch, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100", apiBase, owner, repo)
	var branches []Branch
	if err := c.get(url, &branches); err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	return branches, nil
}

// GetLatestCommit returns the latest commit on a branch.
func (c *Client) GetLatestCommit(owner, repo, branch string) (*Commit, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", apiBase, owner, repo, branch)
	var commit Commit
	if err := c.get(url, &commit); err != nil {
		return nil, fmt.Errorf("get latest commit: %w", err)
	}
	return &commit, nil
}

func (c *Client) get(url string, target interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
