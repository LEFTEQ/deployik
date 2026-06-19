package build

import (
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func batchNames(batches [][]db.Project) [][]string {
	out := make([][]string, len(batches))
	for i, b := range batches {
		names := make([]string, len(b))
		for j, p := range b {
			names[j] = p.Name
		}
		out[i] = names
	}
	return out
}

func TestOrderAppMembersUnordered(t *testing.T) {
	members := []db.Project{
		{Name: "web", DeployOrder: 5},
		{Name: "api", DeployOrder: 1},
		{Name: "db", DeployOrder: 0},
	}
	batches := OrderAppMembers(members, false)
	if len(batches) != 1 {
		t.Fatalf("unordered must be one batch, got %d", len(batches))
	}
	got := batchNames(batches)[0]
	want := []string{"api", "db", "web"} // sorted by name
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unordered batch = %v, want %v", got, want)
		}
	}
}

func TestOrderAppMembersOrdered(t *testing.T) {
	members := []db.Project{
		{Name: "web", DeployOrder: 2},
		{Name: "worker", DeployOrder: 2},
		{Name: "api", DeployOrder: 1},
		{Name: "db", DeployOrder: 0},
	}
	batches := OrderAppMembers(members, true)
	got := batchNames(batches)
	want := [][]string{{"db"}, {"api"}, {"web", "worker"}}
	if len(got) != len(want) {
		t.Fatalf("ordered batches = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("batch %d = %v, want %v", i, got[i], want[i])
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("batch %d = %v, want %v", i, got[i], want[i])
			}
		}
	}
}

func TestOrderAppMembersEmpty(t *testing.T) {
	if got := OrderAppMembers(nil, true); got != nil {
		t.Fatalf("empty members should yield nil, got %v", got)
	}
}

func TestRunRolloutAllHealthy(t *testing.T) {
	batches := [][]db.Project{
		{{ID: "p-db", Name: "db"}},
		{{ID: "p-api", Name: "api"}, {ID: "p-web", Name: "web"}},
	}
	var order []string
	res := RunRollout(batches, func(p db.Project) (string, bool) {
		order = append(order, p.ID)
		return "deploy-" + p.ID, true
	})
	if !res.Succeeded {
		t.Fatalf("expected success, got %+v", res)
	}
	if len(res.Deployed) != 3 {
		t.Fatalf("deployed = %d, want 3", len(res.Deployed))
	}
	if order[0] != "p-db" {
		t.Fatalf("db must deploy first, order = %v", order)
	}
	if res.Deployed[0].DeploymentID != "deploy-p-db" {
		t.Fatalf("recorded deployment id = %q", res.Deployed[0].DeploymentID)
	}
}

func TestRunRolloutHaltsOnFailure(t *testing.T) {
	batches := [][]db.Project{
		{{ID: "p-db", Name: "db"}},
		{{ID: "p-api", Name: "api"}},
		{{ID: "p-web", Name: "web"}},
	}
	var deployed []string
	res := RunRollout(batches, func(p db.Project) (string, bool) {
		deployed = append(deployed, p.ID)
		return "deploy-" + p.ID, p.ID != "p-api" // api fails
	})
	if res.Succeeded {
		t.Fatal("expected failure")
	}
	if res.FailedProjectID != "p-api" {
		t.Fatalf("failed project = %q, want p-api", res.FailedProjectID)
	}
	// web (after the failed api) must NOT have been deployed.
	for _, id := range deployed {
		if id == "p-web" {
			t.Fatal("rollout must halt before deploying web")
		}
	}
	// db (before api) is the already-swapped set to roll back.
	if len(res.Deployed) != 1 || res.Deployed[0].ProjectID != "p-db" {
		t.Fatalf("already-deployed set = %+v, want [p-db]", res.Deployed)
	}
}
