package handlers

import (
	"testing"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

func TestDeriveTopologyEdges(t *testing.T) {
	members := []db.Project{{ID: "p-web", Name: "web"}, {ID: "p-api", Name: "api"}, {ID: "p-db", Name: "db"}}
	memberVars := map[string][]db.ProjectVariable{
		"p-web": {{Key: "API_URL", Kind: "env", Value: "http://deployik-api-production:4000"}},
		"p-api": {{Key: "DATABASE_URL", Kind: "secret", Value: "postgres://u:p@deployik-db-production:5432/app"}},
		"p-db":  {},
	}
	tokens := map[string][]string{
		"p-web": {"deployik-web-production"},
		"p-api": {"deployik-api-production"},
		"p-db":  {"deployik-db-production"},
	}

	edges := deriveTopologyEdges(members, memberVars, tokens)
	if len(edges) != 2 {
		t.Fatalf("expected 2 confirmed edges, got %d: %+v", len(edges), edges)
	}
	want := map[string]topologyEdge{
		"p-web->p-api": {Source: "p-web", Target: "p-api", Via: "API_URL", Kind: "env", Confirmed: true},
		"p-api->p-db":  {Source: "p-api", Target: "p-db", Via: "DATABASE_URL", Kind: "secret", Confirmed: true},
	}
	for _, e := range edges {
		w, ok := want[e.Source+"->"+e.Target]
		if !ok || e != w {
			t.Fatalf("unexpected edge %+v", e)
		}
	}
}

func TestDeriveTopologyEdgesNoFalseMesh(t *testing.T) {
	members := []db.Project{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}}
	edges := deriveTopologyEdges(members,
		map[string][]db.ProjectVariable{"a": {{Key: "X", Kind: "env", Value: "literal"}}, "b": {}},
		map[string][]string{"a": {"deployik-a-production"}, "b": {"deployik-b-production"}},
	)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %+v", edges)
	}
}
