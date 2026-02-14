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
// Import edges (target "import:X") are resolved to internal package nodes
// via path suffix matching, so "import:github.com/o/r/internal/model"
// maps to a node whose relative path is "internal/model".
func FilterGraph(graph *model.ArchGraph, level ViewLevel) *VisibleGraph {
	nodes := FilterNodesByViewLevel(graph.Nodes(), level)
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		ids[n.ID] = true
	}

	// Build path refs for suffix-matching import targets to node IDs.
	type pathRef struct {
		relPath string
		id      string
	}
	var pathRefs []pathRef
	rootPath := graph.RootPath
	for _, n := range nodes {
		if n.Path == "" {
			continue
		}
		relPath := n.Path
		if rootPath != "" {
			if rel, ok := strings.CutPrefix(n.Path, rootPath+"/"); ok {
				relPath = rel
			}
		}
		pathRefs = append(pathRefs, pathRef{relPath: relPath, id: n.ID})
	}

	// Cache resolved import paths to avoid repeated suffix matching.
	importCache := make(map[string]string) // import path → node ID ("" = unresolvable)

	var edges []*model.Edge
	seen := make(map[string]bool)

	for _, e := range graph.Edges() {
		source := e.Source
		target := e.Target

		// Resolve "import:" targets to internal node IDs.
		if importPath, ok := strings.CutPrefix(target, "import:"); ok {
			if nodeID, cached := importCache[importPath]; cached {
				if nodeID != "" {
					target = nodeID
				}
			} else {
				found := false
				for _, ref := range pathRefs {
					if strings.HasSuffix(importPath, "/"+ref.relPath) || importPath == ref.relPath {
						importCache[importPath] = ref.id
						target = ref.id
						found = true
						break
					}
				}
				if !found {
					importCache[importPath] = ""
				}
			}
		}

		if !ids[source] || !ids[target] {
			continue
		}
		if source == target {
			continue
		}

		// Deduplicate resolved edges (many files in a package produce identical edges).
		key := source + "|" + target + "|" + string(e.Type)
		if seen[key] {
			continue
		}
		seen[key] = true

		edges = append(edges, &model.Edge{
			Source: source,
			Target: target,
			Type:   e.Type,
			Label:  e.Label,
		})
	}

	return &VisibleGraph{Nodes: nodes, Edges: edges, IDs: ids}
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
