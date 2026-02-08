// Package render provides diagram output renderers.
package render

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Format enumerates supported output formats.
type Format string

const (
	FormatMermaid     Format = "mermaid"
	FormatPlantUML    Format = "plantuml"
	FormatC4          Format = "c4"
	FormatStructurizr Format = "structurizr"
	FormatJSON        Format = "json"
	FormatDrawIO      Format = "drawio"
	FormatExcalidraw  Format = "excalidraw"
)

// ViewLevel controls the level of detail in rendered output.
type ViewLevel string

const (
	ViewSystem    ViewLevel = "system"    // High-level service overview
	ViewContainer ViewLevel = "container" // Services + databases + queues
	ViewComponent ViewLevel = "component" // Internal packages and modules
)

// Options controls rendering behavior.
type Options struct {
	Format    Format
	ViewLevel ViewLevel
	Title     string
	Direction string // TB, LR, RL, BT (Mermaid direction)
}

// DefaultOptions returns sensible rendering defaults.
func DefaultOptions() Options {
	return Options{
		Format:    FormatMermaid,
		ViewLevel: ViewContainer,
		Direction: "TB",
	}
}

// Mermaid renders an ArchGraph as a Mermaid diagram.
func Mermaid(graph *model.ArchGraph, opts Options) string {
	var sb strings.Builder

	direction := opts.Direction
	if direction == "" {
		direction = "TB"
	}

	title := opts.Title
	if title == "" {
		title = "Architecture"
	}

	fmt.Fprintf(&sb, "---\ntitle: %s\n---\n", title)
	fmt.Fprintf(&sb, "graph %s\n", direction)

	vg := FilterGraph(graph, opts.ViewLevel)

	// Render nodes grouped by type
	renderNodeGroup(&sb, vg.Nodes, model.NodeService, "Services")
	renderNodeGroup(&sb, vg.Nodes, model.NodeModule, "Modules")
	renderNodeGroup(&sb, vg.Nodes, model.NodePackage, "Packages")
	renderNodeGroup(&sb, vg.Nodes, model.NodeDatabase, "Data Stores")
	renderNodeGroup(&sb, vg.Nodes, model.NodeQueue, "Message Queues")
	renderNodeGroup(&sb, vg.Nodes, model.NodeCache, "Caches")
	renderNodeGroup(&sb, vg.Nodes, model.NodeExternalAPI, "External APIs")
	renderNodeGroup(&sb, vg.Nodes, model.NodeEndpoint, "Endpoints")

	// Render edges between visible nodes
	for _, e := range vg.Edges {
		label := e.Label
		if label == "" {
			label = string(e.Type)
		}
		fmt.Fprintf(&sb, "    %s -->|%s| %s\n",
			SanitizeID(e.Source),
			label,
			SanitizeID(e.Target),
		)
	}

	return sb.String()
}

func renderNodeGroup(sb *strings.Builder, nodes []*model.Node, nodeType model.NodeType, groupLabel string) {
	var group []*model.Node
	for _, n := range nodes {
		if n.Type == nodeType {
			group = append(group, n)
		}
	}
	if len(group) == 0 {
		return
	}

	fmt.Fprintf(sb, "    subgraph %s\n", groupLabel)
	for _, n := range group {
		id := SanitizeID(n.ID)
		shape := mermaidShape(n.Type)
		fmt.Fprintf(sb, "        %s%s%s%s\n", id, shape.open, n.Name, shape.close)
	}
	sb.WriteString("    end\n")
}

type shapeDelimiters struct {
	open, close string
}

func mermaidShape(t model.NodeType) shapeDelimiters {
	switch t {
	case model.NodeDatabase:
		return shapeDelimiters{"[(", ")]"}
	case model.NodeQueue:
		return shapeDelimiters{"[[", "]]"}
	case model.NodeCache:
		return shapeDelimiters{"((", "))"}
	case model.NodeExternalAPI:
		return shapeDelimiters{">", "]"}
	case model.NodeEndpoint:
		return shapeDelimiters{"{{", "}}"}
	default:
		return shapeDelimiters{"[", "]"}
	}
}
