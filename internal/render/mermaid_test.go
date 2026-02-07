package render

import (
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestMermaid_BasicOutput(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API Server", Type: model.NodeService})
	g.AddNode(&model.Node{ID: "db:postgres", Name: "PostgreSQL", Type: model.NodeDatabase})
	g.AddEdge(&model.Edge{Source: "svc:api", Target: "db:postgres", Type: model.EdgeReadWrite, Label: "queries"})

	result := Mermaid(g, DefaultOptions())

	if !strings.Contains(result, "graph TB") {
		t.Fatal("expected Mermaid graph TB directive")
	}
	if !strings.Contains(result, "API Server") {
		t.Fatal("expected service node in output")
	}
	if !strings.Contains(result, "PostgreSQL") {
		t.Fatal("expected database node in output")
	}
	if !strings.Contains(result, "queries") {
		t.Fatal("expected edge label in output")
	}
}

func TestMermaid_DatabaseShape(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "db:pg", Name: "PG", Type: model.NodeDatabase})

	result := Mermaid(g, Options{Format: FormatMermaid, ViewLevel: ViewContainer})

	// Database nodes use cylinder shape [(name)]
	if !strings.Contains(result, "[(") {
		t.Fatal("expected database cylinder shape [( in output")
	}
}

func TestMermaid_ViewLevelFiltering(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	g.AddNode(&model.Node{ID: "pkg:utils", Name: "utils", Type: model.NodePackage})
	g.AddNode(&model.Node{ID: "ep:health", Name: "GET /health", Type: model.NodeEndpoint})

	// System level: only services and external APIs
	sysResult := Mermaid(g, Options{ViewLevel: ViewSystem})
	if strings.Contains(sysResult, "utils") {
		t.Fatal("system view should not show packages")
	}
	if strings.Contains(sysResult, "/health") {
		t.Fatal("system view should not show endpoints")
	}

	// Component level: everything
	compResult := Mermaid(g, Options{ViewLevel: ViewComponent})
	if !strings.Contains(compResult, "utils") {
		t.Fatal("component view should show packages")
	}
	if !strings.Contains(compResult, "/health") {
		t.Fatal("component view should show endpoints")
	}
}

func TestMermaid_CustomTitle(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	result := Mermaid(g, Options{Title: "My Architecture", ViewLevel: ViewContainer})
	if !strings.Contains(result, "title: My Architecture") {
		t.Fatal("expected custom title in output")
	}
}

func TestMermaid_LRDirection(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	result := Mermaid(g, Options{Direction: "LR", ViewLevel: ViewContainer})
	if !strings.Contains(result, "graph LR") {
		t.Fatal("expected LR direction")
	}
}

func TestSanitizeMermaidID(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"svc:api", "svc_api"},
		{"pkg/utils", "pkg_utils"},
		{"node.name", "node_name"},
		{"my-service", "my_service"},
	}
	for _, tt := range tests {
		got := sanitizeMermaidID(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeMermaidID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
