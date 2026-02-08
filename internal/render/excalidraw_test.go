package render

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestExcalidraw_ValidJSON(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := Excalidraw(graph, Options{ViewLevel: ViewContainer})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("Excalidraw output is not valid JSON: %v", err)
	}

	if parsed["type"] != "excalidraw" {
		t.Fatal("type field should be 'excalidraw'")
	}
}

func TestExcalidraw_HasElements(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})

	out := Excalidraw(graph, Options{ViewLevel: ViewContainer})

	var parsed map[string]any
	json.Unmarshal([]byte(out), &parsed)

	elements, ok := parsed["elements"].([]any)
	if !ok {
		t.Fatal("elements should be an array")
	}
	// 2 nodes * 2 (rect+text) = 4 elements minimum
	if len(elements) < 4 {
		t.Fatalf("expected at least 4 elements (2 nodes * 2), got %d", len(elements))
	}
}

func TestExcalidraw_ArrowBindings(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite})

	out := Excalidraw(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "startBinding") {
		t.Fatal("arrows should have startBinding")
	}
	if !strings.Contains(out, "endBinding") {
		t.Fatal("arrows should have endBinding")
	}
	if !strings.Contains(out, `"type": "arrow"`) {
		t.Fatal("edges should produce arrow elements")
	}
}

func TestExcalidraw_ViewLevelFiltering(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "pkg:utils", Name: "utils", Type: model.NodePackage})

	out := Excalidraw(graph, Options{ViewLevel: ViewSystem})

	if !strings.Contains(out, "API") {
		t.Fatal("system view should include services")
	}
	if strings.Contains(out, "utils") {
		t.Fatal("system view should exclude packages")
	}
}

func TestExcalidraw_EmptyGraph(t *testing.T) {
	graph := model.NewGraph("/tmp/test")

	out := Excalidraw(graph, Options{ViewLevel: ViewContainer})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("empty graph should produce valid JSON: %v", err)
	}

	elements, ok := parsed["elements"].([]any)
	if !ok {
		t.Fatal("elements should be an array")
	}
	if len(elements) != 0 {
		t.Fatalf("empty graph should have 0 elements, got %d", len(elements))
	}
}
