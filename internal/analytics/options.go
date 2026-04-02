package analytics

import (
	"fmt"
	"strings"
	"time"
)

func NormalizeEnvironment(value string) EnvironmentFilter {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(EnvironmentPreview):
		return EnvironmentPreview
	case string(EnvironmentProduction):
		return EnvironmentProduction
	default:
		return EnvironmentAll
	}
}

func NormalizeRange(value string) RangePreset {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(Range1Hour):
		return Range1Hour
	case string(Range7Days):
		return Range7Days
	case string(Range30Days):
		return Range30Days
	default:
		return Range24Hour
	}
}

func NormalizeTimezone(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "UTC"
	}
	if _, err := time.LoadLocation(trimmed); err != nil {
		return "UTC"
	}
	return trimmed
}

func (r RangePreset) WindowDuration() time.Duration {
	switch r {
	case Range1Hour:
		return time.Hour
	case Range7Days:
		return 7 * 24 * time.Hour
	case Range30Days:
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (r RangePreset) WindowString() string {
	switch r {
	case Range1Hour:
		return "1h"
	case Range7Days:
		return "7d"
	case Range30Days:
		return "30d"
	default:
		return "24h"
	}
}

func (r RangePreset) LokiStep() string {
	switch r {
	case Range1Hour:
		return "1m"
	case Range7Days:
		return "2h"
	case Range30Days:
		return "1d"
	default:
		return "30m"
	}
}

func (r RangePreset) UmamiUnit() string {
	switch r {
	case Range1Hour:
		return "minute"
	case Range7Days, Range30Days:
		return "day"
	default:
		return "hour"
	}
}

func (r RangePreset) UmamiRange(now time.Time) (startAt, endAt int64) {
	endAt = now.UnixMilli()
	startAt = now.Add(-r.WindowDuration()).UnixMilli()
	return startAt, endAt
}

func (r RangePreset) ComparisonThreshold() time.Duration {
	return 7 * 24 * time.Hour
}

func (r RangePreset) Validate() error {
	switch r {
	case Range1Hour, Range24Hour, Range7Days, Range30Days:
		return nil
	default:
		return fmt.Errorf("unsupported range %q", r)
	}
}
