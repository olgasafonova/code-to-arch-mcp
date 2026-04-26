package render

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/ridge/internal/model"
)

// PlantUML renders an ArchGraph as a PlantUML diagram.
func PlantUML(graph *model.ArchGraph, opts Options) string {
	var sb strings.Builder

	title := opts.Title
	if title == "" {
		title = "Architecture"
	}

	sb.WriteString("@startuml\n")
	fmt.Fprintf(&sb, "title %s\n", title)

	if opts.Direction == "LR" {
		sb.WriteString("left to right direction\n")
	}

	sb.WriteString("\n")

	vg := PrepareGraph(graph, opts)
	vg.TransitiveReduce()

	// Render nodes grouped by type
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodeService, "Services")
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodeModule, "Modules")
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodePackage, "Packages")
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodeDatabase, "Data Stores")
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodeQueue, "Message Queues")
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodeCache, "Caches")
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodeExternalAPI, "External APIs")
	plantUMLNodeGroup(&sb, vg.Nodes, model.NodeEndpoint, "Endpoints")

	// Render edges between visible nodes
	for _, e := range vg.Edges {
		label := EdgeLabel(e, vg.Names[e.Target])
		arrow := plantUMLArrow(e.Type)
		if label != "" {
			fmt.Fprintf(&sb, "%s %s %s : %s\n",
				SanitizeID(e.Source), arrow, SanitizeID(e.Target), label)
		} else {
			fmt.Fprintf(&sb, "%s %s %s\n",
				SanitizeID(e.Source), arrow, SanitizeID(e.Target))
		}
	}

	sb.WriteString("\n@enduml\n")
	return sb.String()
}

func plantUMLNodeGroup(sb *strings.Builder, nodes []*model.Node, nodeType model.NodeType, groupLabel string) {
	var group []*model.Node
	for _, n := range nodes {
		if n.Type == nodeType {
			group = append(group, n)
		}
	}
	if len(group) == 0 {
		return
	}

	fmt.Fprintf(sb, "package \"%s\" {\n", groupLabel)
	for _, n := range group {
		id := SanitizeID(n.ID)
		keyword := plantUMLKeyword(n.Type)
		fmt.Fprintf(sb, "    %s \"%s\" as %s\n", keyword, n.Name, id)
	}
	sb.WriteString("}\n\n")
}

func plantUMLKeyword(t model.NodeType) string {
	switch t {
	case model.NodeService:
		return "component"
	case model.NodeDatabase:
		return "database"
	case model.NodeQueue:
		return "queue"
	case model.NodeCache:
		return "storage"
	case model.NodeExternalAPI:
		return "cloud"
	case model.NodeEndpoint:
		return "usecase"
	case model.NodeModule:
		return "rectangle"
	case model.NodePackage:
		return "rectangle"
	default:
		return "rectangle"
	}
}

func plantUMLArrow(t model.EdgeType) string {
	switch t {
	case model.EdgeDataFlow:
		return "..>"
	case model.EdgeReadWrite:
		return "<-->"
	default:
		return "-->"
	}
}
