package render

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
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

func TestDrawIO_TopologicalLayout(t *testing.T) {
	// Build a 3-layer graph: app → svc → db
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "pkg:app", Name: "app", Type: model.NodePackage})
	graph.AddNode(&model.Node{ID: "pkg:svc", Name: "svc", Type: model.NodePackage})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "db", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "pkg:app", Target: "pkg:svc", Type: model.EdgeDependency})
	graph.AddEdge(&model.Edge{Source: "pkg:svc", Target: "infra:db", Type: model.EdgeReadWrite})

	out := DrawIO(graph, Options{ViewLevel: ViewComponent})

	// Extract Y coordinates for each node.
	yOf := func(name string) int {
		// find the mxCell with value="name", then parse y from its mxGeometry
		re := regexp.MustCompile(`value="` + name + `".*?\n\s*<mxGeometry[^>]*y="(\d+)"`)
		m := re.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("could not find y coordinate for %s", name)
		}
		y, _ := strconv.Atoi(m[1])
		return y
	}

	appY := yOf("app")
	svcY := yOf("svc")
	dbY := yOf("db")

	// Root (app) should be above svc, svc above db.
	if appY >= svcY {
		t.Errorf("app (y=%d) should be above svc (y=%d)", appY, svcY)
	}
	if svcY >= dbY {
		t.Errorf("svc (y=%d) should be above db (y=%d)", svcY, dbY)
	}
}

func TestDrawIO_CurvedEdges(t *testing.T) {
	graph := model.NewGraph("/tmp/test")
	graph.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})
	graph.AddNode(&model.Node{ID: "infra:db", Name: "DB", Type: model.NodeDatabase})
	graph.AddEdge(&model.Edge{Source: "svc:api", Target: "infra:db", Type: model.EdgeReadWrite, Label: "queries"})

	out := DrawIO(graph, Options{ViewLevel: ViewContainer})

	if !strings.Contains(out, "curved=1") {
		t.Fatal("edges should use curved style")
	}
	if strings.Contains(out, "orthogonalEdgeStyle") {
		t.Fatal("edges should not use orthogonal style")
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
