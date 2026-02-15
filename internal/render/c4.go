package render

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// C4 renders an ArchGraph using C4-PlantUML notation.
func C4(graph *model.ArchGraph, opts Options) string {
	var sb strings.Builder

	title := opts.Title
	if title == "" {
		title = "Architecture"
	}

	vg := FilterGraph(graph, opts.ViewLevel)
	vg.TransitiveReduce()

	sb.WriteString("@startuml\n")
	sb.WriteString("!include <C4/C4_Container>\n\n")
	fmt.Fprintf(&sb, "title %s\n\n", title)

	// External systems go outside the boundary
	var externals []*model.Node
	var internals []*model.Node
	for _, n := range vg.Nodes {
		if n.Type == model.NodeExternalAPI {
			externals = append(externals, n)
		} else {
			internals = append(internals, n)
		}
	}

	// Render external systems
	for _, n := range externals {
		id := SanitizeID(n.ID)
		fmt.Fprintf(&sb, "System_Ext(%s, \"%s\")\n", id, n.Name)
	}
	if len(externals) > 0 {
		sb.WriteString("\n")
	}

	// Render system boundary with internals
	if len(internals) > 0 {
		sb.WriteString("System_Boundary(system, \"System\") {\n")
		for _, n := range internals {
			id := SanitizeID(n.ID)
			c4Element(&sb, n, id)
		}
		sb.WriteString("}\n\n")
	}

	// Render relationships
	for _, e := range vg.Edges {
		label := e.Label
		if label == "" {
			label = string(e.Type)
		}
		fmt.Fprintf(&sb, "Rel(%s, %s, \"%s\")\n",
			SanitizeID(e.Source),
			SanitizeID(e.Target),
			label,
		)
	}

	sb.WriteString("\n@enduml\n")
	return sb.String()
}

func c4Element(sb *strings.Builder, n *model.Node, id string) {
	lang := n.Language
	if lang == "" {
		lang = ""
	}

	switch n.Type {
	case model.NodeDatabase:
		fmt.Fprintf(sb, "    ContainerDb(%s, \"%s\", \"%s\", \"Data store\")\n", id, n.Name, lang)
	case model.NodeQueue:
		fmt.Fprintf(sb, "    Container(%s, \"%s\", \"%s\", \"Message queue\") <<queue>>\n", id, n.Name, lang)
	case model.NodeCache:
		fmt.Fprintf(sb, "    Container(%s, \"%s\", \"%s\", \"Cache\") <<cache>>\n", id, n.Name, lang)
	case model.NodeService, model.NodeModule:
		fmt.Fprintf(sb, "    Container(%s, \"%s\", \"%s\", \"Service\")\n", id, n.Name, lang)
	case model.NodePackage:
		fmt.Fprintf(sb, "    Component(%s, \"%s\", \"%s\", \"Package\")\n", id, n.Name, lang)
	case model.NodeEndpoint:
		fmt.Fprintf(sb, "    Component(%s, \"%s\", \"%s\", \"Endpoint\")\n", id, n.Name, lang)
	default:
		fmt.Fprintf(sb, "    Container(%s, \"%s\", \"%s\", \"\")\n", id, n.Name, lang)
	}
}
