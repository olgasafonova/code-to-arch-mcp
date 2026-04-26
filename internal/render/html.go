package render

import (
	_ "embed"
	"fmt"
	"html"
	"strings"

	"github.com/olgasafonova/ridge/internal/model"
)

// mermaidJS is the Mermaid runtime, embedded for offline rendering.
// See assets/SOURCES.md for version, source URL, and refresh procedure.
//
//go:embed assets/mermaid.min.js
var mermaidJS string

// HTML renders an ArchGraph as a single self-contained HTML document with
// the Mermaid runtime embedded inline. Output works fully offline; no
// network requests are issued when the file is opened in a browser.
//
// Output size is roughly 900 KB (Mermaid runtime ~880 KB plus the diagram
// source and a small wrapper). For larger graphs that exceed Mermaid's
// practical layout limit, prefer FormatExcalidraw or FormatDrawIO.
//
// Mermaid's strict securityLevel is enabled; user-controlled node names
// are escaped during HTML emission and Mermaid additionally sanitizes
// label content. Diagram source is HTML-escaped before insertion into the
// `<pre class="mermaid">` block so the browser's HTML parser does not
// misinterpret characters like `<` and `>` that may appear in node labels.
func HTML(graph *model.ArchGraph, opts Options) string {
	mermaidOpts := opts
	mermaidOpts.Format = FormatMermaid
	diagram := Mermaid(graph, mermaidOpts)

	title := opts.Title
	if title == "" {
		title = "Architecture diagram"
	}

	var sb strings.Builder
	sb.Grow(len(mermaidJS) + len(diagram) + 1024)

	sb.WriteString("<!doctype html>\n")
	sb.WriteString("<html lang=\"en\">\n<head>\n")
	sb.WriteString("<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&sb, "<title>%s</title>\n", html.EscapeString(title))
	sb.WriteString("<style>")
	sb.WriteString("body{margin:0;padding:24px;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#fafafa;color:#111}")
	sb.WriteString("h1{font-size:18px;margin:0 0 16px;font-weight:600}")
	sb.WriteString(".mermaid{background:#fff;padding:24px;border-radius:8px;box-shadow:0 1px 3px rgba(0,0,0,.08);overflow-x:auto}")
	sb.WriteString("</style>\n</head>\n<body>\n")
	fmt.Fprintf(&sb, "<h1>%s</h1>\n", html.EscapeString(title))
	sb.WriteString("<pre class=\"mermaid\">\n")
	sb.WriteString(html.EscapeString(diagram))
	sb.WriteString("\n</pre>\n")
	sb.WriteString("<script>\n")
	sb.WriteString(mermaidJS)
	sb.WriteString("\n</script>\n")
	sb.WriteString("<script>mermaid.initialize({startOnLoad:true,securityLevel:\"strict\",theme:\"default\",maxTextSize:1000000,maxEdges:5000});</script>\n")
	sb.WriteString("</body>\n</html>\n")
	return sb.String()
}
