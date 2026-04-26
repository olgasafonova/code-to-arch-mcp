package detector

import (
	"github.com/olgasafonova/ridge/internal/model"
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
// Import edges ("import:X" targets) are resolved to real package node IDs
// before computing fan-in/fan-out and dependency depth.
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

	// Use resolved edges so import:X targets become real pkg node IDs.
	resolved := graph.ResolvedEdges()

	// Edge counts by type (from resolved edges)
	for _, e := range resolved {
		m.EdgeCounts[string(e.Type)]++
	}

	// Build fan-in/fan-out maps from resolved edges.
	fanOutMap := make(map[string]int)
	fanInMap := make(map[string]int)
	for _, e := range resolved {
		if e.Type == model.EdgeDependency {
			fanOutMap[e.Source]++
			fanInMap[e.Target]++
		}
	}

	// Coupling and instability per non-infrastructure node
	var couplingSum, instabilitySum float64
	var count int

	for _, n := range graph.Nodes() {
		if isInfraNode(n.Type) {
			continue
		}

		fanOut := fanOutMap[n.ID]
		fanIn := fanInMap[n.ID]

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

	// Max dependency depth via BFS on resolved edges
	m.MaxDepth = computeMaxDepth(resolved, graph.Nodes())

	return m
}

func isInfraNode(t model.NodeType) bool {
	return t == model.NodeDatabase || t == model.NodeQueue ||
		t == model.NodeCache || t == model.NodeExternalAPI
}

// computeMaxDepth finds the longest dependency chain in the graph.
// Uses DFS with memoization (valid because the graph is a DAG).
func computeMaxDepth(edges []*model.Edge, nodes []*model.Node) int {
	adj := make(map[string][]string)
	for _, e := range edges {
		if e.Type == model.EdgeDependency {
			adj[e.Source] = append(adj[e.Source], e.Target)
		}
	}

	memo := make(map[string]int)
	visiting := make(map[string]bool) // cycle detection
	var longest func(id string) int
	longest = func(id string) int {
		if v, ok := memo[id]; ok {
			return v
		}
		if visiting[id] {
			return 0 // back-edge: break cycle
		}
		visiting[id] = true
		best := 0
		for _, next := range adj[id] {
			if d := 1 + longest(next); d > best {
				best = d
			}
		}
		visiting[id] = false
		memo[id] = best
		return best
	}

	maxDepth := 0
	for _, n := range nodes {
		if isInfraNode(n.Type) {
			continue
		}
		if d := longest(n.ID); d > maxDepth {
			maxDepth = d
		}
	}
	return maxDepth
}
