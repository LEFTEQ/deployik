package handlers

import (
	"strings"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type topologyNode struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Framework string `json:"framework"`
}

type topologyEdge struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	Via       string `json:"via"`
	Kind      string `json:"kind"`
	Confirmed bool   `json:"confirmed"`
}

type appTopology struct {
	Nodes []topologyNode `json:"nodes"`
	Edges []topologyEdge `json:"edges"`
}

// deriveTopologyEdges returns one confirmed directed edge per (member -> sibling)
// pair where any of the member's own decrypted variable values references a
// sibling's host token (container name or domain). siblingTokens maps a
// project id to the strings that, if found in a var value, confirm a dependency.
// Reachable (faint) links are rendered client-side; only confirmed edges are returned.
func deriveTopologyEdges(members []db.Project, memberVars map[string][]db.ProjectVariable, siblingTokens map[string][]string) []topologyEdge {
	edges := make([]topologyEdge, 0)
	for _, m := range members {
		vars := memberVars[m.ID]
		for _, s := range members {
			if s.ID == m.ID {
				continue
			}
			tokens := siblingTokens[s.ID]
			if len(tokens) == 0 {
				continue
			}
			matched := false
			for _, v := range vars {
				for _, tok := range tokens {
					if tok != "" && strings.Contains(v.Value, tok) {
						edges = append(edges, topologyEdge{
							Source: m.ID, Target: s.ID, Via: v.Key, Kind: string(v.Kind), Confirmed: true,
						})
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}
	}
	return edges
}
