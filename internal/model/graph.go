// Package model defines the core architecture graph data model.
// ArchGraph is the central type; all analyzers produce Nodes and Edges into it.
package model

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// NodeType classifies architecture components.
type NodeType string

const (
	NodeService     NodeType = "service"
	NodeModule      NodeType = "module"
	NodeDatabase    NodeType = "database"
	NodeQueue       NodeType = "queue"
	NodeCache       NodeType = "cache"
	NodeExternalAPI NodeType = "external_api"
	NodePackage     NodeType = "package"
	NodeEndpoint    NodeType = "endpoint"
)

// EdgeType classifies relationships between components.
type EdgeType string

const (
	EdgeDependency EdgeType = "dependency"
	EdgeAPICall    EdgeType = "api_call"
	EdgeDataFlow   EdgeType = "data_flow"
	EdgePublish    EdgeType = "publish"
	EdgeSubscribe  EdgeType = "subscribe"
	EdgeReadWrite  EdgeType = "read_write"
)

// Node represents an architecture component.
type Node struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       NodeType          `json:"type"`
	Language   string            `json:"language,omitempty"`
	Path       string            `json:"path,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Type       EdgeType          `json:"type"`
	Label      string            `json:"label,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// ArchGraph is the central architecture model.
// All analyzers produce Nodes and Edges into the same graph structure.
type ArchGraph struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	edges []*Edge

	// Metadata
	RootPath string            `json:"root_path"`
	Topology TopologyType      `json:"topology"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// TopologyType describes the overall project structure.
type TopologyType string

const (
	TopologyMonolith     TopologyType = "monolith"
	TopologyMonorepo     TopologyType = "monorepo"
	TopologyMicroservice TopologyType = "microservice"
	TopologyUnknown      TopologyType = "unknown"
)

// NewGraph creates an empty architecture graph.
func NewGraph(rootPath string) *ArchGraph {
	return &ArchGraph{
		nodes:    make(map[string]*Node),
		RootPath: rootPath,
		Topology: TopologyUnknown,
		Meta:     make(map[string]string),
	}
}

// AddNode adds a node to the graph. Returns false if a node with the same ID exists.
func (g *ArchGraph) AddNode(n *Node) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[n.ID]; exists {
		return false
	}
	g.nodes[n.ID] = n
	return true
}

// GetNode returns a node by ID, or nil if not found.
func (g *ArchGraph) GetNode(id string) *Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[id]
}

// Nodes returns all nodes sorted by ID for deterministic output.
func (g *ArchGraph) Nodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// NodesByType returns nodes filtered by type.
func (g *ArchGraph) NodesByType(t NodeType) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Node
	for _, n := range g.nodes {
		if n.Type == t {
			result = append(result, n)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// AddEdge adds an edge to the graph. Returns false if an identical edge
// (same source, target, and type) already exists.
func (g *ArchGraph) AddEdge(e *Edge) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, existing := range g.edges {
		if existing.Source == e.Source && existing.Target == e.Target && existing.Type == e.Type {
			return false
		}
	}
	g.edges = append(g.edges, e)
	return true
}

// Edges returns all edges.
func (g *ArchGraph) Edges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Edge, len(g.edges))
	copy(result, g.edges)
	return result
}

// EdgesFrom returns edges originating from a specific node.
func (g *ArchGraph) EdgesFrom(nodeID string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Edge
	for _, e := range g.edges {
		if e.Source == nodeID {
			result = append(result, e)
		}
	}
	return result
}

// EdgesTo returns edges targeting a specific node.
func (g *ArchGraph) EdgesTo(nodeID string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Edge
	for _, e := range g.edges {
		if e.Target == nodeID {
			result = append(result, e)
		}
	}
	return result
}

// NodeCount returns the number of nodes.
func (g *ArchGraph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// EdgeCount returns the number of edges.
func (g *ArchGraph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.edges)
}

// Merge incorporates nodes and edges from another graph.
// Existing nodes with the same ID are skipped (first write wins).
func (g *ArchGraph) Merge(other *ArchGraph) {
	other.mu.RLock()
	defer other.mu.RUnlock()

	for _, n := range other.nodes {
		g.AddNode(n)
	}

	g.mu.Lock()
	g.edges = append(g.edges, other.edges...)
	g.mu.Unlock()
}

// HasCycle checks for circular dependencies using DFS.
func (g *ArchGraph) HasCycle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	adj := make(map[string][]string)
	for _, e := range g.edges {
		if e.Type == EdgeDependency {
			adj[e.Source] = append(adj[e.Source], e.Target)
		}
	}

	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		inStack[node] = true

		for _, neighbor := range adj[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if inStack[neighbor] {
				return true
			}
		}

		inStack[node] = false
		return false
	}

	for id := range g.nodes {
		if !visited[id] {
			if dfs(id) {
				return true
			}
		}
	}
	return false
}

// RelativePaths converts absolute node paths to paths relative to the graph root.
func (g *ArchGraph) RelativePaths() {
	g.mu.Lock()
	defer g.mu.Unlock()

	prefix := g.RootPath + "/"
	for _, n := range g.nodes {
		if rel, ok := strings.CutPrefix(n.Path, prefix); ok {
			n.Path = rel
		}
	}
}

// ResolvedEdges returns edges with "import:" targets resolved to real node IDs.
// Import paths are matched to nodes via path suffix (e.g.
// "import:github.com/user/repo/internal/model" → node with path "internal/model").
// Edges whose target cannot be resolved are dropped. Duplicates are removed.
func (g *ArchGraph) ResolvedEdges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Build suffix lookup: relPath → nodeID.
	type pathRef struct {
		relPath string
		id      string
	}
	var refs []pathRef
	for _, n := range g.nodes {
		if n.Path == "" {
			continue
		}
		relPath := n.Path
		if g.RootPath != "" {
			if rel, ok := strings.CutPrefix(n.Path, g.RootPath+"/"); ok {
				relPath = rel
			}
		}
		refs = append(refs, pathRef{relPath: relPath, id: n.ID})
	}

	importCache := make(map[string]string) // import path → node ID ("" = unresolvable)

	var result []*Edge
	seen := make(map[string]bool)

	for _, e := range g.edges {
		target := e.Target

		if importPath, ok := strings.CutPrefix(target, "import:"); ok {
			if nodeID, cached := importCache[importPath]; cached {
				if nodeID == "" {
					continue
				}
				target = nodeID
			} else {
				found := false
				for _, ref := range refs {
					if strings.HasSuffix(importPath, "/"+ref.relPath) || importPath == ref.relPath {
						importCache[importPath] = ref.id
						target = ref.id
						found = true
						break
					}
				}
				if !found {
					importCache[importPath] = ""
					continue
				}
			}
		}

		if _, exists := g.nodes[target]; !exists {
			continue
		}
		if e.Source == target {
			continue
		}

		key := e.Source + "|" + target + "|" + string(e.Type)
		if seen[key] {
			continue
		}
		seen[key] = true

		result = append(result, &Edge{
			Source: e.Source,
			Target: target,
			Type:   e.Type,
			Label:  e.Label,
		})
	}
	return result
}

// Summary returns a human-readable summary of the graph.
func (g *ArchGraph) Summary() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	typeCounts := make(map[NodeType]int)
	for _, n := range g.nodes {
		typeCounts[n.Type]++
	}

	edgeTypeCounts := make(map[EdgeType]int)
	for _, e := range g.edges {
		edgeTypeCounts[e.Type]++
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Architecture Graph: %d nodes, %d edges\n", len(g.nodes), len(g.edges))
	fmt.Fprintf(&sb, "Topology: %s\n", g.Topology)
	fmt.Fprintf(&sb, "Root: %s\n", filepath.Base(g.RootPath))

	if len(typeCounts) > 0 {
		sb.WriteString("Nodes: ")
		parts := make([]string, 0, len(typeCounts))
		for t, c := range typeCounts {
			parts = append(parts, fmt.Sprintf("%d %s", c, t))
		}
		sort.Strings(parts)
		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteString("\n")
	}

	if len(edgeTypeCounts) > 0 {
		sb.WriteString("Edges: ")
		parts := make([]string, 0, len(edgeTypeCounts))
		for t, c := range edgeTypeCounts {
			parts = append(parts, fmt.Sprintf("%d %s", c, t))
		}
		sort.Strings(parts)
		sb.WriteString(strings.Join(parts, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}
