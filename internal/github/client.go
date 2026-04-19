package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// apiBase is a var (not const) so tests can point it at a local httptest.Server
// without needing to change every method signature.
var apiBase = "https://api.github.com"

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

type createRefRequest struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
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

// CreateTagReference creates a lightweight git tag ref for the provided commit SHA.
func (c *Client) CreateTagReference(owner, repo, tagName, sha string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/git/refs", apiBase, owner, repo)
	body, err := json.Marshal(createRefRequest{
		Ref: "refs/tags/" + tagName,
		SHA: sha,
	})
	if err != nil {
		return fmt.Errorf("marshal create ref: %w", err)
	}

	if err := c.post(url, bytes.NewReader(body), http.StatusCreated, nil); err != nil {
		return fmt.Errorf("create tag reference: %w", err)
	}
	return nil
}

// Webhook represents a GitHub repository webhook.
type Webhook struct {
	ID     int64 `json:"id"`
	Active bool  `json:"active"`
}

type createWebhookRequest struct {
	Name   string                 `json:"name"`
	Active bool                   `json:"active"`
	Events []string               `json:"events"`
	Config map[string]interface{} `json:"config"`
}

// CreateWebhook creates a push webhook on the given repo and returns the webhook ID.
func (c *Client) CreateWebhook(owner, repo, webhookURL, secret string) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/hooks", apiBase, owner, repo)
	body, err := json.Marshal(createWebhookRequest{
		Name:   "web",
		Active: true,
		Events: []string{"push"},
		Config: map[string]interface{}{
			"url":          webhookURL,
			"content_type": "application/json",
			"secret":       secret,
			"insecure_ssl": "0",
		},
	})
	if err != nil {
		return 0, fmt.Errorf("marshal create webhook: %w", err)
	}

	var webhook Webhook
	if err := c.post(url, bytes.NewReader(body), http.StatusCreated, &webhook); err != nil {
		return 0, fmt.Errorf("create webhook: %w", err)
	}
	return webhook.ID, nil
}

// DeleteWebhook deletes a webhook from the given repo.
func (c *Client) DeleteWebhook(owner, repo string, webhookID int64) error {
	url := fmt.Sprintf("%s/repos/%s/%s/hooks/%d", apiBase, owner, repo, webhookID)
	if err := c.delete(url); err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	return nil
}

// UpdateWebhookActive enables or disables a webhook.
func (c *Client) UpdateWebhookActive(owner, repo string, webhookID int64, active bool) error {
	url := fmt.Sprintf("%s/repos/%s/%s/hooks/%d", apiBase, owner, repo, webhookID)
	body, err := json.Marshal(map[string]bool{"active": active})
	if err != nil {
		return fmt.Errorf("marshal update webhook: %w", err)
	}
	if err := c.patch(url, bytes.NewReader(body), nil); err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	return nil
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

func (c *Client) post(url string, body io.Reader, expectedStatus int, target interface{}) error {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(payload))
	}

	if target == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *Client) delete(url string) error {
	req, err := http.NewRequest("DELETE", url, nil)
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

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) patch(url string, body io.Reader, target interface{}) error {
	req, err := http.NewRequest("PATCH", url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github API returned %d: %s", resp.StatusCode, string(payload))
	}

	if target == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
