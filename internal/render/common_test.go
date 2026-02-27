package render

import (
	"sort"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestPruneSuperNodes_Disabled(t *testing.T) {
	vg := makeTestVisibleGraph()
	pruned := PruneSuperNodes(vg, 0)
	if len(pruned) != 0 {
		t.Fatalf("expected no pruning when threshold=0, got %v", pruned)
	}
	if len(vg.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(vg.Nodes))
	}
}

func TestPruneSuperNodes_PrunesHighFanIn(t *testing.T) {
	// Graph: A->logging, B->logging, C->logging, A->B
	// logging has fan-in 3/3 = 100%, well above 0.5 threshold
	vg := makeTestVisibleGraph()
	pruned := PruneSuperNodes(vg, 0.5)

	if len(pruned) != 1 || pruned[0] != "logging" {
		t.Fatalf("expected [logging] pruned, got %v", pruned)
	}

	// Verify logging node is removed
	for _, n := range vg.Nodes {
		if n.ID == "pkg:logging" {
			t.Fatal("logging node should have been removed")
		}
	}

	// Verify edges to/from logging are removed
	for _, e := range vg.Edges {
		if e.Source == "pkg:logging" || e.Target == "pkg:logging" {
			t.Fatal("edges involving logging should have been removed")
		}
	}

	// A->B edge should survive
	if len(vg.Edges) != 1 {
		t.Fatalf("expected 1 remaining edge (A->B), got %d", len(vg.Edges))
	}
}

func TestPruneSuperNodes_ThresholdAt100(t *testing.T) {
	// Threshold 1.0 means prune only nodes with ratio > 1.0, which is impossible
	vg := makeTestVisibleGraph()
	pruned := PruneSuperNodes(vg, 1.0)
	if len(pruned) != 0 {
		t.Fatalf("threshold 1.0 should prune nothing, got %v", pruned)
	}
}

func TestPruneSuperNodes_NoEdges(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:a", Name: "A", Type: model.NodeService})
	vg := FilterGraph(g, ViewComponent)
	pruned := PruneSuperNodes(vg, 0.5)
	if len(pruned) != 0 {
		t.Fatalf("expected no pruning with no edges, got %v", pruned)
	}
}

func TestPruneSuperNodes_MultipleSuperNodes(t *testing.T) {
	// A->fmt, B->fmt, C->fmt, A->errors, B->errors, C->errors
	// Both fmt and errors have fan-in 3/3 = 100%
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "pkg:a", Name: "A", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:b", Name: "B", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:c", Name: "C", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:fmt", Name: "fmt", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:errors", Name: "errors", Type: model.NodePackage})
	g.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:fmt", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:b", Target: "pkg:fmt", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:c", Target: "pkg:fmt", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:errors", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:b", Target: "pkg:errors", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:c", Target: "pkg:errors", Type: model.EdgeDependency})

	vg := FilterGraph(g, ViewComponent)
	pruned := PruneSuperNodes(vg, 0.5)
	sort.Strings(pruned)

	if len(pruned) != 2 {
		t.Fatalf("expected 2 pruned nodes, got %v", pruned)
	}
	if pruned[0] != "errors" || pruned[1] != "fmt" {
		t.Fatalf("expected [errors, fmt], got %v", pruned)
	}
	if len(vg.Nodes) != 3 {
		t.Fatalf("expected 3 remaining nodes, got %d", len(vg.Nodes))
	}
	if len(vg.Edges) != 0 {
		t.Fatalf("expected 0 remaining edges, got %d", len(vg.Edges))
	}
}

func TestPrepareGraph_WithPruning(t *testing.T) {
	g := makeTestGraph()
	opts := Options{ViewLevel: ViewComponent, PruneThreshold: 0.5}
	vg := PrepareGraph(g, opts)

	if len(vg.PrunedNodes) != 1 || vg.PrunedNodes[0] != "logging" {
		t.Fatalf("expected [logging] in PrunedNodes, got %v", vg.PrunedNodes)
	}
}

func TestPrepareGraph_WithoutPruning(t *testing.T) {
	g := makeTestGraph()
	opts := Options{ViewLevel: ViewComponent}
	vg := PrepareGraph(g, opts)

	if len(vg.PrunedNodes) != 0 {
		t.Fatalf("expected no pruned nodes, got %v", vg.PrunedNodes)
	}
	if len(vg.Nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(vg.Nodes))
	}
}

// makeTestGraph builds A->logging, B->logging, C->logging, A->B
func makeTestGraph() *model.ArchGraph {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "pkg:a", Name: "A", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:b", Name: "B", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:c", Name: "C", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:logging", Name: "logging", Type: model.NodePackage})
	g.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:logging", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:b", Target: "pkg:logging", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:c", Target: "pkg:logging", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:b", Type: model.EdgeDependency})
	return g
}

func makeTestVisibleGraph() *VisibleGraph {
	return FilterGraph(makeTestGraph(), ViewComponent)
}
