package detector

import (
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Metrics holds architecture fitness scores.
type Metrics struct {
	Components     map[string]int     `json:"components"`      // node type → count
	EdgeCounts     map[string]int     `json:"edge_counts"`     // edge type → count
	Coupling       map[string]float64 `json:"coupling"`        // nodeID → fan-out count
	Instability    map[string]float64 `json:"instability"`     // nodeID → I metric (0=stable, 1=unstable)
	MaxDepth       int                `json:"max_depth"`       // longest dependency chain
	AvgCoupling    float64            `json:"avg_coupling"`    // mean fan-out
	AvgInstability float64            `json:"avg_instability"` // mean instability
}

// ComputeMetrics calculates architecture fitness metrics from a graph.
func ComputeMetrics(graph *model.ArchGraph) *Metrics {
	m := &Metrics{
		Components:  make(map[string]int),
		EdgeCounts:  make(map[string]int),
		Coupling:    make(map[string]float64),
		Instability: make(map[string]float64),
	}

	// Component counts by type
	for _, n := range graph.Nodes() {
		m.Components[string(n.Type)]++
	}

	// Edge counts by type
	for _, e := range graph.Edges() {
		m.EdgeCounts[string(e.Type)]++
	}

	// Coupling and instability per non-infrastructure node
	var couplingSum, instabilitySum float64
	var count int

	for _, n := range graph.Nodes() {
		if isInfraNode(n.Type) {
			continue
		}

		fanOut := depEdgeCount(graph.EdgesFrom(n.ID))
		fanIn := depEdgeCount(graph.EdgesTo(n.ID))

		m.Coupling[n.ID] = float64(fanOut)

		total := fanIn + fanOut
		if total > 0 {
			m.Instability[n.ID] = float64(fanOut) / float64(total)
		}

		couplingSum += float64(fanOut)
		instabilitySum += m.Instability[n.ID]
		count++
	}

	if count > 0 {
		m.AvgCoupling = couplingSum / float64(count)
		m.AvgInstability = instabilitySum / float64(count)
	}

	// Max dependency depth via BFS
	m.MaxDepth = computeMaxDepth(graph)

	return m
}

func isInfraNode(t model.NodeType) bool {
	return t == model.NodeDatabase || t == model.NodeQueue ||
		t == model.NodeCache || t == model.NodeExternalAPI
}

func depEdgeCount(edges []*model.Edge) int {
	count := 0
	for _, e := range edges {
		if e.Type == model.EdgeDependency {
			count++
		}
	}
	return count
}

// computeMaxDepth finds the longest dependency chain using BFS from each root.
func computeMaxDepth(graph *model.ArchGraph) int {
	// Build adjacency list for dependency edges
	adj := make(map[string][]string)
	hasIncoming := make(map[string]bool)
	for _, e := range graph.Edges() {
		if e.Type == model.EdgeDependency {
			adj[e.Source] = append(adj[e.Source], e.Target)
			hasIncoming[e.Target] = true
		}
	}

	// Start BFS from nodes with no incoming dependency edges
	maxDepth := 0
	for _, n := range graph.Nodes() {
		if hasIncoming[n.ID] || isInfraNode(n.Type) {
			continue
		}
		depth := bfsDepth(n.ID, adj)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

func bfsDepth(start string, adj map[string][]string) int {
	type entry struct {
		id    string
		depth int
	}
	visited := map[string]bool{start: true}
	queue := []entry{{start, 0}}
	maxDepth := 0

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if curr.depth > maxDepth {
			maxDepth = curr.depth
		}
		for _, next := range adj[curr.id] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, entry{next, curr.depth + 1})
			}
		}
	}
	return maxDepth
}
