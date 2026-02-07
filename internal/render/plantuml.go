package render

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
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

	nodes := graph.Nodes()
	edges := graph.Edges()

	visible := FilterNodesByViewLevel(nodes, opts.ViewLevel)
	visibleIDs := make(map[string]bool)
	for _, n := range visible {
		visibleIDs[n.ID] = true
	}

	// Render nodes grouped by type
	plantUMLNodeGroup(&sb, visible, model.NodeService, "Services")
	plantUMLNodeGroup(&sb, visible, model.NodeModule, "Modules")
	plantUMLNodeGroup(&sb, visible, model.NodePackage, "Packages")
	plantUMLNodeGroup(&sb, visible, model.NodeDatabase, "Data Stores")
	plantUMLNodeGroup(&sb, visible, model.NodeQueue, "Message Queues")
	plantUMLNodeGroup(&sb, visible, model.NodeCache, "Caches")
	plantUMLNodeGroup(&sb, visible, model.NodeExternalAPI, "External APIs")
	plantUMLNodeGroup(&sb, visible, model.NodeEndpoint, "Endpoints")

	// Render edges between visible nodes
	for _, e := range edges {
		if !visibleIDs[e.Source] || !visibleIDs[e.Target] {
			continue
		}
		label := e.Label
		if label == "" {
			label = string(e.Type)
		}
		arrow := plantUMLArrow(e.Type)
		fmt.Fprintf(&sb, "%s %s %s : %s\n",
			SanitizeID(e.Source),
			arrow,
			SanitizeID(e.Target),
			label,
		)
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
