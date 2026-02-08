package render

import (
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestC4_HasIncludes(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := C4(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "!include <C4/C4_Container>") {
		t.Fatal("C4 output missing include directive")
	}
	if !strings.Contains(out, "@startuml") {
		t.Fatal("C4 output missing @startuml")
	}
	if !strings.Contains(out, "@enduml") {
		t.Fatal("C4 output missing @enduml")
	}
}

func TestC4_DatabaseUsesContainerDb(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "infra:db", Name: "PostgreSQL", Type: model.NodeDatabase})

	out := C4(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "ContainerDb(") {
		t.Fatal("database should render as ContainerDb")
	}
	if !strings.Contains(out, "PostgreSQL") {
		t.Fatal("database name should appear in output")
	}
}

func TestC4_ExternalUsesSystemExt(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "infra:external_api", Name: "External API", Type: model.NodeExternalAPI})

	out := C4(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "System_Ext(") {
		t.Fatal("external API should render as System_Ext")
	}
}

func TestC4_ViewLevelFiltering(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "pkg:utils", Name: "utils", Type: model.NodePackage})

	out := C4(graph, Options{ViewLevel: ViewSystem})

	if !strings.Contains(out, "API") {
		t.Fatal("system view should include services")
	}
	if strings.Contains(out, "utils") {
		t.Fatal("system view should exclude packages")
	}
}

func TestC4_SystemBoundary(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})

	out := C4(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "System_Boundary(") {
		t.Fatal("should have System_Boundary for internal nodes")
	}
}

func TestC4_Relationships(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite, Label: "queries"})

	out := C4(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "Rel(") {
		t.Fatal("should have Rel() for edges")
	}
	if !strings.Contains(out, "queries") {
		t.Fatal("edge label should appear in Rel")
	}
}
