package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func newAnalyticsTestDB(t *testing.T) *db.DB {
	t.Helper()

	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func newAnalyticsProject(t *testing.T, database *db.DB) (*db.Project, []db.Domain) {
	t.Helper()

	user := &db.User{ID: db.NewID(), GithubID: 1, Username: "analytics", Role: "admin"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	project := &db.Project{
		Name:           "analytics-demo",
		GithubRepo:     "repo",
		GithubOwner:    "owner",
		Branch:         "main",
		UserID:         user.ID,
		Framework:      "nextjs",
		PackageManager: "pnpm",
		RootDirectory:  "apps/web",
		BuildCommand:   "pnpm run build",
		InstallCommand: "pnpm install --frozen-lockfile",
		NodeVersion:    "22",
		Status:         "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	domains := []db.Domain{
		{
			ProjectID:   project.ID,
			DomainName:  "analytics-demo.preview.example.com",
			Environment: "preview",
			IsAuto:      true,
			SSLStatus:   "active",
		},
		{
			ProjectID:   project.ID,
			DomainName:  "analytics-demo.com",
			Environment: "production",
			IsAuto:      false,
			SSLStatus:   "active",
		},
	}
	for i := range domains {
		if err := database.CreateDomain(&domains[i]); err != nil {
			t.Fatalf("CreateDomain(%d): %v", i, err)
		}
	}

	return project, domains
}

func TestBuildAIPromptIncludesProjectContext(t *testing.T) {
	project := &db.Project{
		Name:           "analytics-demo",
		Framework:      "nextjs",
		PackageManager: "pnpm",
		RootDirectory:  "apps/web",
	}

	prompt := buildAIPrompt(project, "website-123", "https://analytics.example.com", "https://cdn.example.com/deployik-analytics/umami/latest.js", DomainGroups{
		All:        []string{"analytics-demo.preview.example.com", "analytics-demo.com"},
		Preview:    []string{"analytics-demo.preview.example.com"},
		Production: []string{"analytics-demo.com"},
	})

	for _, expected := range []string{
		"Project name: analytics-demo",
		"Framework preset: nextjs",
		"Package manager: pnpm",
		"Root directory: apps/web",
		"Tracker script URL: https://cdn.example.com/deployik-analytics/umami/latest.js",
		`data-website-id="website-123"`,
		`src="https://cdn.example.com/deployik-analytics/umami/latest.js"`,
		"analytics-demo.com",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q\n%s", expected, prompt)
		}
	}
}

func TestBuildLokiSelectorUsesFilenameFilter(t *testing.T) {
	tests := []struct {
		name        string
		environment EnvironmentFilter
		expected    string
	}{
		{
			name:        "all environments",
			environment: EnvironmentAll,
			expected:    `{job="deployik-nginx",filename=~".*deployik-01KN5PPX9D28XX9J83C2XQYF2T-.*"}`,
		},
		{
			name:        "preview environment",
			environment: EnvironmentPreview,
			expected:    `{job="deployik-nginx",filename=~".*deployik-01KN5PPX9D28XX9J83C2XQYF2T-.*-preview.*"}`,
		},
		{
			name:        "production environment",
			environment: EnvironmentProduction,
			expected:    `{job="deployik-nginx",filename=~".*deployik-01KN5PPX9D28XX9J83C2XQYF2T-.*-production.*"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if selector := buildLokiSelector("01KN5PPX9D28XX9J83C2XQYF2T", tt.environment); selector != tt.expected {
				t.Fatalf("unexpected selector:\nwant %s\ngot  %s", tt.expected, selector)
			}
		})
	}
}

func TestGetProjectPayloadProvisioningAndMetrics(t *testing.T) {
	database := newAnalyticsTestDB(t)
	project, domains := newAnalyticsProject(t, database)

	umamiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/auth/login":
			json.NewEncoder(w).Encode(map[string]any{"token": "test-token"})
		case r.URL.Path == "/api/websites" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"id":     "website-1",
				"name":   "analytics-demo",
				"domain": "analytics-demo.com",
			})
		case r.URL.Path == "/api/websites/website-1" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"id":     "website-1",
				"name":   "analytics-demo",
				"domain": "analytics-demo.com",
			})
		case r.URL.Path == "/api/websites/website-1/daterange":
			json.NewEncoder(w).Encode(map[string]any{
				"startDate": "2026-04-01T00:00:00Z",
				"endDate":   "2026-04-02T12:00:00Z",
			})
		case r.URL.Path == "/api/websites/website-1/stats":
			if r.URL.Query().Get("hostname") != "analytics-demo.com" {
				t.Fatalf("expected production hostname filter, got %q", r.URL.Query().Get("hostname"))
			}
			json.NewEncoder(w).Encode(map[string]any{
				"pageviews": 120,
				"visitors":  40,
				"visits":    52,
				"bounces":   12,
				"totaltime": 520000,
				"comparison": map[string]any{
					"pageviews": 0,
					"visitors":  0,
					"visits":    0,
					"bounces":   0,
					"totaltime": 0,
				},
			})
		case r.URL.Path == "/api/websites/website-1/pageviews":
			json.NewEncoder(w).Encode(map[string]any{
				"pageviews": []map[string]any{
					{"x": "2026-04-02T10:00:00Z", "y": 40},
					{"x": "2026-04-02T11:00:00Z", "y": 80},
				},
				"sessions": []map[string]any{
					{"x": "2026-04-02T10:00:00Z", "y": 20},
					{"x": "2026-04-02T11:00:00Z", "y": 32},
				},
			})
		case r.URL.Path == "/api/websites/website-1/metrics/expanded":
			switch r.URL.Query().Get("type") {
			case "path":
				json.NewEncoder(w).Encode([]map[string]any{
					{"name": "/pricing", "pageviews": 75, "visitors": 24, "visits": 30, "bounces": 5, "totaltime": 220000},
				})
			case "referrer":
				json.NewEncoder(w).Encode([]map[string]any{
					{"name": "google.com", "pageviews": 50, "visitors": 18, "visits": 22, "bounces": 4, "totaltime": 120000},
				})
			case "country":
				json.NewEncoder(w).Encode([]map[string]any{
					{"name": "Czechia", "pageviews": 35, "visitors": 10, "visits": 12, "bounces": 2, "totaltime": 90000},
				})
			default:
				t.Fatalf("unexpected metrics type %q", r.URL.Query().Get("type"))
			}
		case r.URL.Path == "/api/websites/website-1/active":
			json.NewEncoder(w).Encode(map[string]any{"visitors": 6})
		default:
			t.Fatalf("unexpected Umami request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer umamiServer.Close()

	lokiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		query := r.URL.Query().Get("query")

		switch r.URL.Path {
		case "/loki/api/v1/query":
			switch {
			case strings.Contains(query, `sum by (status)`):
				json.NewEncoder(w).Encode(mockLokiVector([]mockVectorRow{
					{Metric: map[string]string{"status": "200"}, Value: "180"},
					{Metric: map[string]string{"status": "500"}, Value: "20"},
				}))
			case strings.Contains(query, `topk(10, sum by (request_path)`):
				json.NewEncoder(w).Encode(mockLokiVector([]mockVectorRow{
					{Metric: map[string]string{"request_path": "/api/contact"}, Value: "40"},
					{Metric: map[string]string{"request_path": "/"}, Value: "100"},
				}))
			case strings.Contains(query, `status=~"4..|5.."`):
				json.NewEncoder(w).Encode(mockLokiScalar("20"))
			case strings.Contains(query, "body_bytes_sent"):
				json.NewEncoder(w).Encode(mockLokiScalar("524288"))
			case strings.Contains(query, `request_path=~"/api/.*"`):
				json.NewEncoder(w).Encode(mockLokiScalar("40"))
			case strings.Contains(query, "quantile_over_time"):
				json.NewEncoder(w).Encode(mockLokiScalar("0.62"))
			default:
				json.NewEncoder(w).Encode(mockLokiScalar("200"))
			}
		case "/loki/api/v1/query_range":
			switch {
			case strings.Contains(query, "body_bytes_sent"):
				json.NewEncoder(w).Encode(mockLokiMatrix("bandwidth", []string{"1024", "2048"}))
			case strings.Contains(query, "quantile_over_time"):
				json.NewEncoder(w).Encode(mockLokiMatrix("latency", []string{"0.4", "0.8"}))
			case strings.Contains(query, `request_path=~"/api/.*"`):
				json.NewEncoder(w).Encode(mockLokiMatrix("api", []string{"10", "30"}))
			default:
				json.NewEncoder(w).Encode(mockLokiMatrix("requests", []string{"60", "140"}))
			}
		default:
			t.Fatalf("unexpected Loki path: %s", r.URL.Path)
		}
	}))
	defer lokiServer.Close()

	service := NewService(
		database,
		NewUmamiClient(umamiServer.URL, "admin", "password"),
		"https://analytics.example.com",
		"https://cdn.example.com/deployik-analytics/umami/latest.js",
		NewLokiClient(lokiServer.URL),
	)

	payload, err := service.GetProjectPayload(context.Background(), project, domains, QueryOptions{
		Environment: EnvironmentProduction,
		Range:       Range24Hour,
		Timezone:    "UTC",
	})
	if err != nil {
		t.Fatalf("GetProjectPayload: %v", err)
	}

	if payload.Audience.WebsiteID != "website-1" {
		t.Fatalf("website_id = %q, want %q", payload.Audience.WebsiteID, "website-1")
	}
	if payload.Audience.Status != string(db.AnalyticsAudienceStatusReceivingData) {
		t.Fatalf("audience status = %q, want %q", payload.Audience.Status, db.AnalyticsAudienceStatusReceivingData)
	}
	if payload.Audience.Summary.Pageviews != 120 {
		t.Fatalf("pageviews = %d, want 120", payload.Audience.Summary.Pageviews)
	}
	if !strings.Contains(payload.Audience.Install.Snippet, `data-website-id="website-1"`) {
		t.Fatalf("expected snippet to contain website id, got %q", payload.Audience.Install.Snippet)
	}
	if !strings.Contains(payload.Audience.Install.Snippet, `src="https://cdn.example.com/deployik-analytics/umami/latest.js"`) {
		t.Fatalf("expected snippet to contain CDN script URL, got %q", payload.Audience.Install.Snippet)
	}
	if !strings.Contains(payload.Audience.Install.AIPrompt, "Framework preset: nextjs") {
		t.Fatalf("expected framework in AI prompt, got %q", payload.Audience.Install.AIPrompt)
	}

	if !payload.Runtime.Available {
		t.Fatal("runtime should be available")
	}
	if payload.Runtime.Summary.Requests != 200 {
		t.Fatalf("runtime requests = %v, want 200", payload.Runtime.Summary.Requests)
	}
	if payload.Runtime.Summary.APIRequests != 40 {
		t.Fatalf("api requests = %v, want 40", payload.Runtime.Summary.APIRequests)
	}
	if payload.Runtime.Summary.BandwidthBytes != 524288 {
		t.Fatalf("bandwidth = %v, want 524288", payload.Runtime.Summary.BandwidthBytes)
	}
	if payload.Runtime.Summary.P95LatencyMS != 620 {
		t.Fatalf("p95 latency = %v, want 620", payload.Runtime.Summary.P95LatencyMS)
	}
	if len(payload.Runtime.TopPaths) != 2 {
		t.Fatalf("top paths len = %d, want 2", len(payload.Runtime.TopPaths))
	}

	record, err := database.GetProjectAnalytics(project.ID)
	if err != nil {
		t.Fatalf("GetProjectAnalytics: %v", err)
	}
	if record == nil || record.UmamiWebsiteID != "website-1" {
		t.Fatal("expected analytics record to be persisted")
	}
	if record.AudienceStatus != db.AnalyticsAudienceStatusReceivingData {
		t.Fatalf("stored audience status = %q, want %q", record.AudienceStatus, db.AnalyticsAudienceStatusReceivingData)
	}
}

func TestGetProjectPayloadProductionWithoutDomainsReturnsEmptyAudienceSeries(t *testing.T) {
	database := newAnalyticsTestDB(t)
	project, domains := newAnalyticsProject(t, database)
	domains = domains[:1]

	umamiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/auth/login":
			json.NewEncoder(w).Encode(map[string]any{"token": "test-token"})
		case r.URL.Path == "/api/websites" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"id":     "website-1",
				"name":   "analytics-demo",
				"domain": "analytics-demo.preview.example.com",
			})
		case r.URL.Path == "/api/websites/website-1" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"id":     "website-1",
				"name":   "analytics-demo",
				"domain": "analytics-demo.preview.example.com",
			})
		case r.URL.Path == "/api/websites/website-1/daterange":
			json.NewEncoder(w).Encode(map[string]any{
				"startDate": "2026-04-01T00:00:00Z",
				"endDate":   "2026-04-02T12:00:00Z",
			})
		default:
			t.Fatalf("unexpected Umami request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer umamiServer.Close()

	service := NewService(
		database,
		NewUmamiClient(umamiServer.URL, "admin", "password"),
		"https://analytics.example.com",
		"https://cdn.example.com/deployik-analytics/umami/latest.js",
		nil,
	)

	payload, err := service.GetProjectPayload(context.Background(), project, domains, QueryOptions{
		Environment: EnvironmentProduction,
		Range:       Range24Hour,
		Timezone:    "UTC",
	})
	if err != nil {
		t.Fatalf("GetProjectPayload: %v", err)
	}

	if payload.Audience.Series.Pageviews == nil {
		t.Fatal("pageviews series should be an empty slice, not nil")
	}
	if payload.Audience.Series.Visits == nil {
		t.Fatal("visits series should be an empty slice, not nil")
	}
	if len(payload.Audience.Series.Pageviews) != 0 {
		t.Fatalf("pageviews series len = %d, want 0", len(payload.Audience.Series.Pageviews))
	}
	if len(payload.Audience.Series.Visits) != 0 {
		t.Fatalf("visits series len = %d, want 0", len(payload.Audience.Series.Visits))
	}
}

type mockVectorRow struct {
	Metric map[string]string
	Value  string
}

func mockLokiScalar(value string) map[string]any {
	return mockLokiVector([]mockVectorRow{{Metric: map[string]string{}, Value: value}})
}

func mockLokiVector(rows []mockVectorRow) map[string]any {
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		result = append(result, map[string]any{
			"metric": row.Metric,
			"value":  []any{"1712052000", row.Value},
		})
	}
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "vector",
			"result":     result,
		},
	}
}

func mockLokiMatrix(metric string, values []string) map[string]any {
	points := make([][]any, 0, len(values))
	for index, value := range values {
		points = append(points, []any{float64(1712052000 + index*1800), value})
	}
	return map[string]any{
		"status": "success",
		"data": map[string]any{
			"resultType": "matrix",
			"result": []map[string]any{
				{
					"metric": map[string]string{"series": metric},
					"values": points,
				},
			},
		},
	}
}
