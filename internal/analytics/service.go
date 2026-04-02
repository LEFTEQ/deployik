package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type Service struct {
	DB             *db.DB
	Umami          *UmamiClient
	UmamiPublicURL string
	Loki           *LokiClient
}

func NewService(database *db.DB, umami *UmamiClient, umamiPublicURL string, loki *LokiClient) *Service {
	return &Service{
		DB:             database,
		Umami:          umami,
		UmamiPublicURL: strings.TrimRight(strings.TrimSpace(umamiPublicURL), "/"),
		Loki:           loki,
	}
}

func (s *Service) GetProjectPayload(ctx context.Context, project *db.Project, domains []db.Domain, opts QueryOptions) (ProjectPayload, error) {
	groups := buildDomainGroups(domains)
	record, err := s.ensureProjectAnalytics(ctx, project, groups)
	if err != nil {
		return ProjectPayload{}, err
	}

	payload := ProjectPayload{
		Environment: string(opts.Environment),
		Range:       string(opts.Range),
		Timezone:    opts.Timezone,
		Domains:     groups,
	}

	payload.Audience = s.buildAudiencePayload(record, project, groups)
	if record != nil && payload.Audience.Available && record.UmamiWebsiteID != "" {
		payload.Audience = s.populateAudience(ctx, project, groups, opts, record, payload.Audience)
	}

	payload.Runtime = s.populateRuntime(ctx, project.ID, opts)
	return payload, nil
}

func (s *Service) EnsureProject(ctx context.Context, project *db.Project, domains []db.Domain) error {
	if project == nil {
		return nil
	}
	_, err := s.ensureProjectAnalytics(ctx, project, buildDomainGroups(domains))
	return err
}

func (s *Service) DeleteProjectAnalytics(ctx context.Context, projectID string) error {
	if s.DB == nil {
		return nil
	}

	record, err := s.DB.GetProjectAnalytics(projectID)
	if err != nil {
		return err
	}
	if record != nil && record.UmamiWebsiteID != "" && s.Umami != nil && s.Umami.Available() {
		_ = s.Umami.DeleteWebsite(ctx, record.UmamiWebsiteID)
	}
	return s.DB.DeleteProjectAnalytics(projectID)
}

func (s *Service) ensureProjectAnalytics(ctx context.Context, project *db.Project, groups DomainGroups) (*db.ProjectAnalytics, error) {
	if s.DB == nil || project == nil {
		return nil, nil
	}

	record, err := s.DB.GetProjectAnalytics(project.ID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		record = &db.ProjectAnalytics{
			ProjectID:       project.ID,
			AudienceEnabled: true,
			TrackingMode:    db.AnalyticsTrackingModeAIInstall,
			AudienceStatus:  db.AnalyticsAudienceStatusReadyToInstall,
		}
	}

	if !record.AudienceEnabled || record.TrackingMode == db.AnalyticsTrackingModeDisabled {
		if err := s.DB.UpsertProjectAnalytics(record); err != nil {
			return nil, err
		}
		return record, nil
	}

	if s.Umami == nil || !s.Umami.Available() {
		record.AudienceStatus = db.AnalyticsAudienceStatusUnavailable
		record.LastError = "Audience analytics is not configured on this Deployik instance."
		if err := s.DB.UpsertProjectAnalytics(record); err != nil {
			return nil, err
		}
		return record, nil
	}

	desiredName := project.Name
	desiredDomain := preferredAnalyticsDomain(project, groups)

	if record.UmamiWebsiteID == "" {
		record.AudienceStatus = db.AnalyticsAudienceStatusProvisioning
		record.LastError = ""
		if err := s.DB.UpsertProjectAnalytics(record); err != nil {
			return nil, err
		}

		website, createErr := s.Umami.CreateWebsite(ctx, desiredName, desiredDomain)
		if createErr != nil {
			record.AudienceStatus = db.AnalyticsAudienceStatusError
			record.LastError = fmt.Sprintf("Failed to create the Umami website: %v", createErr)
			if err := s.DB.UpsertProjectAnalytics(record); err != nil {
				return nil, err
			}
			return record, nil
		}

		record.UmamiWebsiteID = website.ID
		record.UmamiWebsiteName = website.Name
		record.AudienceStatus = db.AnalyticsAudienceStatusReadyToInstall
		record.LastError = ""
	}

	if record.UmamiWebsiteID != "" {
		website, updateErr := s.Umami.UpdateWebsite(ctx, record.UmamiWebsiteID, desiredName, desiredDomain)
		switch {
		case updateErr == nil:
			record.UmamiWebsiteName = website.Name
			record.LastError = ""
		case updateErr == errNotFound:
			record.UmamiWebsiteID = ""
			record.UmamiWebsiteName = ""
			record.AudienceStatus = db.AnalyticsAudienceStatusProvisioning
			record.LastError = ""
			if err := s.DB.UpsertProjectAnalytics(record); err != nil {
				return nil, err
			}
			return s.ensureProjectAnalytics(ctx, project, groups)
		default:
			if record.LastError == "" {
				record.LastError = fmt.Sprintf("Failed to sync Umami website settings: %v", updateErr)
			}
		}
	}

	if err := s.DB.UpsertProjectAnalytics(record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *Service) buildAudiencePayload(record *db.ProjectAnalytics, project *db.Project, groups DomainGroups) AudiencePayload {
	payload := AudiencePayload{
		Enabled:      true,
		TrackingMode: string(db.AnalyticsTrackingModeAIInstall),
		Status:       string(db.AnalyticsAudienceStatusReadyToInstall),
		Install: InstallPayload{
			HostURL: s.UmamiPublicURL,
			Domains: groups,
		},
		TopPages:     []BreakdownItem{},
		TopReferrers: []BreakdownItem{},
		TopCountries: []BreakdownItem{},
	}
	if s.UmamiPublicURL != "" {
		payload.Install.ScriptURL = s.UmamiPublicURL + "/script.js"
	}

	if record == nil {
		return payload
	}

	payload.Available = record.AudienceStatus != db.AnalyticsAudienceStatusUnavailable
	payload.Enabled = record.AudienceEnabled
	payload.TrackingMode = string(record.TrackingMode)
	payload.Status = string(record.AudienceStatus)
	payload.WebsiteID = record.UmamiWebsiteID
	payload.WebsiteName = record.UmamiWebsiteName
	payload.Error = record.LastError
	if s.UmamiPublicURL != "" && record.UmamiWebsiteID != "" {
		payload.OpenURL = s.UmamiPublicURL + "/websites/" + url.PathEscape(record.UmamiWebsiteID)
	}
	if record.VerifiedAt.Valid {
		payload.VerifiedAt = &record.VerifiedAt.Time
	}
	if record.LastEventAt.Valid {
		payload.LastEventAt = &record.LastEventAt.Time
	}
	payload.Install = s.buildInstallPayload(record, project, groups)
	return payload
}

func (s *Service) buildInstallPayload(record *db.ProjectAnalytics, project *db.Project, groups DomainGroups) InstallPayload {
	payload := InstallPayload{
		HostURL: s.UmamiPublicURL,
		Domains: groups,
	}
	if s.UmamiPublicURL != "" {
		payload.ScriptURL = strings.TrimRight(s.UmamiPublicURL, "/") + "/script.js"
	}
	if record == nil || record.UmamiWebsiteID == "" || s.UmamiPublicURL == "" {
		return payload
	}

	attrs := []string{
		`defer`,
		fmt.Sprintf(`src="%s/script.js"`, s.UmamiPublicURL),
		fmt.Sprintf(`data-website-id="%s"`, record.UmamiWebsiteID),
		fmt.Sprintf(`data-host-url="%s"`, s.UmamiPublicURL),
		`data-performance="true"`,
	}
	if len(groups.All) > 0 {
		attrs = append(attrs, fmt.Sprintf(`data-domains="%s"`, strings.Join(groups.All, ",")))
	}

	payload.Snippet = "<script\n  " + strings.Join(attrs, "\n  ") + "\n></script>"
	payload.AIPrompt = buildAIPrompt(project, record.UmamiWebsiteID, s.UmamiPublicURL, groups)
	return payload
}

func buildAIPrompt(project *db.Project, websiteID, hostURL string, groups DomainGroups) string {
	allDomains := joinOrFallback(groups.All, "No Deployik domains configured yet.")
	previewDomains := joinOrFallback(groups.Preview, "No preview domains configured.")
	productionDomains := joinOrFallback(groups.Production, "No production domains configured.")
	projectName := "this project"
	framework := "unknown"
	packageManager := "auto"
	rootDirectory := "."
	if project != nil {
		if strings.TrimSpace(project.Name) != "" {
			projectName = project.Name
		}
		if strings.TrimSpace(project.Framework) != "" {
			framework = project.Framework
		}
		if strings.TrimSpace(project.PackageManager) != "" {
			packageManager = project.PackageManager
		}
		if strings.TrimSpace(project.RootDirectory) != "" {
			rootDirectory = project.RootDirectory
		}
	}

	var prompt strings.Builder
	prompt.WriteString("You are modifying an existing web application.\n\n")
	prompt.WriteString("Goal:\n")
	prompt.WriteString("Integrate self-hosted Umami analytics into this app using the existing Deployik project configuration.\n\n")
	prompt.WriteString("Analytics config:\n")
	prompt.WriteString(fmt.Sprintf("- Project name: %s\n", projectName))
	prompt.WriteString(fmt.Sprintf("- Framework preset: %s\n", framework))
	prompt.WriteString(fmt.Sprintf("- Package manager: %s\n", packageManager))
	prompt.WriteString(fmt.Sprintf("- Root directory: %s\n", rootDirectory))
	prompt.WriteString(fmt.Sprintf("- Umami host: %s\n", hostURL))
	prompt.WriteString(fmt.Sprintf("- Umami website ID: %s\n", websiteID))
	prompt.WriteString(fmt.Sprintf("- Preview domains: %s\n", previewDomains))
	prompt.WriteString(fmt.Sprintf("- Production domains: %s\n", productionDomains))
	prompt.WriteString(fmt.Sprintf("- All known domains: %s\n\n", allDomains))
	prompt.WriteString("Requirements:\n")
	prompt.WriteString("1. Find the correct shared layout, head, or app shell where analytics should be installed once for the whole app.\n")
	prompt.WriteString("2. Add the official Umami script using the exact configuration below and avoid duplicate injection.\n")
	prompt.WriteString("3. Keep the integration SSR-safe and preserve the current architecture instead of creating a parallel analytics system.\n")
	prompt.WriteString("4. Do not inject the script in local development unless the project already intentionally tracks non-production traffic.\n")
	prompt.WriteString("5. Create a small reusable helper with two functions:\n")
	prompt.WriteString("   - trackAnalyticsEvent(name, data?) -> wraps window.umami.track(name, data)\n")
	prompt.WriteString("   - identifyAnalyticsUser(idOrData, data?) -> wraps window.umami.identify(...)\n")
	prompt.WriteString("6. If the app already has a tracking or analytics utility, extend that instead of introducing a new competing abstraction.\n")
	prompt.WriteString("7. Return the changed files and a short verification checklist.\n\n")
	prompt.WriteString("Use this exact tracker snippet configuration:\n")
	prompt.WriteString("```html\n")
	prompt.WriteString(fmt.Sprintf("<script defer src=\"%s/script.js\" data-website-id=\"%s\" data-host-url=\"%s\"", hostURL, websiteID, hostURL))
	if len(groups.All) > 0 {
		prompt.WriteString(fmt.Sprintf(" data-domains=\"%s\"", strings.Join(groups.All, ",")))
	}
	prompt.WriteString(" data-performance=\"true\"></script>\n")
	prompt.WriteString("```\n\n")
	prompt.WriteString("Implementation guidance:\n")
	prompt.WriteString("- Prefer the framework's official script/head primitive if it exists.\n")
	prompt.WriteString("- Do not add manual pageview tracking unless the app disables Umami auto-tracking.\n")
	prompt.WriteString("- If CSP is present, update it only as much as needed for the Umami script host.\n")
	prompt.WriteString("- Include one or two concrete verification steps, such as confirming the script loads once and that pageviews appear in Umami.\n")
	return prompt.String()
}

func (s *Service) populateAudience(ctx context.Context, project *db.Project, groups DomainGroups, opts QueryOptions, record *db.ProjectAnalytics, payload AudiencePayload) AudiencePayload {
	startAt, endAt := opts.Range.UmamiRange(time.Now().UTC())
	hostnames := hostnamesForEnvironment(groups, opts.Environment)
	aggregator := newAudienceAggregator()

	if daterange, err := s.Umami.GetDateRange(ctx, record.UmamiWebsiteID); err == nil && daterange != nil {
		if parsed, parseErr := time.Parse(time.RFC3339, daterange.EndDate); parseErr == nil {
			record.LastEventAt = sql.NullTime{Time: parsed.UTC(), Valid: true}
			payload.LastEventAt = &record.LastEventAt.Time
		}
	}

	params := url.Values{}
	params.Set("startAt", strconv.FormatInt(startAt, 10))
	params.Set("endAt", strconv.FormatInt(endAt, 10))
	params.Set("timezone", opts.Timezone)
	params.Set("unit", opts.Range.UmamiUnit())

	if opts.Environment != EnvironmentAll && len(hostnames) == 0 {
		payload.Status = string(record.AudienceStatus)
		return payload
	}

	if len(hostnames) == 0 {
		if err := aggregator.fetchForHost(ctx, s.Umami, record.UmamiWebsiteID, params, ""); err != nil {
			payload.Status = string(db.AnalyticsAudienceStatusError)
			payload.Error = err.Error()
			record.AudienceStatus = db.AnalyticsAudienceStatusError
			record.LastError = err.Error()
			_ = s.DB.UpsertProjectAnalytics(record)
			return payload
		}
	} else {
		for _, hostname := range hostnames {
			if err := aggregator.fetchForHost(ctx, s.Umami, record.UmamiWebsiteID, params, hostname); err != nil {
				payload.Status = string(db.AnalyticsAudienceStatusError)
				payload.Error = err.Error()
				record.AudienceStatus = db.AnalyticsAudienceStatusError
				record.LastError = err.Error()
				_ = s.DB.UpsertProjectAnalytics(record)
				return payload
			}
		}
	}

	payload.Summary = aggregator.summary()
	payload.Series = aggregator.series()
	payload.TopPages = aggregator.topRows("path")
	payload.TopReferrers = aggregator.topRows("referrer")
	payload.TopCountries = aggregator.topRows("country")
	if opts.Environment == EnvironmentAll {
		if realtime, err := s.Umami.GetRealtime(ctx, record.UmamiWebsiteID); err == nil && realtime != nil {
			payload.Realtime = &RealtimeSummary{
				Visitors: realtime.Visitors,
			}
		}
	}

	switch {
	case payload.Summary.Pageviews > 0 || payload.Summary.Visitors > 0 || payload.Summary.Visits > 0:
		record.AudienceStatus = db.AnalyticsAudienceStatusReceivingData
		if !record.VerifiedAt.Valid {
			record.VerifiedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
		}
	case record.LastEventAt.Valid && time.Since(record.LastEventAt.Time) > opts.Range.ComparisonThreshold():
		record.AudienceStatus = db.AnalyticsAudienceStatusStale
	case record.LastEventAt.Valid:
		record.AudienceStatus = db.AnalyticsAudienceStatusWaitingForData
	default:
		record.AudienceStatus = db.AnalyticsAudienceStatusReadyToInstall
	}

	record.LastError = ""
	_ = s.DB.UpsertProjectAnalytics(record)
	payload.Status = string(record.AudienceStatus)
	payload.Error = record.LastError
	if record.VerifiedAt.Valid {
		payload.VerifiedAt = &record.VerifiedAt.Time
	}
	if record.LastEventAt.Valid {
		payload.LastEventAt = &record.LastEventAt.Time
	}
	return payload
}

func (s *Service) populateRuntime(ctx context.Context, projectID string, opts QueryOptions) RuntimePayload {
	payload := RuntimePayload{
		TopPaths:    []BreakdownItem{},
		StatusCodes: []BreakdownItem{},
	}
	if s.Loki == nil || !s.Loki.Available() {
		payload.Error = "Runtime analytics is not configured on this Deployik instance."
		return payload
	}
	payload.Available = true

	window := opts.Range.WindowString()
	step := opts.Range.LokiStep()
	selector := buildLokiSelector(projectID, opts.Environment)

	requests, err := s.queryScalar(ctx, fmt.Sprintf("sum(count_over_time(%s[%s]))", selector, window))
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	apiRequests, err := s.queryScalar(ctx, fmt.Sprintf(`sum(count_over_time(%s | json | request_path=~"/api/.*" [%s]))`, selector, window))
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	bandwidth, err := s.queryScalar(ctx, fmt.Sprintf("sum(sum_over_time(%s | json | unwrap body_bytes_sent [%s]))", selector, window))
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	p95Latency, err := s.queryScalar(ctx, fmt.Sprintf("max(quantile_over_time(0.95, %s | json | unwrap request_time [%s]))", selector, window))
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	errorRequests, err := s.queryScalar(ctx, fmt.Sprintf(`sum(count_over_time(%s | status=~"4..|5.." [%s]))`, selector, window))
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	statusCodes, err := s.queryBreakdown(ctx, fmt.Sprintf("sum by (status) (count_over_time(%s[%s]))", selector, window), "status")
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	topPaths, err := s.queryBreakdown(ctx, fmt.Sprintf("topk(10, sum by (request_path) (count_over_time(%s | json [%s])))", selector, window), "request_path")
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	now := time.Now().UTC()
	start := now.Add(-opts.Range.WindowDuration())
	requestSeries, err := s.querySeries(ctx, fmt.Sprintf("sum(count_over_time(%s[%s]))", selector, step), start, now, step, 1)
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	apiSeries, err := s.querySeries(ctx, fmt.Sprintf(`sum(count_over_time(%s | json | request_path=~"/api/.*" [%s]))`, selector, step), start, now, step, 1)
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	bandwidthSeries, err := s.querySeries(ctx, fmt.Sprintf("sum(sum_over_time(%s | json | unwrap body_bytes_sent [%s]))", selector, step), start, now, step, 1)
	if err != nil {
		payload.Error = err.Error()
		return payload
	}
	latencySeries, err := s.querySeries(ctx, fmt.Sprintf("max(quantile_over_time(0.95, %s | json | unwrap request_time [%s]))", selector, step), start, now, step, 1000)
	if err != nil {
		payload.Error = err.Error()
		return payload
	}

	payload.Summary = RuntimeSummary{
		Requests:       requests,
		APIRequests:    apiRequests,
		BandwidthBytes: bandwidth,
		ErrorRate:      safeRatio(errorRequests, requests),
		P95LatencyMS:   p95Latency * 1000,
	}
	payload.Series = RuntimeSeries{
		Requests:    requestSeries,
		APIRequests: apiSeries,
		Bandwidth:   bandwidthSeries,
		P95Latency:  latencySeries,
	}
	payload.TopPaths = topPaths
	payload.StatusCodes = statusCodes
	return payload
}

func (s *Service) queryScalar(ctx context.Context, query string) (float64, error) {
	response, err := s.Loki.Query(ctx, query)
	if err != nil {
		return 0, err
	}
	if len(response.Data.Result) == 0 || len(response.Data.Result[0].Value) < 2 {
		return 0, nil
	}
	return asFloat(response.Data.Result[0].Value[1]), nil
}

func (s *Service) querySeries(ctx context.Context, query string, start, end time.Time, step string, multiplier float64) ([]TimePoint, error) {
	response, err := s.Loki.QueryRange(ctx, query, start, end, step)
	if err != nil {
		return nil, err
	}
	if len(response.Data.Result) == 0 {
		return []TimePoint{}, nil
	}

	var points []TimePoint
	for _, value := range response.Data.Result[0].Values {
		if len(value) < 2 {
			continue
		}
		timestampSeconds := asFloat(value[0])
		points = append(points, TimePoint{
			Timestamp: time.Unix(int64(timestampSeconds), 0).UTC(),
			Value:     asFloat(value[1]) * multiplier,
		})
	}
	return points, nil
}

func (s *Service) queryBreakdown(ctx context.Context, query, label string) ([]BreakdownItem, error) {
	response, err := s.Loki.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	items := make([]BreakdownItem, 0, len(response.Data.Result))
	for _, row := range response.Data.Result {
		name := strings.TrimSpace(row.Metric[label])
		if name == "" {
			name = "Unknown"
		}
		items = append(items, BreakdownItem{
			Name:  name,
			Value: asFloat(row.Value[1]),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Value > items[j].Value
	})
	return items, nil
}

func buildLokiSelector(projectID string, environment EnvironmentFilter) string {
	parts := []string{
		`job="deployik-nginx"`,
		fmt.Sprintf(`project_id="%s"`, projectID),
	}
	if environment == EnvironmentPreview || environment == EnvironmentProduction {
		parts = append(parts, fmt.Sprintf(`environment="%s"`, environment))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func buildDomainGroups(domains []db.Domain) DomainGroups {
	all := make([]string, 0, len(domains))
	preview := make([]string, 0, len(domains))
	production := make([]string, 0, len(domains))
	seen := map[string]struct{}{}

	for _, domain := range domains {
		name := strings.TrimSpace(domain.DomainName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; !ok {
			all = append(all, name)
			seen[name] = struct{}{}
		}
		switch domain.Environment {
		case string(EnvironmentPreview):
			preview = append(preview, name)
		case string(EnvironmentProduction):
			production = append(production, name)
		}
	}

	sort.Strings(all)
	sort.Strings(preview)
	sort.Strings(production)
	return DomainGroups{All: all, Preview: preview, Production: production}
}

func preferredAnalyticsDomain(project *db.Project, groups DomainGroups) string {
	if len(groups.Production) > 0 {
		return groups.Production[0]
	}
	if len(groups.Preview) > 0 {
		return groups.Preview[0]
	}
	if project != nil && strings.TrimSpace(project.Name) != "" {
		return strings.TrimSpace(project.Name) + ".preview.example.com"
	}
	return "preview.example.com"
}

func hostnamesForEnvironment(groups DomainGroups, environment EnvironmentFilter) []string {
	switch environment {
	case EnvironmentPreview:
		return append([]string(nil), groups.Preview...)
	case EnvironmentProduction:
		return append([]string(nil), groups.Production...)
	default:
		return nil
	}
}

func joinOrFallback(items []string, fallback string) string {
	if len(items) == 0 {
		return fallback
	}
	return strings.Join(items, ", ")
}

func safeRatio(numerator, denominator float64) float64 {
	if denominator <= 0 {
		return 0
	}
	return numerator / denominator
}

func asFloat(value any) float64 {
	switch typed := value.(type) {
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	case float64:
		return typed
	case int64:
		return float64(typed)
	case jsonNumber:
		parsed, _ := strconv.ParseFloat(string(typed), 64)
		return parsed
	default:
		return 0
	}
}

type jsonNumber string
