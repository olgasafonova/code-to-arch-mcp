package render

import (
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
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
func FilterGraph(graph *model.ArchGraph, level ViewLevel) *VisibleGraph {
	nodes := FilterNodesByViewLevel(graph.Nodes(), level)
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		ids[n.ID] = true
	}
	var edges []*model.Edge
	for _, e := range graph.Edges() {
		if ids[e.Source] && ids[e.Target] {
			edges = append(edges, e)
		}
	}
	return &VisibleGraph{Nodes: nodes, Edges: edges, IDs: ids}
}

// SanitizeID replaces characters that are invalid in diagram node IDs.
func SanitizeID(id string) string {
	r := strings.NewReplacer(
		"/", "_",
		":", "_",
		".", "_",
		" ", "_",
		"-", "_",
	)
	return r.Replace(id)
}
