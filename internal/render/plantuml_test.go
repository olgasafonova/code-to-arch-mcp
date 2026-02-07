package render

import (
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestPlantUML_BasicOutput(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API Server", Type: model.NodeService})
	g.AddNode(&model.Node{ID: "db:postgres", Name: "PostgreSQL", Type: model.NodeDatabase})
	g.AddEdge(&model.Edge{Source: "svc:api", Target: "db:postgres", Type: model.EdgeReadWrite, Label: "queries"})

	result := PlantUML(g, Options{ViewLevel: ViewContainer})

	if !strings.Contains(result, "@startuml") {
		t.Fatal("expected @startuml")
	}
	if !strings.Contains(result, "@enduml") {
		t.Fatal("expected @enduml")
	}
	if !strings.Contains(result, "API Server") {
		t.Fatal("expected service node name")
	}
	if !strings.Contains(result, "PostgreSQL") {
		t.Fatal("expected database node name")
	}
	if !strings.Contains(result, "queries") {
		t.Fatal("expected edge label")
	}
}

func TestPlantUML_DatabaseShape(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "db:pg", Name: "PG", Type: model.NodeDatabase})

	result := PlantUML(g, Options{ViewLevel: ViewContainer})

	if !strings.Contains(result, `database "PG"`) {
		t.Fatal("expected database keyword for database node")
	}
}

func TestPlantUML_QueueShape(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "q:rabbit", Name: "RabbitMQ", Type: model.NodeQueue})

	result := PlantUML(g, Options{ViewLevel: ViewContainer})

	if !strings.Contains(result, `queue "RabbitMQ"`) {
		t.Fatal("expected queue keyword for queue node")
	}
}

func TestPlantUML_ViewLevelFiltering(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	g.AddNode(&model.Node{ID: "pkg:utils", Name: "utils", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "ep:health", Name: "GET /health", Type: model.NodeEndpoint})

	sysResult := PlantUML(g, Options{ViewLevel: ViewSystem})
	if strings.Contains(sysResult, "utils") {
		t.Fatal("system view should not show packages")
	}
	if strings.Contains(sysResult, "/health") {
		t.Fatal("system view should not show endpoints")
	}

	compResult := PlantUML(g, Options{ViewLevel: ViewComponent})
	if !strings.Contains(compResult, "utils") {
		t.Fatal("component view should show packages")
	}
	if !strings.Contains(compResult, "/health") {
		t.Fatal("component view should show endpoints")
	}
}

func TestPlantUML_CustomTitle(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	result := PlantUML(g, Options{Title: "My Architecture", ViewLevel: ViewContainer})
	if !strings.Contains(result, "title My Architecture") {
		t.Fatal("expected custom title")
	}
}

func TestPlantUML_LRDirection(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	result := PlantUML(g, Options{Direction: "LR", ViewLevel: ViewContainer})
	if !strings.Contains(result, "left to right direction") {
		t.Fatal("expected left to right direction directive")
	}
}

func TestPlantUML_EdgeArrows(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:a", Name: "A", Type: model.NodeService})
	g.AddNode(&model.Node{ID: "svc:b", Name: "B", Type: model.NodeService})
	g.AddNode(&model.Node{ID: "db:pg", Name: "PG", Type: model.NodeDatabase})

	g.AddEdge(&model.Edge{Source: "svc:a", Target: "svc:b", Type: model.EdgeDataFlow, Label: "stream"})
	g.AddEdge(&model.Edge{Source: "svc:a", Target: "db:pg", Type: model.EdgeReadWrite, Label: "rw"})
	g.AddEdge(&model.Edge{Source: "svc:b", Target: "svc:a", Type: model.EdgeDependency, Label: "dep"})

	result := PlantUML(g, Options{ViewLevel: ViewContainer})

	if !strings.Contains(result, "..>") {
		t.Fatal("expected dashed arrow ..> for DataFlow edge")
	}
	if !strings.Contains(result, "<-->") {
		t.Fatal("expected bidirectional arrow <--> for ReadWrite edge")
	}
	if !strings.Contains(result, "-->") {
		t.Fatal("expected solid arrow --> for dependency edge")
	}
}
