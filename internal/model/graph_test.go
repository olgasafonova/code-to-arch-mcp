package model

import "testing"

func TestNewGraph(t *testing.T) {
	g := NewGraph("/tmp/project")
	if g.RootPath != "/tmp/project" {
		t.Fatalf("expected /tmp/project, got %s", g.RootPath)
	}
	if g.Topology != TopologyUnknown {
		t.Fatalf("expected unknown topology, got %s", g.Topology)
	}
	if g.NodeCount() != 0 || g.EdgeCount() != 0 {
		t.Fatal("new graph should be empty")
	}
}

func TestAddNode(t *testing.T) {
	g := NewGraph("/tmp")
	n := &Node{ID: "svc:api", Name: "API", Type: NodeService}

	if !g.AddNode(n) {
		t.Fatal("first add should return true")
	}
	if g.AddNode(n) {
		t.Fatal("duplicate add should return false")
	}
	if g.NodeCount() != 1 {
		t.Fatalf("expected 1 node, got %d", g.NodeCount())
	}
}

func TestGetNode(t *testing.T) {
	g := NewGraph("/tmp")
	g.AddNode(&Node{ID: "svc:api", Name: "API", Type: NodeService})

	n := g.GetNode("svc:api")
	if n == nil {
		t.Fatal("expected to find node")
	}
	if n.Name != "API" {
		t.Fatalf("expected API, got %s", n.Name)
	}

	if g.GetNode("nonexistent") != nil {
		t.Fatal("expected nil for nonexistent node")
	}
}

func TestNodesByType(t *testing.T) {
	g := NewGraph("/tmp")
	g.AddNode(&Node{ID: "svc:api", Name: "API", Type: NodeService})
	g.AddNode(&Node{ID: "db:pg", Name: "PostgreSQL", Type: NodeDatabase})
	g.AddNode(&Node{ID: "svc:worker", Name: "Worker", Type: NodeService})

	services := g.NodesByType(NodeService)
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}

	databases := g.NodesByType(NodeDatabase)
	if len(databases) != 1 {
		t.Fatalf("expected 1 database, got %d", len(databases))
	}
}

func TestEdges(t *testing.T) {
	g := NewGraph("/tmp")
	g.AddNode(&Node{ID: "a", Name: "A", Type: NodeService})
	g.AddNode(&Node{ID: "b", Name: "B", Type: NodeService})
	g.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeDependency})
	g.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeAPICall})

	if g.EdgeCount() != 2 {
		t.Fatalf("expected 2 edges, got %d", g.EdgeCount())
	}

	from := g.EdgesFrom("a")
	if len(from) != 2 {
		t.Fatalf("expected 2 edges from a, got %d", len(from))
	}

	to := g.EdgesTo("b")
	if len(to) != 2 {
		t.Fatalf("expected 2 edges to b, got %d", len(to))
	}

	fromB := g.EdgesFrom("b")
	if len(fromB) != 0 {
		t.Fatalf("expected 0 edges from b, got %d", len(fromB))
	}
}

func TestAddEdge_Deduplication(t *testing.T) {
	g := NewGraph("/tmp")
	g.AddNode(&Node{ID: "a", Name: "A", Type: NodeService})
	g.AddNode(&Node{ID: "b", Name: "B", Type: NodeService})

	// First add should succeed
	if !g.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeDependency}) {
		t.Fatal("first AddEdge should return true")
	}
	// Duplicate (same source, target, type) should be rejected
	if g.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeDependency}) {
		t.Fatal("duplicate AddEdge should return false")
	}
	// Same source/target but different type is not a duplicate
	if !g.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeAPICall}) {
		t.Fatal("different edge type should return true")
	}
	if g.EdgeCount() != 2 {
		t.Fatalf("expected 2 edges, got %d", g.EdgeCount())
	}
}

func TestMerge(t *testing.T) {
	g1 := NewGraph("/tmp")
	g1.AddNode(&Node{ID: "a", Name: "A", Type: NodeService})

	g2 := NewGraph("/tmp")
	g2.AddNode(&Node{ID: "b", Name: "B", Type: NodeService})
	g2.AddNode(&Node{ID: "a", Name: "A-duplicate", Type: NodeService}) // duplicate
	g2.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeDependency})

	g1.Merge(g2)
	if g1.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes after merge, got %d", g1.NodeCount())
	}
	if g1.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge after merge, got %d", g1.EdgeCount())
	}

	// First-write-wins: original name should be kept
	n := g1.GetNode("a")
	if n.Name != "A" {
		t.Fatalf("expected original name A, got %s", n.Name)
	}
}

func TestHasCycle(t *testing.T) {
	g := NewGraph("/tmp")
	g.AddNode(&Node{ID: "a", Name: "A", Type: NodePackage})
	g.AddNode(&Node{ID: "b", Name: "B", Type: NodePackage})
	g.AddNode(&Node{ID: "c", Name: "C", Type: NodePackage})

	g.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeDependency})
	g.AddEdge(&Edge{Source: "b", Target: "c", Type: EdgeDependency})

	if g.HasCycle() {
		t.Fatal("no cycle should be detected in a->b->c")
	}

	// Add cycle: c -> a
	g.AddEdge(&Edge{Source: "c", Target: "a", Type: EdgeDependency})
	if !g.HasCycle() {
		t.Fatal("cycle should be detected in a->b->c->a")
	}
}

func TestHasCycle_NoCycleWithNonDependencyEdges(t *testing.T) {
	g := NewGraph("/tmp")
	g.AddNode(&Node{ID: "a", Name: "A", Type: NodeService})
	g.AddNode(&Node{ID: "b", Name: "B", Type: NodeDatabase})

	// Data flow edge should not trigger cycle detection
	g.AddEdge(&Edge{Source: "a", Target: "b", Type: EdgeReadWrite})
	g.AddEdge(&Edge{Source: "b", Target: "a", Type: EdgeDataFlow})

	if g.HasCycle() {
		t.Fatal("non-dependency edges should not cause cycles")
	}
}

func TestSummary(t *testing.T) {
	g := NewGraph("/tmp/myproject")
	g.AddNode(&Node{ID: "svc:api", Name: "API", Type: NodeService})
	g.AddNode(&Node{ID: "db:pg", Name: "PG", Type: NodeDatabase})
	g.AddEdge(&Edge{Source: "svc:api", Target: "db:pg", Type: EdgeReadWrite})

	s := g.Summary()
	if s == "" {
		t.Fatal("summary should not be empty")
	}
}

func TestNodesSortedDeterministically(t *testing.T) {
	g := NewGraph("/tmp")
	g.AddNode(&Node{ID: "z", Name: "Z", Type: NodeService})
	g.AddNode(&Node{ID: "a", Name: "A", Type: NodeService})
	g.AddNode(&Node{ID: "m", Name: "M", Type: NodeService})

	nodes := g.Nodes()
	if nodes[0].ID != "a" || nodes[1].ID != "m" || nodes[2].ID != "z" {
		t.Fatal("nodes should be sorted by ID")
	}
}
