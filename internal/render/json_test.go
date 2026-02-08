package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestJSON_ValidOutput(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "Database", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite, Label: "queries"})

	out := JSON(graph, Options{ViewLevel: ViewContainer, Title: "Test"})

	if !strings.Contains(out, `"title": "Test"`) {
		t.Fatal("expected title field in JSON output")
	}
	if !strings.Contains(out, `"API"`) {
		t.Fatal("expected API node in JSON output")
	}
	if !strings.Contains(out, `"Database"`) {
		t.Fatal("expected Database node in JSON output")
	}
}

func TestJSON_ViewLevelFiltering(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "pkg:utils", Name: "utils", Type: model.NodePackage})

	out := JSON(graph, Options{ViewLevel: ViewSystem})

	if !strings.Contains(out, "API") {
		t.Fatal("system view should include services")
	}
	if strings.Contains(out, "utils") {
		t.Fatal("system view should exclude packages")
	}
}

func TestJSON_Parseable(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := JSON(graph, Options{ViewLevel: ViewContainer})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON output is not valid: %v", err)
	}

	if _, ok := parsed["nodes"]; !ok {
		t.Fatal("parsed JSON missing 'nodes' key")
	}
	if _, ok := parsed["edges"]; !ok {
		t.Fatal("parsed JSON missing 'edges' key")
	}
}

func TestJSON_EmptyGraph(t *testing.T) {
	graph := model.NewGraph("/tmp/test")

	out := JSON(graph, Options{ViewLevel: ViewContainer})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("empty graph JSON is not valid: %v", err)
	}

	// Should have [] not null for empty arrays
	if !strings.Contains(out, `"nodes": []`) {
		t.Fatal("empty nodes should serialize as []")
	}
}
