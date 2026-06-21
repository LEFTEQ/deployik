package handlers

import "testing"

import "github.com/LEFTEQ/lovinka-deployik/internal/db"

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
