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

func TestMermaid_ImportEdgeResolution(t *testing.T) {
	// Simulate what the Go analyzer produces: package nodes with absolute paths,
	// and dependency edges targeting "import:" prefixed full module paths.
	g := model.NewGraph("/home/user/myproject")
	g.AddNode(&model.Node{
		ID:   "pkg:tools/tools",
		Name: "tools",
		Type: model.NodePackage,
		Path: "/home/user/myproject/tools",
	})
	g.AddNode(&model.Node{
		ID:   "pkg:model/model",
		Name: "model",
		Type: model.NodePackage,
		Path: "/home/user/myproject/internal/model",
	})
	g.AddNode(&model.Node{
		ID:   "pkg:render/render",
		Name: "render",
		Type: model.NodePackage,
		Path: "/home/user/myproject/internal/render",
	})

	// Internal import edges (should resolve to package nodes)
	g.AddEdge(&model.Edge{
		Source: "pkg:tools/tools",
		Target: "import:github.com/user/myproject/internal/model",
		Type:   model.EdgeDependency,
		Label:  "github.com/user/myproject/internal/model",
	})
	g.AddEdge(&model.Edge{
		Source: "pkg:tools/tools",
		Target: "import:github.com/user/myproject/internal/render",
		Type:   model.EdgeDependency,
		Label:  "github.com/user/myproject/internal/render",
	})
	g.AddEdge(&model.Edge{
		Source: "pkg:render/render",
		Target: "import:github.com/user/myproject/internal/model",
		Type:   model.EdgeDependency,
		Label:  "github.com/user/myproject/internal/model",
	})

	// External import edge (should NOT appear — no matching node)
	g.AddEdge(&model.Edge{
		Source: "pkg:tools/tools",
		Target: "import:fmt",
		Type:   model.EdgeDependency,
		Label:  "fmt",
	})

	result := Mermaid(g, Options{ViewLevel: ViewComponent})

	// Resolved internal edges should produce arrows
	if !strings.Contains(result, "-->") {
		t.Fatalf("expected resolved edges in Mermaid output, got:\n%s", result)
	}

	// Should have edges from tools to model and render
	toolsID := SanitizeID("pkg:tools/tools")
	modelID := SanitizeID("pkg:model/model")
	renderID := SanitizeID("pkg:render/render")

	if !strings.Contains(result, toolsID+" -->") {
		t.Errorf("expected edge from tools, got:\n%s", result)
	}
	if !strings.Contains(result, modelID) {
		t.Errorf("expected model node in output, got:\n%s", result)
	}
	if !strings.Contains(result, renderID) {
		t.Errorf("expected render node in output, got:\n%s", result)
	}

	// External import "fmt" should NOT produce an edge
	if strings.Contains(result, "fmt") {
		t.Errorf("external import 'fmt' should not appear as edge, got:\n%s", result)
	}
}

func TestFilterGraph_DeduplicatesResolvedEdges(t *testing.T) {
	g := model.NewGraph("/project")
	g.AddNode(&model.Node{
		ID:   "pkg:a/a",
		Name: "a",
		Type: model.NodePackage,
		Path: "/project/a",
	})
	g.AddNode(&model.Node{
		ID:   "pkg:b/b",
		Name: "b",
		Type: model.NodePackage,
		Path: "/project/b",
	})

	// Two files in package "a" both import package "b" — produces duplicate edges
	g.AddEdge(&model.Edge{
		Source: "pkg:a/a",
		Target: "import:github.com/x/project/b",
		Type:   model.EdgeDependency,
	})
	g.AddEdge(&model.Edge{
		Source: "pkg:a/a",
		Target: "import:github.com/x/project/b",
		Type:   model.EdgeDependency,
	})

	vg := FilterGraph(g, ViewComponent)

	if len(vg.Edges) != 1 {
		t.Errorf("expected 1 deduplicated edge, got %d", len(vg.Edges))
	}
}

func TestFilterGraph_SkipsSelfEdges(t *testing.T) {
	g := model.NewGraph("/project")
	g.AddNode(&model.Node{
		ID:   "pkg:a/a",
		Name: "a",
		Type: model.NodePackage,
		Path: "/project/a",
	})

	// An edge that resolves to the same package (self-import)
	g.AddEdge(&model.Edge{
		Source: "pkg:a/a",
		Target: "import:github.com/x/project/a",
		Type:   model.EdgeDependency,
	})

	vg := FilterGraph(g, ViewComponent)

	if len(vg.Edges) != 0 {
		t.Errorf("expected 0 edges (self-edge skipped), got %d", len(vg.Edges))
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"svc:api", "svc___api"},
		{"pkg/utils", "pkg__utils"},
		{"node.name", "node_name"},
		{"my-service", "my_service"},
		// Verify different separators produce distinct IDs
		{"api/v1", "api__v1"},
		{"api.v1", "api_v1"},
		{"api:v1", "api___v1"},
	}
	for _, tt := range tests {
		got := SanitizeID(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
