package render

import (
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestHTML_BasicStructure(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API Server", Type: model.NodeService})
	g.AddNode(&model.Node{ID: "db:pg", Name: "Postgres", Type: model.NodeDatabase})
	g.AddEdge(&model.Edge{Source: "svc:api", Target: "db:pg", Type: model.EdgeReadWrite})

	out := HTML(g, Options{Format: FormatHTML, ViewLevel: ViewContainer, Title: "Test diagram"})

	for _, want := range []string{
		"<!doctype html>",
		`<html lang="en">`,
		`<meta charset="utf-8">`,
		"<title>Test diagram</title>",
		`<pre class="mermaid">`,
		"mermaid.initialize",
		`securityLevel:"strict"`,
		"API Server",
		"Postgres",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q", want)
		}
	}
}

func TestHTML_EmbedsMermaidRuntime(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := HTML(g, Options{Format: FormatHTML, ViewLevel: ViewContainer})

	// Mermaid v9 minified bundle contains its package version string.
	// If the embedded asset goes missing or is replaced with an empty
	// file, this assertion fails before the user sees a broken page.
	if !strings.Contains(out, "mermaid") {
		t.Fatal("output should contain the embedded mermaid library marker")
	}
	if len(out) < 500_000 {
		t.Fatalf("output is %d bytes; expected at least 500 KB once the runtime is embedded", len(out))
	}
}

func TestHTML_SelfContained(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := HTML(g, Options{Format: FormatHTML, ViewLevel: ViewContainer})

	// No external script or stylesheet references should appear in the
	// rendered HTML. The Mermaid runtime is inline; everything else is
	// either a <style> block or text content.
	//
	// Note: bare URL strings like "http://" are NOT a useful signal here —
	// the embedded Mermaid library legitimately contains URLs such as
	// http://www.w3.org/2000/svg for SVG namespace declarations. The check
	// targets structural HTML elements that actually trigger network fetches.
	for _, banned := range []string{
		`<script src=`,
		`<link href=`,
		`<link rel="stylesheet"`,
		`@import url`,
	} {
		if strings.Contains(out, banned) {
			t.Errorf("HTML output should be self-contained; found external reference %q", banned)
		}
	}
}

func TestHTML_DefaultTitle(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := HTML(g, Options{Format: FormatHTML, ViewLevel: ViewContainer})

	if !strings.Contains(out, "<title>Architecture diagram</title>") {
		t.Fatal("expected default title 'Architecture diagram' when Title field is empty")
	}
}

func TestHTML_EscapesTitle(t *testing.T) {
	g := model.NewGraph("/tmp")
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService})

	out := HTML(g, Options{
		Format:    FormatHTML,
		ViewLevel: ViewContainer,
		Title:     `<script>alert(1)</script>`,
	})

	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Fatal("title field must be HTML-escaped to prevent injection")
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("expected escaped title in output")
	}
}

func TestHTML_EscapesDiagramContent(t *testing.T) {
	g := model.NewGraph("/tmp")
	// Node name with characters that must be escaped before reaching the browser parser.
	g.AddNode(&model.Node{ID: "svc:api", Name: "io.Reader<T>", Type: model.NodeService})

	out := HTML(g, Options{Format: FormatHTML, ViewLevel: ViewContainer})

	// The diagram source contains the literal node name. After HTML escaping,
	// the angle brackets must appear as entities inside the <pre> block so
	// the browser does not interpret <T> as a tag.
	if strings.Contains(out, "io.Reader<T>") && !strings.Contains(out, "io.Reader&lt;T&gt;") {
		t.Fatal("diagram content with angle brackets must be HTML-escaped in the <pre> block")
	}
}
