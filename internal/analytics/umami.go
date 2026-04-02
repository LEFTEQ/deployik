package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var errNotFound = errors.New("remote resource not found")

type UmamiClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client

	mu    sync.Mutex
	token string
}

type UmamiWebsite struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

type UmamiStats struct {
	Pageviews  int64 `json:"pageviews"`
	Visitors   int64 `json:"visitors"`
	Visits     int64 `json:"visits"`
	Bounces    int64 `json:"bounces"`
	TotalTime  int64 `json:"totaltime"`
	Comparison struct {
		Pageviews int64 `json:"pageviews"`
		Visitors  int64 `json:"visitors"`
		Visits    int64 `json:"visits"`
		Bounces   int64 `json:"bounces"`
		TotalTime int64 `json:"totaltime"`
	} `json:"comparison"`
}

type UmamiPoint struct {
	Timestamp string  `json:"x"`
	Value     float64 `json:"y"`
}

type UmamiPageviews struct {
	Pageviews []UmamiPoint `json:"pageviews"`
	Sessions  []UmamiPoint `json:"sessions"`
}

type UmamiMetricRow struct {
	Name      string `json:"name"`
	Pageviews int64  `json:"pageviews"`
	Visitors  int64  `json:"visitors"`
	Visits    int64  `json:"visits"`
	Bounces   int64  `json:"bounces"`
	TotalTime int64  `json:"totaltime"`
}

type UmamiDateRange struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

type UmamiRealtime struct {
	Visitors int64 `json:"visitors"`
}

type umamiLoginResponse struct {
	Token string `json:"token"`
}

func NewUmamiClient(baseURL, username, password string) *UmamiClient {
	return &UmamiClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username: strings.TrimSpace(username),
		password: password,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *UmamiClient) Available() bool {
	return c != nil && c.baseURL != "" && c.username != "" && c.password != ""
}

func (c *UmamiClient) CreateWebsite(ctx context.Context, name, domain string) (*UmamiWebsite, error) {
	var website UmamiWebsite
	if err := c.request(ctx, http.MethodPost, "/api/websites", nil, map[string]string{
		"name":   name,
		"domain": domain,
	}, &website); err != nil {
		return nil, err
	}
	return &website, nil
}

func (c *UmamiClient) UpdateWebsite(ctx context.Context, websiteID, name, domain string) (*UmamiWebsite, error) {
	var website UmamiWebsite
	if err := c.request(ctx, http.MethodPost, "/api/websites/"+url.PathEscape(websiteID), nil, map[string]string{
		"name":   name,
		"domain": domain,
	}, &website); err != nil {
		return nil, err
	}
	return &website, nil
}

func (c *UmamiClient) DeleteWebsite(ctx context.Context, websiteID string) error {
	var payload map[string]any
	return c.request(ctx, http.MethodDelete, "/api/websites/"+url.PathEscape(websiteID), nil, nil, &payload)
}

func (c *UmamiClient) GetDateRange(ctx context.Context, websiteID string) (*UmamiDateRange, error) {
	var response UmamiDateRange
	if err := c.request(ctx, http.MethodGet, "/api/websites/"+url.PathEscape(websiteID)+"/daterange", nil, nil, &response); err != nil {
		if errors.Is(err, errNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if response.StartDate == "" && response.EndDate == "" {
		return nil, nil
	}
	return &response, nil
}

func (c *UmamiClient) GetStats(ctx context.Context, websiteID string, params url.Values) (UmamiStats, error) {
	var response UmamiStats
	err := c.request(ctx, http.MethodGet, "/api/websites/"+url.PathEscape(websiteID)+"/stats", params, nil, &response)
	return response, err
}

func (c *UmamiClient) GetPageviews(ctx context.Context, websiteID string, params url.Values) (UmamiPageviews, error) {
	var response UmamiPageviews
	err := c.request(ctx, http.MethodGet, "/api/websites/"+url.PathEscape(websiteID)+"/pageviews", params, nil, &response)
	return response, err
}

func (c *UmamiClient) GetExpandedMetrics(ctx context.Context, websiteID string, params url.Values) ([]UmamiMetricRow, error) {
	var response []UmamiMetricRow
	err := c.request(ctx, http.MethodGet, "/api/websites/"+url.PathEscape(websiteID)+"/metrics/expanded", params, nil, &response)
	return response, err
}

func (c *UmamiClient) GetRealtime(ctx context.Context, websiteID string) (*UmamiRealtime, error) {
	var response UmamiRealtime
	if err := c.request(ctx, http.MethodGet, "/api/websites/"+url.PathEscape(websiteID)+"/active", nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *UmamiClient) request(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	if !c.Available() {
		return fmt.Errorf("umami is not configured")
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return err
	}

	err = c.do(ctx, method, path, query, body, out, token)
	if err == nil {
		return nil
	}

	var httpErr *httpError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		return err
	}

	c.clearToken()
	token, authErr := c.getToken(ctx)
	if authErr != nil {
		return authErr
	}
	return c.do(ctx, method, path, query, body, out, token)
}

func (c *UmamiClient) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	payload := map[string]string{
		"username": c.username,
		"password": c.password,
	}

	var response umamiLoginResponse
	if err := c.do(ctx, http.MethodPost, "/api/auth/login", nil, payload, &response, ""); err != nil {
		return "", err
	}
	if response.Token == "" {
		return "", fmt.Errorf("umami login returned an empty token")
	}

	c.mu.Lock()
	c.token = response.Token
	c.mu.Unlock()
	return response.Token, nil
}

func (c *UmamiClient) clearToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
}

func (c *UmamiClient) do(ctx context.Context, method, path string, query url.Values, body any, out any, token string) error {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode umami request: %w", err)
		}
		requestBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return fmt.Errorf("create umami request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do umami request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		content, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &httpError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(content))}
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode umami response: %w", err)
	}
	return nil
}

type httpError struct {
	StatusCode int
	Message    string
}

func (e *httpError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("http %d", e.StatusCode)
	}
	return fmt.Sprintf("http %d: %s", e.StatusCode, e.Message)
}
