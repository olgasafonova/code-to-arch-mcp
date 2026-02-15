package render

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Structurizr renders an ArchGraph as Structurizr DSL.
func Structurizr(graph *model.ArchGraph, opts Options) string {
	var sb strings.Builder

	title := opts.Title
	if title == "" {
		title = "Architecture"
	}

	vg := FilterGraph(graph, opts.ViewLevel)
	vg.TransitiveReduce()

	sb.WriteString("workspace {\n")
	sb.WriteString("    model {\n")

	// External systems
	var externals []*model.Node
	var internals []*model.Node
	for _, n := range vg.Nodes {
		if n.Type == model.NodeExternalAPI {
			externals = append(externals, n)
		} else {
			internals = append(internals, n)
		}
	}

	for _, n := range externals {
		id := SanitizeID(n.ID)
		fmt.Fprintf(&sb, "        %s = softwareSystem \"%s\" {\n", id, n.Name)
		sb.WriteString("            tags \"External\"\n")
		sb.WriteString("        }\n")
	}

	// Software system with containers
	fmt.Fprintf(&sb, "        system = softwareSystem \"%s\" {\n", title)
	for _, n := range internals {
		id := SanitizeID(n.ID)
		structurizrContainer(&sb, n, id)
	}
	sb.WriteString("        }\n")

	// Relationships
	for _, e := range vg.Edges {
		label := e.Label
		if label == "" {
			label = string(e.Type)
		}
		fmt.Fprintf(&sb, "        %s -> %s \"%s\"\n",
			SanitizeID(e.Source),
			SanitizeID(e.Target),
			label,
		)
	}

	sb.WriteString("    }\n") // end model

	// Views
	sb.WriteString("    views {\n")
	sb.WriteString("        container system {\n")
	sb.WriteString("            include *\n")
	sb.WriteString("            autoLayout\n")
	sb.WriteString("        }\n")
	sb.WriteString("    }\n")

	sb.WriteString("}\n")
	return sb.String()
}

func structurizrContainer(sb *strings.Builder, n *model.Node, id string) {
	lang := n.Language
	if lang == "" {
		lang = ""
	}

	switch n.Type {
	case model.NodeDatabase:
		fmt.Fprintf(sb, "            %s = container \"%s\" \"\" \"%s\" {\n", id, n.Name, lang)
		sb.WriteString("                tags \"Database\"\n")
		sb.WriteString("            }\n")
	case model.NodeQueue:
		fmt.Fprintf(sb, "            %s = container \"%s\" \"\" \"%s\" {\n", id, n.Name, lang)
		sb.WriteString("                tags \"Queue\"\n")
		sb.WriteString("            }\n")
	case model.NodeCache:
		fmt.Fprintf(sb, "            %s = container \"%s\" \"\" \"%s\" {\n", id, n.Name, lang)
		sb.WriteString("                tags \"Cache\"\n")
		sb.WriteString("            }\n")
	default:
		fmt.Fprintf(sb, "            %s = container \"%s\" \"\" \"%s\"\n", id, n.Name, lang)
	}
}
