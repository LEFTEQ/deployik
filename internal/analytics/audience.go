package analytics

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"
)

type audienceAggregator struct {
	stats     AudienceSummary
	pageviews map[time.Time]float64
	visits    map[time.Time]float64
	rows      map[string]map[string]*BreakdownItem
}

func newAudienceAggregator() *audienceAggregator {
	return &audienceAggregator{
		pageviews: map[time.Time]float64{},
		visits:    map[time.Time]float64{},
		rows: map[string]map[string]*BreakdownItem{
			"path":     {},
			"referrer": {},
			"country":  {},
		},
	}
}

func (a *audienceAggregator) fetchForHost(ctx context.Context, client *UmamiClient, websiteID string, baseParams url.Values, hostname string) error {
	params := cloneValues(baseParams)
	if hostname != "" {
		params.Set("hostname", hostname)
	}

	stats, err := client.GetStats(ctx, websiteID, params)
	if err != nil {
		return fmt.Errorf("load audience stats: %w", err)
	}
	a.stats.Pageviews += stats.Pageviews
	a.stats.Visitors += stats.Visitors
	a.stats.Visits += stats.Visits
	a.stats.Bounces += stats.Bounces
	a.stats.TotalVisitDurationMS += stats.TotalTime

	pageviews, err := client.GetPageviews(ctx, websiteID, params)
	if err != nil {
		return fmt.Errorf("load audience time series: %w", err)
	}
	for _, point := range pageviews.Pageviews {
		timestamp, parseErr := time.Parse(time.RFC3339, point.Timestamp)
		if parseErr != nil {
			continue
		}
		a.pageviews[timestamp.UTC()] += point.Value
	}
	for _, point := range pageviews.Sessions {
		timestamp, parseErr := time.Parse(time.RFC3339, point.Timestamp)
		if parseErr != nil {
			continue
		}
		a.visits[timestamp.UTC()] += point.Value
	}

	for _, metricType := range []string{"path", "referrer", "country"} {
		metricParams := cloneValues(params)
		metricParams.Set("type", metricType)
		metricParams.Set("limit", "8")
		rows, metricErr := client.GetExpandedMetrics(ctx, websiteID, metricParams)
		if metricErr != nil {
			return fmt.Errorf("load %s metrics: %w", metricType, metricErr)
		}
		for _, row := range rows {
			current := a.rows[metricType][row.Name]
			if current == nil {
				current = &BreakdownItem{Name: row.Name}
				a.rows[metricType][row.Name] = current
			}
			current.Pageviews += row.Pageviews
			current.Visitors += row.Visitors
			current.Visits += row.Visits
			current.Bounces += row.Bounces
			current.TotalVisitDurationMS += row.TotalTime
			current.Value += float64(row.Pageviews)
		}
	}

	return nil
}

func (a *audienceAggregator) summary() AudienceSummary {
	summary := a.stats
	if summary.Visits > 0 {
		summary.BounceRate = float64(summary.Bounces) / float64(summary.Visits)
		summary.AvgVisitDurationMS = float64(summary.TotalVisitDurationMS) / float64(summary.Visits)
	}
	return summary
}

func (a *audienceAggregator) series() AudienceSeries {
	return AudienceSeries{
		Pageviews: sortedPoints(a.pageviews),
		Visits:    sortedPoints(a.visits),
	}
}

func (a *audienceAggregator) topRows(kind string) []BreakdownItem {
	rows := a.rows[kind]
	if len(rows) == 0 {
		return []BreakdownItem{}
	}

	items := make([]BreakdownItem, 0, len(rows))
	for _, item := range rows {
		if item == nil {
			continue
		}
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Value > items[j].Value
	})
	if len(items) > 8 {
		items = items[:8]
	}
	return items
}

func sortedPoints(values map[time.Time]float64) []TimePoint {
	points := make([]TimePoint, 0, len(values))
	for timestamp, value := range values {
		points = append(points, TimePoint{
			Timestamp: timestamp.UTC(),
			Value:     value,
		})
	}
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})
	return points
}

func cloneValues(values url.Values) url.Values {
	cloned := url.Values{}
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}
	return cloned
}
