package render

import (
	"strings"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
)

func TestStructurizr_HasWorkspace(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := Structurizr(graph, Options{ViewLevel: ViewContainer})

	if !strings.HasPrefix(out, "workspace {") {
		t.Fatal("Structurizr output should start with 'workspace {'")
	}
}

func TestStructurizr_ContainsModel(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := Structurizr(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "model {") {
		t.Fatal("Structurizr output should contain 'model {'")
	}
}

func TestStructurizr_DatabaseTagged(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "infra:db", Name: "PostgreSQL", Type: model.NodeDatabase})

	out := Structurizr(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, `tags "Database"`) {
		t.Fatal("database container should have Database tag")
	}
	if !strings.Contains(out, "PostgreSQL") {
		t.Fatal("database name should appear")
	}
}

func TestStructurizr_QueueTagged(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "infra:queue", Name: "RabbitMQ", Type: model.NodeQueue})

	out := Structurizr(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, `tags "Queue"`) {
		t.Fatal("queue container should have Queue tag")
	}
}

func TestStructurizr_HasViews(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := Structurizr(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "views {") {
		t.Fatal("Structurizr output should contain views block")
	}
	if !strings.Contains(out, "autoLayout") {
		t.Fatal("views should include autoLayout")
	}
}

func TestStructurizr_ExternalSystem(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "infra:ext", Name: "External API", Type: model.NodeExternalAPI})

	out := Structurizr(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, `tags "External"`) {
		t.Fatal("external systems should have External tag")
	}
	if !strings.Contains(out, "softwareSystem") {
		t.Fatal("external APIs should be modeled as softwareSystem")
	}
}

func TestStructurizr_Relationships(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite, Label: "queries"})

	out := Structurizr(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "->") {
		t.Fatal("should have relationship arrow")
	}
	if !strings.Contains(out, "queries") {
		t.Fatal("edge label should appear in relationship")
	}
}
