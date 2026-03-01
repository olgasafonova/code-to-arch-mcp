package detector

import (
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestComputeTraces_LinearChain(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "endpoint:api:1", Name: "GET /users", Type: model.NodeEndpoint})
	g.AddNode(&model.Node{ID: "pkg:handler", Name: "handler", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "infra:database", Name: "Database", Type: model.NodeDatabase})

	g.AddEdge(&model.Edge{Source: "endpoint:api:1", Target: "pkg:handler", Type: model.EdgeDependency, Confidence: 0.9})
	g.AddEdge(&model.Edge{Source: "pkg:handler", Target: "infra:database", Type: model.EdgeReadWrite, Confidence: 0.8})

	traces := ComputeTraces(g)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	tr := traces[0]
	if tr.EntryPoint != "endpoint:api:1" {
		t.Errorf("expected entry point endpoint:api:1, got %s", tr.EntryPoint)
	}
	if tr.Terminal != "infra:database" {
		t.Errorf("expected terminal infra:database, got %s", tr.Terminal)
	}
	if len(tr.Chain) != 3 {
		t.Errorf("expected chain length 3, got %d", len(tr.Chain))
	}
	if tr.Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %.2f", tr.Confidence)
	}
}

func TestComputeTraces_BranchingGraph(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "endpoint:api:1", Name: "POST /order", Type: model.NodeEndpoint})
	g.AddNode(&model.Node{ID: "pkg:service", Name: "service", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "infra:database", Name: "Database", Type: model.NodeDatabase})
	g.AddNode(&model.Node{ID: "infra:queue", Name: "Queue", Type: model.NodeQueue})

	g.AddEdge(&model.Edge{Source: "endpoint:api:1", Target: "pkg:service", Type: model.EdgeDependency, Confidence: 0.9})
	g.AddEdge(&model.Edge{Source: "pkg:service", Target: "infra:database", Type: model.EdgeReadWrite, Confidence: 0.8})
	g.AddEdge(&model.Edge{Source: "pkg:service", Target: "infra:queue", Type: model.EdgePublish, Confidence: 0.8})

	traces := ComputeTraces(g)
	if len(traces) != 2 {
		t.Fatalf("expected 2 traces (one per terminal), got %d", len(traces))
	}

	terminals := map[string]bool{}
	for _, tr := range traces {
		terminals[tr.Terminal] = true
		if tr.EntryPoint != "endpoint:api:1" {
			t.Errorf("expected entry point endpoint:api:1, got %s", tr.EntryPoint)
		}
	}
	if !terminals["infra:database"] || !terminals["infra:queue"] {
		t.Errorf("expected both database and queue terminals, got %v", terminals)
	}
}

func TestComputeTraces_CycleCapped(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "endpoint:api:1", Name: "GET /loop", Type: model.NodeEndpoint})
	g.AddNode(&model.Node{ID: "pkg:a", Name: "A", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:b", Name: "B", Type: model.NodePackage})

	g.AddEdge(&model.Edge{Source: "endpoint:api:1", Target: "pkg:a", Type: model.EdgeDependency, Confidence: 0.9})
	g.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:b", Type: model.EdgeDependency, Confidence: 0.9})
	g.AddEdge(&model.Edge{Source: "pkg:b", Target: "pkg:a", Type: model.EdgeDependency, Confidence: 0.9})

	// Should not infinite loop; visited set prevents cycles.
	traces := ComputeTraces(g)
	// The cycle means no terminal is reachable; traces may be empty or
	// contain partial leaf traces. The key assertion is that it terminates.
	_ = traces
}

func TestComputeTraces_NoEndpoints(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "pkg:a", Name: "A", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "pkg:b", Name: "B", Type: model.NodePackage})
	g.AddEdge(&model.Edge{Source: "pkg:a", Target: "pkg:b", Type: model.EdgeDependency, Confidence: 0.9})

	traces := ComputeTraces(g)
	if len(traces) != 0 {
		t.Fatalf("expected 0 traces without endpoints, got %d", len(traces))
	}
}

func TestComputeTraces_MinConfidenceAcrossChain(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "endpoint:api:1", Name: "GET /data", Type: model.NodeEndpoint})
	g.AddNode(&model.Node{ID: "pkg:mid", Name: "middleware", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "infra:cache", Name: "Cache", Type: model.NodeCache})

	g.AddEdge(&model.Edge{Source: "endpoint:api:1", Target: "pkg:mid", Type: model.EdgeDependency, Confidence: 0.9})
	g.AddEdge(&model.Edge{Source: "pkg:mid", Target: "infra:cache", Type: model.EdgeReadWrite, Confidence: 0.7})

	traces := ComputeTraces(g)
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	if traces[0].Confidence != 0.7 {
		t.Errorf("expected min confidence 0.7, got %.2f", traces[0].Confidence)
	}
}
