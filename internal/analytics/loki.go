package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type LokiClient struct {
	baseURL    string
	httpClient *http.Client
}

type lokiResponse struct {
	Data struct {
		ResultType string          `json:"resultType"`
		Result     []lokiResultRow `json:"result"`
	} `json:"data"`
}

type lokiResultRow struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
	Values [][]any           `json:"values"`
}

func NewLokiClient(baseURL string) *LokiClient {
	return &LokiClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *LokiClient) Available() bool {
	return c != nil && c.baseURL != ""
}

func (c *LokiClient) Query(ctx context.Context, query string) (lokiResponse, error) {
	values := url.Values{}
	values.Set("query", query)
	return c.request(ctx, "/loki/api/v1/query", values)
}

func (c *LokiClient) QueryRange(ctx context.Context, query string, start, end time.Time, step string) (lokiResponse, error) {
	values := url.Values{}
	values.Set("query", query)
	values.Set("start", fmt.Sprintf("%d000000", start.UnixMilli()))
	values.Set("end", fmt.Sprintf("%d000000", end.UnixMilli()))
	values.Set("step", step)
	return c.request(ctx, "/loki/api/v1/query_range", values)
}

func (c *LokiClient) request(ctx context.Context, path string, params url.Values) (lokiResponse, error) {
	var payload lokiResponse
	if !c.Available() {
		return payload, fmt.Errorf("loki is not configured")
	}

	endpoint := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return payload, fmt.Errorf("create loki request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return payload, fmt.Errorf("do loki request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		content, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return payload, fmt.Errorf("loki request failed with %d: %s", resp.StatusCode, strings.TrimSpace(string(content)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return payload, fmt.Errorf("decode loki response: %w", err)
	}
	return payload, nil
}
