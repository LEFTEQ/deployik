package analytics

import "time"

type EnvironmentFilter string

const (
	EnvironmentAll        EnvironmentFilter = "all"
	EnvironmentPreview    EnvironmentFilter = "preview"
	EnvironmentProduction EnvironmentFilter = "production"
)

type RangePreset string

const (
	Range1Hour  RangePreset = "1h"
	Range24Hour RangePreset = "24h"
	Range7Days  RangePreset = "7d"
	Range30Days RangePreset = "30d"
)

type QueryOptions struct {
	Environment EnvironmentFilter
	Range       RangePreset
	Timezone    string
}

type DomainGroups struct {
	All        []string `json:"all"`
	Preview    []string `json:"preview"`
	Production []string `json:"production"`
}

type InstallPayload struct {
	HostURL   string       `json:"host_url"`
	ScriptURL string       `json:"script_url"`
	Snippet   string       `json:"snippet"`
	AIPrompt  string       `json:"ai_prompt"`
	Domains   DomainGroups `json:"domains"`
}

type AudienceSummary struct {
	Visitors             int64   `json:"visitors"`
	Pageviews            int64   `json:"pageviews"`
	Visits               int64   `json:"visits"`
	Bounces              int64   `json:"bounces"`
	BounceRate           float64 `json:"bounce_rate"`
	AvgVisitDurationMS   float64 `json:"avg_visit_duration_ms"`
	TotalVisitDurationMS int64   `json:"total_visit_duration_ms"`
}

type RealtimeSummary struct {
	Views    int64 `json:"views"`
	Visitors int64 `json:"visitors"`
	Events   int64 `json:"events"`
}

type TimePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type AudienceSeries struct {
	Pageviews []TimePoint `json:"pageviews"`
	Visits    []TimePoint `json:"visits"`
}

type RuntimeSeries struct {
	Requests    []TimePoint `json:"requests"`
	APIRequests []TimePoint `json:"api_requests"`
	Bandwidth   []TimePoint `json:"bandwidth"`
	P95Latency  []TimePoint `json:"p95_latency_ms"`
}

type BreakdownItem struct {
	Name                 string  `json:"name"`
	Value                float64 `json:"value"`
	Pageviews            int64   `json:"pageviews,omitempty"`
	Visitors             int64   `json:"visitors,omitempty"`
	Visits               int64   `json:"visits,omitempty"`
	Bounces              int64   `json:"bounces,omitempty"`
	TotalVisitDurationMS int64   `json:"total_visit_duration_ms,omitempty"`
}

type RuntimeSummary struct {
	Requests       float64 `json:"requests"`
	APIRequests    float64 `json:"api_requests"`
	BandwidthBytes float64 `json:"bandwidth_bytes"`
	ErrorRate      float64 `json:"error_rate"`
	P95LatencyMS   float64 `json:"p95_latency_ms"`
}

type AudiencePayload struct {
	Available    bool             `json:"available"`
	Enabled      bool             `json:"enabled"`
	TrackingMode string           `json:"tracking_mode"`
	Status       string           `json:"status"`
	WebsiteID    string           `json:"website_id"`
	WebsiteName  string           `json:"website_name"`
	OpenURL      string           `json:"open_url"`
	VerifiedAt   *time.Time       `json:"verified_at,omitempty"`
	LastEventAt  *time.Time       `json:"last_event_at,omitempty"`
	Error        string           `json:"error,omitempty"`
	Install      InstallPayload   `json:"install"`
	Summary      AudienceSummary  `json:"summary"`
	Realtime     *RealtimeSummary `json:"realtime,omitempty"`
	Series       AudienceSeries   `json:"series"`
	TopPages     []BreakdownItem  `json:"top_pages"`
	TopReferrers []BreakdownItem  `json:"top_referrers"`
	TopCountries []BreakdownItem  `json:"top_countries"`
}

type RuntimePayload struct {
	Available   bool            `json:"available"`
	Error       string          `json:"error,omitempty"`
	Summary     RuntimeSummary  `json:"summary"`
	Series      RuntimeSeries   `json:"series"`
	TopPaths    []BreakdownItem `json:"top_paths"`
	StatusCodes []BreakdownItem `json:"status_codes"`
}

type ProjectPayload struct {
	Environment string          `json:"environment"`
	Range       string          `json:"range"`
	Timezone    string          `json:"timezone"`
	Domains     DomainGroups    `json:"domains"`
	Audience    AudiencePayload `json:"audience"`
	Runtime     RuntimePayload  `json:"runtime"`
}
