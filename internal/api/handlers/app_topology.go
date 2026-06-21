package handlers

import (
	"log"
	"net/http"
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

// GetTopology returns the app's members as nodes plus confirmed dependency edges
// derived from each member's own env/secret values referencing a sibling's host.
func (h *AppHandler) GetTopology(w http.ResponseWriter, r *http.Request) {
	app, _, ok := h.loadManagedApp(w, r)
	if !ok {
		return
	}
	environment, valid := normalizeAppEnvironment(r.URL.Query().Get("environment"))
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment must be preview or production"})
		return
	}
	members, err := h.DB.ListProjectsByApp(app.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load members"})
		return
	}

	nodes := make([]topologyNode, 0, len(members))
	siblingTokens := make(map[string][]string, len(members))
	for _, m := range members {
		nodes = append(nodes, topologyNode{ProjectID: m.ID, Name: m.Name, Framework: m.Framework})
		tokens := []string{db.DeploymentContainerName(m.Name, environment, nil)}
		if domains, derr := h.DB.ListDomains(m.ID); derr == nil {
			for _, d := range domains {
				if d.Environment == environment && d.DomainName != "" {
					tokens = append(tokens, d.DomainName)
				}
			}
		} else {
			log.Printf("app topology: list domains for project %s: %v", m.ID, derr)
		}
		siblingTokens[m.ID] = tokens
	}

	memberVars := make(map[string][]db.ProjectVariable, len(members))
	for i := range members {
		m := members[i]
		var decrypted []db.ProjectVariable
		for _, kind := range []db.VariableKind{db.VariableKindEnv, db.VariableKindSecret} {
			vars, verr := h.DB.ListResolvedDeployVariables(&m, environment, kind)
			if verr != nil {
				log.Printf("app topology: resolve %s variables for project %s: %v", kind, m.ID, verr)
				continue
			}
			for _, v := range vars {
				plain, derr := h.Encryptor.Decrypt(v.Value)
				if derr != nil {
					log.Printf("app topology: decrypt variable %s for project %s: %v", v.Key, m.ID, derr)
					continue
				}
				decrypted = append(decrypted, db.ProjectVariable{Key: v.Key, Kind: v.Kind, Value: plain})
			}
		}
		memberVars[m.ID] = decrypted
	}

	writeJSON(w, http.StatusOK, appTopology{
		Nodes: nodes,
		Edges: deriveTopologyEdges(members, memberVars, siblingTokens),
	})
}
