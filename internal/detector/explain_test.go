package detector

import (
	"strings"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
)

func TestExplainArchitecture_MonolithReason(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	boundaries := &BoundaryResult{
		Topology: model.TopologyMonolith,
		Boundaries: []Boundary{
			{Name: "root", Path: "/tmp/test", Type: "service"},
		},
	}

	exp := ExplainArchitecture(graph, boundaries)

	if !strings.Contains(exp.TopologyReason, "monolith") {
		t.Fatalf("expected monolith in topology reason, got: %s", exp.TopologyReason)
	}
}

func TestExplainArchitecture_DetectsSharedDB(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "svc:worker", Name: "Worker", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "mod:auth", Name: "Auth", Type: model.NodeModule})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "PostgreSQL", Type: model.NodeDatabase, Properties: map[string]string{"detected_via": "pg"}})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite})
	graph.AddEdge(&model.Edge{Source: "svc:worker", Target: "infra:db", Type: model.EdgeReadWrite})

	exp := ExplainArchitecture(graph, nil)

	foundSharedDB := false
	for _, p := range exp.Patterns {
		if strings.Contains(p, "Shared database") {
			foundSharedDB = true
		}
	}
	if !foundSharedDB {
		t.Fatalf("expected shared database pattern, got patterns: %v", exp.Patterns)
	}

	foundRisk := false
	for _, r := range exp.Risks {
		if strings.Contains(r, "Single database") {
			foundRisk = true
		}
	}
	if !foundRisk {
		t.Fatalf("expected shared database risk, got risks: %v", exp.Risks)
	}
}

func TestExplainArchitecture_DetectsEventDriven(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:queue", Name: "Kafka", Type: model.NodeQueue, Properties: map[string]string{"detected_via": "kafkajs"}})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:queue", Type: model.EdgePublish})

	exp := ExplainArchitecture(graph, nil)

	foundEventDriven := false
	for _, p := range exp.Patterns {
		if strings.Contains(p, "Event-driven") {
			foundEventDriven = true
		}
	}
	if !foundEventDriven {
		t.Fatalf("expected event-driven pattern, got patterns: %v", exp.Patterns)
	}
}

func TestExplainArchitecture_DecisionsFromInfra(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService, Language: "go"})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "Database", Type: model.NodeDatabase, Properties: map[string]string{"detected_via": "pgx"}})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite})

	exp := ExplainArchitecture(graph, nil)

	foundDBDecision := false
	for _, d := range exp.KeyDecisions {
		if strings.Contains(d, "pgx") {
			foundDBDecision = true
		}
	}
	if !foundDBDecision {
		t.Fatalf("expected database decision mentioning pgx, got: %v", exp.KeyDecisions)
	}
}

func TestExplainArchitecture_NilBoundaries(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	exp := ExplainArchitecture(graph, nil)

	if exp.TopologyReason == "" {
		t.Fatal("topology reason should not be empty even with nil boundaries")
	}
}
