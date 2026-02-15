package render

import (
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"strings"
)

// FilterNodesByViewLevel filters nodes based on the requested view level.
func FilterNodesByViewLevel(nodes []*model.Node, level ViewLevel) []*model.Node {
	var result []*model.Node
	for _, n := range nodes {
		switch level {
		case ViewSystem:
			// Only services and external APIs
			if n.Type == model.NodeService || n.Type == model.NodeExternalAPI {
				result = append(result, n)
			}
		case ViewContainer:
			// Services, databases, queues, caches, external APIs
			if n.Type != model.NodePackage && n.Type != model.NodeEndpoint {
				result = append(result, n)
			}
		case ViewComponent:
			// Everything
			result = append(result, n)
		}
	}
	return result
}

// VisibleGraph holds filtered nodes and edges for rendering.
type VisibleGraph struct {
	Nodes []*model.Node
	Edges []*model.Edge
	IDs   map[string]bool
}

// FilterGraph returns nodes and edges visible at the given view level.
// Edges are included only if both source and target are visible.
// Import edges (target "import:X") are resolved to internal package nodes
// via ArchGraph.ResolvedEdges().
func FilterGraph(graph *model.ArchGraph, level ViewLevel) *VisibleGraph {
	nodes := FilterNodesByViewLevel(graph.Nodes(), level)
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		ids[n.ID] = true
	}

	var edges []*model.Edge
	for _, e := range graph.ResolvedEdges() {
		if !ids[e.Source] || !ids[e.Target] {
			continue
		}
		edges = append(edges, e)
	}

	return &VisibleGraph{Nodes: nodes, Edges: edges, IDs: ids}
}

// TransitiveReduce removes edges that are implied by longer paths.
// For each edge (u→v) of type T, if v is reachable from u through
// intermediate nodes using only edges of type T, the direct edge is redundant.
func (vg *VisibleGraph) TransitiveReduce() {
	// Build adjacency grouped by edge type.
	type adjKey = string
	adj := make(map[adjKey]map[string][]string) // edgeType → source → targets
	for _, e := range vg.Edges {
		t := string(e.Type)
		if adj[t] == nil {
			adj[t] = make(map[string][]string)
		}
		adj[t][e.Source] = append(adj[t][e.Source], e.Target)
	}

	var kept []*model.Edge
	for _, e := range vg.Edges {
		if transitiveReachable(adj[string(e.Type)], e.Source, e.Target) {
			continue
		}
		kept = append(kept, e)
	}
	vg.Edges = kept
}

// transitiveReachable checks if target is reachable from source via a path
// of length >= 2 (through intermediate nodes).
func transitiveReachable(adj map[string][]string, source, target string) bool {
	visited := map[string]bool{source: true}
	queue := make([]string, 0)

	// Seed BFS with source's neighbors, excluding the direct target.
	for _, neighbor := range adj[source] {
		if neighbor == target {
			continue
		}
		if !visited[neighbor] {
			visited[neighbor] = true
			queue = append(queue, neighbor)
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr == target {
			return true
		}
		for _, next := range adj[curr] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

// SanitizeID replaces characters that are invalid in diagram node IDs.
// Different separators use distinct replacements to avoid collisions
// (e.g., "api/v1" and "api.v1" produce different IDs).
func SanitizeID(id string) string {
	r := strings.NewReplacer(
		"/", "__",
		":", "___",
		".", "_",
		" ", "_",
		"-", "_",
	)
	return r.Replace(id)
}
