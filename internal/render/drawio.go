package render

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

const (
	drawIOCellWidth  = 200
	drawIOCellHeight = 60
	drawIOGapX       = 40
	drawIOGapY       = 80
	drawIOColsPerRow = 4
	drawIOMarginX    = 40
	drawIOMarginY    = 40
)

// DrawIO renders an ArchGraph as draw.io (mxGraphModel) XML.
func DrawIO(graph *model.ArchGraph, opts Options) string {
	var sb strings.Builder

	nodes := FilterNodesByViewLevel(graph.Nodes(), opts.ViewLevel)
	visibleIDs := make(map[string]bool)
	for _, n := range nodes {
		visibleIDs[n.ID] = true
	}

	sb.WriteString("<mxGraphModel>\n")
	sb.WriteString("  <root>\n")
	sb.WriteString("    <mxCell id=\"0\"/>\n")
	sb.WriteString("    <mxCell id=\"1\" parent=\"0\"/>\n")

	// Layout nodes in a grid
	for i, n := range nodes {
		id := SanitizeID(n.ID)
		style := drawIOStyle(n.Type)
		col := i % drawIOColsPerRow
		row := i / drawIOColsPerRow
		x := drawIOMarginX + col*(drawIOCellWidth+drawIOGapX)
		y := drawIOMarginY + row*(drawIOCellHeight+drawIOGapY)

		fmt.Fprintf(&sb, "    <mxCell id=\"%s\" value=\"%s\" style=\"%s\" vertex=\"1\" parent=\"1\">\n",
			id, xmlEscape(n.Name), style)
		fmt.Fprintf(&sb, "      <mxGeometry x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" as=\"geometry\"/>\n",
			x, y, drawIOCellWidth, drawIOCellHeight)
		sb.WriteString("    </mxCell>\n")
	}

	// Render edges
	for i, e := range graph.Edges() {
		if !visibleIDs[e.Source] || !visibleIDs[e.Target] {
			continue
		}
		label := e.Label
		if label == "" {
			label = string(e.Type)
		}
		edgeID := fmt.Sprintf("edge_%d", i)
		fmt.Fprintf(&sb, "    <mxCell id=\"%s\" value=\"%s\" style=\"edgeStyle=orthogonalEdgeStyle;\" edge=\"1\" source=\"%s\" target=\"%s\" parent=\"1\">\n",
			edgeID, xmlEscape(label), SanitizeID(e.Source), SanitizeID(e.Target))
		sb.WriteString("      <mxGeometry relative=\"1\" as=\"geometry\"/>\n")
		sb.WriteString("    </mxCell>\n")
	}

	sb.WriteString("  </root>\n")
	sb.WriteString("</mxGraphModel>\n")
	return sb.String()
}

func drawIOStyle(t model.NodeType) string {
	switch t {
	case model.NodeDatabase:
		return "shape=cylinder3;whiteSpace=wrap;html=1;boundedLbl=1;backgroundOutline=1;size=15;fillColor=#d5e8d4;strokeColor=#82b366;"
	case model.NodeQueue:
		return "shape=parallelogram;whiteSpace=wrap;html=1;fillColor=#fff2cc;strokeColor=#d6b656;"
	case model.NodeCache:
		return "rounded=1;whiteSpace=wrap;html=1;dashed=1;fillColor=#f8cecc;strokeColor=#b85450;"
	case model.NodeExternalAPI:
		return "shape=cloud;whiteSpace=wrap;html=1;fillColor=#e1d5e7;strokeColor=#9673a6;"
	case model.NodeEndpoint:
		return "shape=hexagon;whiteSpace=wrap;html=1;fillColor=#f5f5f5;strokeColor=#666666;"
	default:
		return "rounded=1;whiteSpace=wrap;html=1;fillColor=#dae8fc;strokeColor=#6c8ebf;"
	}
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
