package detector

import (
	"fmt"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Violation represents a single architecture violation.
type Violation struct {
	Rule     string             `json:"rule"`
	Severity model.DiffSeverity `json:"severity"`
	Subject  string             `json:"subject"`
	Detail   string             `json:"detail"`
}

// ValidateGraph checks the graph against built-in and custom architecture rules.
func ValidateGraph(graph *model.ArchGraph, customRules *RulesConfig) []Violation {
	var violations []Violation
	violations = append(violations, checkCycles(graph)...)
	violations = append(violations, checkOrphans(graph)...)
	violations = append(violations, checkLayeringViolations(graph)...)
	if customRules != nil {
		violations = append(violations, CheckCustomRules(graph, customRules, graph.RootPath)...)
	}
	return violations
}

// checkCycles detects circular dependencies.
func checkCycles(graph *model.ArchGraph) []Violation {
	if !graph.HasCycle() {
		return nil
	}

	// Find nodes participating in cycles using Tarjan's SCC
	sccs := tarjanSCC(graph)
	var violations []Violation
	for _, scc := range sccs {
		if len(scc) > 1 {
			violations = append(violations, Violation{
				Rule:     "no_circular_dependencies",
				Severity: model.SeverityCritical,
				Subject:  fmt.Sprintf("cycle(%d nodes)", len(scc)),
				Detail:   fmt.Sprintf("Circular dependency among: %v", scc),
			})
		}
	}

	if len(violations) == 0 && graph.HasCycle() {
		violations = append(violations, Violation{
			Rule:     "no_circular_dependencies",
			Severity: model.SeverityCritical,
			Subject:  "cycle",
			Detail:   "Circular dependency detected",
		})
	}

	return violations
}

// tarjanSCC finds strongly connected components.
func tarjanSCC(graph *model.ArchGraph) [][]string {
	adj := make(map[string][]string)
	for _, e := range graph.Edges() {
		if e.Type == model.EdgeDependency {
			adj[e.Source] = append(adj[e.Source], e.Target)
		}
	}

	index := 0
	nodeIndex := make(map[string]int)
	nodeLowlink := make(map[string]int)
	onStack := make(map[string]bool)
	var stack []string
	var sccs [][]string

	var strongConnect func(v string)
	strongConnect = func(v string) {
		nodeIndex[v] = index
		nodeLowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := nodeIndex[w]; !visited {
				strongConnect(w)
				if nodeLowlink[w] < nodeLowlink[v] {
					nodeLowlink[v] = nodeLowlink[w]
				}
			} else if onStack[w] {
				if nodeIndex[w] < nodeLowlink[v] {
					nodeLowlink[v] = nodeIndex[w]
				}
			}
		}

		if nodeLowlink[v] == nodeIndex[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, scc)
		}
	}

	for _, n := range graph.Nodes() {
		if _, visited := nodeIndex[n.ID]; !visited {
			strongConnect(n.ID)
		}
	}

	return sccs
}

// checkOrphans finds nodes with zero edges (excluding endpoints, which are leaf nodes).
func checkOrphans(graph *model.ArchGraph) []Violation {
	edgeNodes := make(map[string]bool)
	for _, e := range graph.Edges() {
		edgeNodes[e.Source] = true
		edgeNodes[e.Target] = true
	}

	var violations []Violation
	for _, n := range graph.Nodes() {
		// Endpoints are expected to be leaves
		if n.Type == model.NodeEndpoint {
			continue
		}
		// Infrastructure nodes may only be targets
		if n.Type == model.NodeDatabase || n.Type == model.NodeQueue || n.Type == model.NodeCache || n.Type == model.NodeExternalAPI {
			continue
		}
		if !edgeNodes[n.ID] {
			violations = append(violations, Violation{
				Rule:     "no_orphan_nodes",
				Severity: model.SeverityLow,
				Subject:  n.ID,
				Detail:   fmt.Sprintf("Node %q (%s) has no connections", n.Name, n.Type),
			})
		}
	}

	return violations
}

// checkLayeringViolations detects direct edges from endpoints to databases.
func checkLayeringViolations(graph *model.ArchGraph) []Violation {
	var violations []Violation

	for _, e := range graph.Edges() {
		sourceNode := graph.GetNode(e.Source)
		targetNode := graph.GetNode(e.Target)
		if sourceNode == nil || targetNode == nil {
			continue
		}

		if sourceNode.Type == model.NodeEndpoint && targetNode.Type == model.NodeDatabase {
			violations = append(violations, Violation{
				Rule:     "no_endpoint_to_database",
				Severity: model.SeverityMedium,
				Subject:  fmt.Sprintf("%s -> %s", sourceNode.Name, targetNode.Name),
				Detail:   fmt.Sprintf("Endpoint %q directly accesses database %q; consider a service layer", sourceNode.Name, targetNode.Name),
			})
		}
	}

	return violations
}
