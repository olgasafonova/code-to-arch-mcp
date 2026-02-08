package detector

import (
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestValidateGraph_Clean(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite})

	violations := ValidateGraph(graph)

	if len(violations) != 0 {
		t.Fatalf("expected 0 violations for clean graph, got %d: %v", len(violations), violations)
	}
}

func TestValidateGraph_Cycle(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "pkg:a", Name: "A", Type: model.NodePackage})
	graph.AddNode(&model.Node{ID: "pkg:b", Name: "B", Type: model.NodePackage})
	graph.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:b", Type: model.EdgeDependency})
	graph.AddEdge(&model.Edge{Source: "pkg:b", Target: "pkg:a", Type: model.EdgeDependency})

	violations := ValidateGraph(graph)

	foundCycle := false
	for _, v := range violations {
		if v.Rule == "no_circular_dependencies" {
			foundCycle = true
		}
	}
	if !foundCycle {
		t.Fatal("expected circular dependency violation")
	}
}

func TestValidateGraph_OrphanNode(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "pkg:orphan", Name: "orphan", Type: model.NodePackage})
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite})

	violations := ValidateGraph(graph)

	foundOrphan := false
	for _, v := range violations {
		if v.Rule == "no_orphan_nodes" && v.Subject == "pkg:orphan" {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Fatal("expected orphan node violation for pkg:orphan")
	}
}

func TestValidateGraph_LayeringViolation(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "endpoint:api:1", Name: "GET /users", Type: model.NodeEndpoint})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "endpoint:api:1", Target: "infra:db", Type: model.EdgeReadWrite})

	violations := ValidateGraph(graph)

	foundLayering := false
	for _, v := range violations {
		if v.Rule == "no_endpoint_to_database" {
			foundLayering = true
		}
	}
	if !foundLayering {
		t.Fatal("expected layering violation for endpoint -> database")
	}
}

func TestValidateGraph_NoCycleInDAG(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "pkg:a", Name: "A", Type: model.NodePackage})
	graph.AddNode(&model.Node{ID: "pkg:b", Name: "B", Type: model.NodePackage})
	graph.AddNode(&model.Node{ID: "pkg:c", Name: "C", Type: model.NodePackage})
	graph.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:b", Type: model.EdgeDependency})
	graph.AddEdge(&model.Edge{Source: "pkg:b", Target: "pkg:c", Type: model.EdgeDependency})
	graph.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:c", Type: model.EdgeDependency})

	violations := ValidateGraph(graph)

	for _, v := range violations {
		if v.Rule == "no_circular_dependencies" {
			t.Fatal("DAG should not have cycle violations")
		}
	}
}
