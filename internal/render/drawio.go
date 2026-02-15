package render

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

const (
	drawIOCellW   = 200
	drawIOCellH   = 60
	drawIOGapX    = 60
	drawIOGapY    = 120
	drawIOMarginX = 40
	drawIOMarginY = 40
)

// DrawIO renders an ArchGraph as draw.io (mxGraphModel) XML.
// Nodes are arranged in topological layers: root packages at the top,
// leaf dependencies at the bottom.
func DrawIO(graph *model.ArchGraph, opts Options) string {
	var sb strings.Builder

	vg := FilterGraph(graph, opts.ViewLevel)
	vg.TransitiveReduce()

	sb.WriteString("<mxGraphModel>\n")
	sb.WriteString("  <root>\n")
	sb.WriteString("    <mxCell id=\"0\"/>\n")
	sb.WriteString("    <mxCell id=\"1\" parent=\"0\"/>\n")

	// Build adjacency for topological layering.
	// Edges go source → target (source depends on target).
	outgoing := make(map[string][]string)
	nodeByID := make(map[string]*model.Node)
	for _, n := range vg.Nodes {
		nodeByID[n.ID] = n
	}
	for _, e := range vg.Edges {
		if _, ok := nodeByID[e.Source]; !ok {
			continue
		}
		if _, ok := nodeByID[e.Target]; !ok {
			continue
		}
		outgoing[e.Source] = append(outgoing[e.Source], e.Target)
	}

	// Compute depth = longest path through outgoing edges.
	// Leaf nodes (no outgoing) get depth 0; roots get highest depth.
	depth := make(map[string]int)
	computing := make(map[string]bool)

	var computeDepth func(string) int
	computeDepth = func(id string) int {
		if d, ok := depth[id]; ok {
			return d
		}
		if computing[id] {
			return 0
		}
		computing[id] = true
		maxChild := -1
		for _, t := range outgoing[id] {
			if cd := computeDepth(t); cd > maxChild {
				maxChild = cd
			}
		}
		d := maxChild + 1
		depth[id] = d
		delete(computing, id)
		return d
	}

	for _, n := range vg.Nodes {
		computeDepth(n.ID)
	}

	// Group nodes by depth layer.
	maxDepth := 0
	for _, d := range depth {
		if d > maxDepth {
			maxDepth = d
		}
	}

	layerNodes := make([][]*model.Node, maxDepth+1)
	for _, n := range vg.Nodes {
		layerNodes[depth[n.ID]] = append(layerNodes[depth[n.ID]], n)
	}

	BarycenterOrder(layerNodes, vg.Edges)

	// Find widest layer for centering.
	maxCount := 0
	for _, nodes := range layerNodes {
		if len(nodes) > maxCount {
			maxCount = len(nodes)
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}
	maxWidth := maxCount*drawIOCellW + (maxCount-1)*drawIOGapX

	// Track node positions and group bounds by NodeType.
	type bounds struct {
		minX, minY, maxX, maxY int
		label                  string
	}
	groupBounds := make(map[model.NodeType]*bounds)
	groupPad := 16

	// Position: highest depth at top (row 0), depth 0 at bottom.
	for d := maxDepth; d >= 0; d-- {
		nodes := layerNodes[d]
		row := maxDepth - d
		layerWidth := len(nodes)*drawIOCellW + (len(nodes)-1)*drawIOGapX
		offsetX := (maxWidth - layerWidth) / 2

		for j, n := range nodes {
			id := SanitizeID(n.ID)
			style := drawIOStyle(n.Type)
			x := drawIOMarginX + offsetX + j*(drawIOCellW+drawIOGapX)
			y := drawIOMarginY + row*(drawIOCellH+drawIOGapY)

			fmt.Fprintf(&sb, "    <mxCell id=\"%s\" value=\"%s\" style=\"%s\" vertex=\"1\" parent=\"1\">\n",
				id, xmlEscape(n.Name), style)
			fmt.Fprintf(&sb, "      <mxGeometry x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" as=\"geometry\"/>\n",
				x, y, drawIOCellW, drawIOCellH)
			sb.WriteString("    </mxCell>\n")

			// Expand group bounds for this node type.
			b := groupBounds[n.Type]
			if b == nil {
				b = &bounds{minX: x, minY: y, maxX: x + drawIOCellW, maxY: y + drawIOCellH, label: drawIOGroupLabel(n.Type)}
				groupBounds[n.Type] = b
			} else {
				if x < b.minX {
					b.minX = x
				}
				if y < b.minY {
					b.minY = y
				}
				if x+drawIOCellW > b.maxX {
					b.maxX = x + drawIOCellW
				}
				if y+drawIOCellH > b.maxY {
					b.maxY = y + drawIOCellH
				}
			}
		}
	}

	// Render group frames behind nodes.
	for nt, b := range groupBounds {
		gx := b.minX - groupPad
		gy := b.minY - groupPad - 20 // extra space for label
		gw := b.maxX - b.minX + 2*groupPad
		gh := b.maxY - b.minY + 2*groupPad + 20
		gID := fmt.Sprintf("group_%s", SanitizeID(string(nt)))
		color := drawIOGroupColor(nt)
		fmt.Fprintf(&sb, "    <mxCell id=\"%s\" value=\"%s\" style=\"rounded=1;whiteSpace=wrap;html=1;fillColor=%s;strokeColor=%s;opacity=30;verticalAlign=top;fontStyle=1;fontSize=12;\" vertex=\"1\" parent=\"1\">\n",
			gID, xmlEscape(b.label), color, color)
		fmt.Fprintf(&sb, "      <mxGeometry x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" as=\"geometry\"/>\n",
			gx, gy, gw, gh)
		sb.WriteString("    </mxCell>\n")
	}

	// Render edges with curved routing to reduce label overlap.
	for i, e := range vg.Edges {
		label := EdgeLabel(e, vg.Names[e.Target])
		edgeID := fmt.Sprintf("edge_%d", i)
		fmt.Fprintf(&sb, "    <mxCell id=\"%s\" value=\"%s\" style=\"curved=1;endArrow=blockThin;endFill=1;fontSize=11;\" edge=\"1\" source=\"%s\" target=\"%s\" parent=\"1\">\n",
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

func drawIOGroupLabel(t model.NodeType) string {
	switch t {
	case model.NodeService:
		return "Services"
	case model.NodeDatabase:
		return "Data Stores"
	case model.NodeQueue:
		return "Queues"
	case model.NodeCache:
		return "Caches"
	case model.NodeExternalAPI:
		return "External"
	case model.NodeEndpoint:
		return "Endpoints"
	case model.NodePackage:
		return "Packages"
	case model.NodeModule:
		return "Modules"
	default:
		return "Components"
	}
}

func drawIOGroupColor(t model.NodeType) string {
	switch t {
	case model.NodeDatabase:
		return "#82b366"
	case model.NodeQueue:
		return "#d6b656"
	case model.NodeCache:
		return "#b85450"
	case model.NodeExternalAPI:
		return "#9673a6"
	case model.NodeEndpoint:
		return "#666666"
	default:
		return "#6c8ebf"
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
