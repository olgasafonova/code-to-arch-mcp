package render

import (
	"sort"
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
			// Services, databases, queues, caches, external APIs, notes
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
	Nodes       []*model.Node
	Edges       []*model.Edge
	IDs         map[string]bool
	Names       map[string]string // node ID → display name
	PrunedNodes []string          // names of nodes removed by super-node pruning
}

// FilterGraph returns nodes and edges visible at the given view level.
// Edges are included only if both source and target are visible.
// Import edges (target "import:X") are resolved to internal package nodes
// via ArchGraph.ResolvedEdges().
func FilterGraph(graph *model.ArchGraph, level ViewLevel) *VisibleGraph {
	nodes := FilterNodesByViewLevel(graph.Nodes(), level)
	ids := make(map[string]bool, len(nodes))
	names := make(map[string]string, len(nodes))
	for _, n := range nodes {
		ids[n.ID] = true
		names[n.ID] = n.Name
	}

	var edges []*model.Edge
	for _, e := range graph.ResolvedEdges() {
		if !ids[e.Source] || !ids[e.Target] {
			continue
		}
		edges = append(edges, e)
	}

	return &VisibleGraph{Nodes: nodes, Edges: edges, IDs: ids, Names: names}
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

// BarycenterOrder reorders nodes within each topological layer to minimize
// edge crossings. It takes layerNodes (grouped by depth, highest depth first)
// and runs the barycenter heuristic: each node is placed at the average
// position of its connected neighbors in the adjacent layer.
// Two passes (top-down, then bottom-up) are enough for most DAGs.
func BarycenterOrder(layerNodes [][]*model.Node, edges []*model.Edge) {
	if len(layerNodes) < 2 {
		return
	}

	// Build bidirectional adjacency.
	neighbors := make(map[string][]string)
	for _, e := range edges {
		neighbors[e.Source] = append(neighbors[e.Source], e.Target)
		neighbors[e.Target] = append(neighbors[e.Target], e.Source)
	}

	// Position index: node ID → position within its layer.
	posOf := make(map[string]int)
	rebuildPos := func() {
		for _, layer := range layerNodes {
			for j, n := range layer {
				posOf[n.ID] = j
			}
		}
	}
	rebuildPos()

	sortLayer := func(layer []*model.Node) {
		bary := make(map[string]float64)
		for _, n := range layer {
			nbrs := neighbors[n.ID]
			if len(nbrs) == 0 {
				bary[n.ID] = float64(posOf[n.ID])
				continue
			}
			sum := 0.0
			for _, nb := range nbrs {
				sum += float64(posOf[nb])
			}
			bary[n.ID] = sum / float64(len(nbrs))
		}
		sort.SliceStable(layer, func(i, j int) bool {
			return bary[layer[i].ID] < bary[layer[j].ID]
		})
	}

	// Top-down pass: fix layer 0, reorder layers 1..n based on layer above.
	for i := 1; i < len(layerNodes); i++ {
		sortLayer(layerNodes[i])
		rebuildPos()
	}
	// Bottom-up pass: fix last layer, reorder layers n-1..0 based on layer below.
	for i := len(layerNodes) - 2; i >= 0; i-- {
		sortLayer(layerNodes[i])
		rebuildPos()
	}
}

// EdgeLabel returns a display label for an edge. It suppresses labels that
// match the edge type name (e.g., "dependency" on a dependency edge) or that
// duplicate the target node name, since both are redundant noise.
func EdgeLabel(e *model.Edge, targetName string) string {
	label := e.Label
	if label == "" || label == string(e.Type) {
		return ""
	}
	// Suppress label that just repeats the target node name.
	if label == targetName {
		return ""
	}
	return label
}

// PruneSuperNodes removes nodes whose fan-in ratio exceeds the threshold.
// Fan-in ratio = (unique sources targeting this node) / (total unique source nodes).
// Returns the names of pruned nodes.
func PruneSuperNodes(vg *VisibleGraph, threshold float64) []string {
	if threshold <= 0 || len(vg.Edges) == 0 {
		return nil
	}

	// Count unique source nodes across all edges.
	sources := make(map[string]bool)
	for _, e := range vg.Edges {
		sources[e.Source] = true
	}
	totalSources := len(sources)
	if totalSources == 0 {
		return nil
	}

	// Count unique sources per target (fan-in).
	fanIn := make(map[string]map[string]bool)
	for _, e := range vg.Edges {
		if fanIn[e.Target] == nil {
			fanIn[e.Target] = make(map[string]bool)
		}
		fanIn[e.Target][e.Source] = true
	}

	// Identify super-nodes.
	pruneSet := make(map[string]bool)
	var prunedNames []string
	for nodeID, srcSet := range fanIn {
		ratio := float64(len(srcSet)) / float64(totalSources)
		if ratio > threshold {
			pruneSet[nodeID] = true
		}
	}

	if len(pruneSet) == 0 {
		return nil
	}

	// Collect pruned names and rebuild node/edge slices.
	nameOf := make(map[string]string, len(vg.Nodes))
	for _, n := range vg.Nodes {
		nameOf[n.ID] = n.Name
	}
	for id := range pruneSet {
		prunedNames = append(prunedNames, nameOf[id])
		delete(vg.IDs, id)
	}

	filtered := vg.Nodes[:0]
	for _, n := range vg.Nodes {
		if !pruneSet[n.ID] {
			filtered = append(filtered, n)
		}
	}
	vg.Nodes = filtered

	filteredEdges := vg.Edges[:0]
	for _, e := range vg.Edges {
		if !pruneSet[e.Source] && !pruneSet[e.Target] {
			filteredEdges = append(filteredEdges, e)
		}
	}
	vg.Edges = filteredEdges
	vg.PrunedNodes = prunedNames

	return prunedNames
}

// PrepareGraph applies view-level filtering and optional super-node pruning.
func PrepareGraph(graph *model.ArchGraph, opts Options) *VisibleGraph {
	vg := FilterGraph(graph, opts.ViewLevel)
	PruneSuperNodes(vg, opts.PruneThreshold)
	return vg
}

// SanitizeID replaces characters that are invalid in diagram node IDs.
// Different separators use distinct replacements to avoid collisions
// (e.g., "api/v1" and "api.v1" produce different IDs). Any remaining
// non-alphanumeric character is replaced with a single underscore so
// IDs derived from filesystem paths (e.g. note basenames containing
// parens or punctuation) parse correctly across all diagram formats.
func SanitizeID(id string) string {
	r := strings.NewReplacer(
		"/", "__",
		":", "___",
		".", "_",
		" ", "_",
		"-", "_",
	)
	out := r.Replace(id)
	var sb strings.Builder
	sb.Grow(len(out))
	for _, ch := range out {
		switch {
		case ch >= 'a' && ch <= 'z',
			ch >= 'A' && ch <= 'Z',
			ch >= '0' && ch <= '9',
			ch == '_':
			sb.WriteRune(ch)
		default:
			sb.WriteByte('_')
		}
	}
	return sb.String()
}
