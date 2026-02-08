package render

import (
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestDrawIO_ValidXML(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := DrawIO(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "<mxGraphModel>") {
		t.Fatal("draw.io output should start with <mxGraphModel>")
	}
	if !strings.Contains(out, "</mxGraphModel>") {
		t.Fatal("draw.io output should end with </mxGraphModel>")
	}
}

func TestDrawIO_HasNodeCells(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "Database", Type: model.NodeDatabase})

	out := DrawIO(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, `vertex="1"`) {
		t.Fatal("nodes should have vertex='1' attribute")
	}
	if !strings.Contains(out, "API") {
		t.Fatal("node name should appear in output")
	}
	if !strings.Contains(out, "Database") {
		t.Fatal("database name should appear in output")
	}
}

func TestDrawIO_HasEdgeCells(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite, Label: "queries"})

	out := DrawIO(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, `edge="1"`) {
		t.Fatal("edges should have edge='1' attribute")
	}
	if !strings.Contains(out, "queries") {
		t.Fatal("edge label should appear in output")
	}
}

func TestDrawIO_ViewLevelFiltering(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "pkg:utils", Name: "utils", Type: model.NodePackage})

	out := DrawIO(graph, Options{ViewLevel: ViewSystem})

	if !strings.Contains(out, "API") {
		t.Fatal("system view should include services")
	}
	if strings.Contains(out, "utils") {
		t.Fatal("system view should exclude packages")
	}
}

func TestDrawIO_DatabaseStyle(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})

	out := DrawIO(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "cylinder3") {
		t.Fatal("database should use cylinder3 shape")
	}
}

func TestDrawIO_XMLEscape(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API & Service <v2>", Type: model.NodeService})

	out := DrawIO(graph, Options{ViewLevel: ViewContainer})

	if strings.Contains(out, "& Service") && !strings.Contains(out, "&amp;") {
		t.Fatal("ampersand should be XML-escaped")
	}
	if strings.Contains(out, "<v2>") && !strings.Contains(out, "&lt;v2&gt;") {
		t.Fatal("angle brackets should be XML-escaped")
	}
}
