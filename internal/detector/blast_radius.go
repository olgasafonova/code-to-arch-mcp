package detector

import (
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// BlastRadiusEntry describes one node that transitively depends on a target.
type BlastRadiusEntry struct {
	NodeID   string   `json:"node_id"`
	NodeName string   `json:"node_name"`
	Depth    int      `json:"depth"`
	PathBack []string `json:"path_back"` // node IDs from this node back to the target
}

// BlastRadiusResult is the full output of ComputeBlastRadius.
type BlastRadiusResult struct {
	TargetID    string             `json:"target_id"`
	Direct      int                `json:"direct"` // count of depth-1 dependents
	Total       int                `json:"total"`
	MaxDepthHit bool               `json:"max_depth_hit"`
	Dependents  []BlastRadiusEntry `json:"dependents"`
}

// ResolveTargetToID maps a free-form target string to a node ID present in
// the graph. Tries an exact ID match first, then a path-suffix match against
// node IDs and node Path fields. Returns the resolved ID and true on success.
func ResolveTargetToID(g *model.ArchGraph, target string) (string, bool) {
	if target == "" {
		return "", false
	}

	if n := g.GetNode(target); n != nil {
		return n.ID, true
	}

	// Path-suffix match. Prefer the shortest matching ID so a more specific
	// target wins when several nodes share a common suffix.
	var best string
	for _, n := range g.Nodes() {
		if !strings.HasSuffix(n.ID, target) && !strings.HasSuffix(n.Path, target) {
			continue
		}
		if best == "" || len(n.ID) < len(best) {
			best = n.ID
		}
	}
	if best != "" {
		return best, true
	}
	return "", false
}

// ComputeBlastRadius walks resolved edges in reverse from target, returning
// every node that transitively depends on it together with the shortest path
// back to the target. BFS so dependents are discovered in depth order.
//
// maxDepth caps the walk; pass 0 or negative to use the default of 50.
// Cycles are not double-traversed because each node is visited once.
func ComputeBlastRadius(g *model.ArchGraph, targetID string, maxDepth int) BlastRadiusResult {
	if maxDepth <= 0 {
		maxDepth = 50
	}

	// Build reverse adjacency from resolved edges.
	reverse := make(map[string][]string)
	for _, e := range g.ResolvedEdges() {
		reverse[e.Target] = append(reverse[e.Target], e.Source)
	}

	type qItem struct {
		id    string
		depth int
	}

	visited := map[string]bool{targetID: true}
	parent := map[string]string{}
	queue := []qItem{{targetID, 0}}
	var dependents []BlastRadiusEntry
	direct := 0
	maxHit := false

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= maxDepth {
			if len(reverse[cur.id]) > 0 {
				maxHit = true
			}
			continue
		}
		for _, src := range reverse[cur.id] {
			if visited[src] {
				continue
			}
			visited[src] = true
			parent[src] = cur.id
			depth := cur.depth + 1
			if depth == 1 {
				direct++
			}

			name := src
			if n := g.GetNode(src); n != nil && n.Name != "" {
				name = n.Name
			}

			dependents = append(dependents, BlastRadiusEntry{
				NodeID:   src,
				NodeName: name,
				Depth:    depth,
				PathBack: tracePathBack(src, parent, targetID),
			})

			queue = append(queue, qItem{src, depth})
		}
	}

	return BlastRadiusResult{
		TargetID:    targetID,
		Direct:      direct,
		Total:       len(dependents),
		MaxDepthHit: maxHit,
		Dependents:  dependents,
	}
}

// tracePathBack walks the parent map from start to target, returning the
// chain of node IDs (start ... target). Returns just [start] if no chain
// is recorded; caller should not happen because BFS records every visited
// non-target node's parent before enqueueing it.
func tracePathBack(start string, parent map[string]string, target string) []string {
	chain := []string{start}
	cur := start
	for cur != target {
		next, ok := parent[cur]
		if !ok {
			break
		}
		chain = append(chain, next)
		cur = next
	}
	return chain
}
