package handlers

import (
	"testing"
	"time"

	"github.com/LEFTEQ/lovinka-deployik/internal/build"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestAppHealthCache(t *testing.T) {
	key := "app-cache-test:production"
	t0 := time.Now()
	if _, ok := getCachedAppHealth(key, t0); ok {
		t.Fatalf("expected empty cache")
	}
	storeCachedAppHealth(key, appHealth{Environment: "production", CombinedStatus: "healthy"}, t0)
	got, ok := getCachedAppHealth(key, t0.Add(3*time.Second))
	if !ok || got.CombinedStatus != "healthy" {
		t.Fatalf("expected cache hit within TTL, got ok=%v %+v", ok, got)
	}
	if _, ok := getCachedAppHealth(key, t0.Add(6*time.Second)); ok {
		t.Fatalf("expected expiry after TTL")
	}
	storeCachedAppHealth(key, appHealth{}, t0)
	invalidateAppHealth("app-cache-test", "production")
	if _, ok := getCachedAppHealth(key, t0); ok {
		t.Fatalf("expected miss after invalidate")
	}
}

func TestDeriveMemberLiveStatusFromDeployment(t *testing.T) {
	cases := []struct {
		name   string
		latest *db.Deployment
		want   string
	}{
		{"none", nil, "none"},
		{"live", &db.Deployment{Status: "live"}, "healthy"},
		{"building", &db.Deployment{Status: "building"}, "deploying"},
		{"queued", &db.Deployment{Status: "queued"}, "deploying"},
		{"deploying", &db.Deployment{Status: "deploying"}, "deploying"},
		{"failed", &db.Deployment{Status: "failed"}, "failed"},
		{"rolled_back", &db.Deployment{Status: "rolled_back"}, "degraded"},
		{"replaced", &db.Deployment{Status: "replaced"}, "degraded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveMemberLiveStatusFromDeployment(c.latest); got != c.want {
				t.Fatalf("status = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCombinedAppStatus(t *testing.T) {
	cases := []struct {
		name    string
		members []string
		want    string
	}{
		{"empty", nil, "none"},
		{"all healthy", []string{"healthy", "healthy"}, "healthy"},
		{"one deploying", []string{"healthy", "deploying"}, "deploying"},
		{"degraded beats deploying", []string{"deploying", "degraded"}, "degraded"},
		{"failed maps to degraded", []string{"healthy", "failed"}, "degraded"},
		{"down is worst", []string{"degraded", "down", "deploying"}, "down"},
		{"none only", []string{"none", "none"}, "none"},
		{"unknown maps to degraded", []string{"healthy", "unknown"}, "degraded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := combinedAppStatus(c.members); got != c.want {
				t.Fatalf("combined = %q, want %q", got, c.want)
			}
		})
	}
}

func TestStatusFromProbe(t *testing.T) {
	live := &db.Deployment{Status: "live"}
	building := &db.Deployment{Status: "building"}
	failed := &db.Deployment{Status: "failed"}
	cases := []struct {
		name   string
		latest *db.Deployment
		probe  build.MemberProbe
		want   string
	}{
		{"mid-deploy beats probe", building, build.MemberProbe{Probed: true, Running: false}, "deploying"},
		{"unprobed -> unknown", live, build.MemberProbe{Probed: false}, "unknown"},
		{"running+ok -> healthy", live, build.MemberProbe{Probed: true, Running: true, OK: true}, "healthy"},
		{"running+notok -> degraded", live, build.MemberProbe{Probed: true, Running: true, OK: false}, "degraded"},
		{"down: was live, not running", live, build.MemberProbe{Probed: true, Running: false}, "down"},
		{"failed: not running, last failed", failed, build.MemberProbe{Probed: true, Running: false}, "failed"},
		{"none: no deployment, not running", nil, build.MemberProbe{Probed: true, Running: false}, "none"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusFromProbe(c.latest, c.probe); got != c.want {
				t.Fatalf("status = %q, want %q", got, c.want)
			}
		})
	}
}
