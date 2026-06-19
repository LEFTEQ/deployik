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
