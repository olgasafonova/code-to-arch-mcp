package render

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

const (
	excaliCellW   = 200
	excaliCellH   = 60
	excaliGapX    = 100
	excaliGapY    = 300
	excaliMarginX = 60
	excaliMarginY = 60
)

// Excalidraw renders an ArchGraph as Excalidraw JSON.
// Nodes are arranged in topological layers: root packages at the top,
// leaf dependencies at the bottom. Arrows use orthogonal routing.
func Excalidraw(graph *model.ArchGraph, opts Options) string {
	vg := PrepareGraph(graph, opts)
	vg.TransitiveReduce()
	nodePositions := make(map[string][2]int) // id -> [x, y]

	elements := make([]map[string]any, 0)
	seed := 1000000

	nextSeed := func() int {
		seed += 7
		return seed
	}

	// Build adjacency for topological layering.
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

	maxCount := 0
	for _, nodes := range layerNodes {
		if len(nodes) > maxCount {
			maxCount = len(nodes)
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}
	maxWidth := maxCount*excaliCellW + (maxCount-1)*excaliGapX

	// Track group bounds by NodeType for visual clustering.
	type bounds struct {
		minX, minY, maxX, maxY int
	}
	groupBounds := make(map[model.NodeType]*bounds)
	groupPad := 16

	// Position: highest depth at top (row 0), depth 0 at bottom.
	for d := maxDepth; d >= 0; d-- {
		nodes := layerNodes[d]
		row := maxDepth - d
		layerWidth := len(nodes)*excaliCellW + (len(nodes)-1)*excaliGapX
		offsetX := (maxWidth - layerWidth) / 2

		for j, n := range nodes {
			x := excaliMarginX + offsetX + j*(excaliCellW+excaliGapX)
			y := excaliMarginY + row*(excaliCellH+excaliGapY)

			rectID := SanitizeID(n.ID)
			textID := rectID + "_text"
			nodePositions[n.ID] = [2]int{x, y}

			bgColor := excalidrawBgColor(n.Type)

			elements = append(elements, excaliRect(rectID, x, y, excaliCellW, excaliCellH, bgColor, nextSeed()))
			elements = append(elements, excaliText(textID, x+10, y+20, excaliCellW-20, 25, n.Name, 16, &rectID, nextSeed()))

			// Expand group bounds.
			b := groupBounds[n.Type]
			if b == nil {
				b = &bounds{minX: x, minY: y, maxX: x + excaliCellW, maxY: y + excaliCellH}
				groupBounds[n.Type] = b
			} else {
				if x < b.minX {
					b.minX = x
				}
				if y < b.minY {
					b.minY = y
				}
				if x+excaliCellW > b.maxX {
					b.maxX = x + excaliCellW
				}
				if y+excaliCellH > b.maxY {
					b.maxY = y + excaliCellH
				}
			}
		}
	}

	// Add group frame elements (rendered behind nodes via order).
	var groupElements []map[string]any
	for nt, b := range groupBounds {
		gx := b.minX - groupPad
		gy := b.minY - groupPad - 24
		gw := b.maxX - b.minX + 2*groupPad
		gh := b.maxY - b.minY + 2*groupPad + 24

		frameID := fmt.Sprintf("group_%s", SanitizeID(string(nt)))
		labelID := frameID + "_label"

		frame := excaliRect(frameID, gx, gy, gw, gh, excalidrawBgColor(nt), nextSeed())
		frame["opacity"] = 20
		frame["strokeColor"] = excalidrawBgColor(nt)
		groupElements = append(groupElements, frame)

		groupElements = append(groupElements, excaliText(labelID, gx+8, gy+4, gw-16, 20, drawIOGroupLabel(nt), 12, nil, nextSeed()))
	}
	if len(groupElements) > 0 {
		elements = append(groupElements, elements...)
	}

	// Port distribution: sort outgoing edges by target X and incoming
	// edges by source X, then spread attachment points across node width.
	type portAssign struct {
		startFrac, endFrac float64
		outRank, outTotal  int
	}
	portMap := make(map[int]portAssign)
	edgesBySource := make(map[string][]int)
	edgesByTarget := make(map[string][]int)
	for i, e := range vg.Edges {
		edgesBySource[e.Source] = append(edgesBySource[e.Source], i)
		edgesByTarget[e.Target] = append(edgesByTarget[e.Target], i)
	}

	for _, indices := range edgesBySource {
		sort.Slice(indices, func(a, b int) bool {
			return nodePositions[vg.Edges[indices[a]].Target][0] <
				nodePositions[vg.Edges[indices[b]].Target][0]
		})
		n := len(indices)
		for rank, idx := range indices {
			frac := 0.5
			if n > 1 {
				frac = 0.15 + 0.7*float64(rank)/float64(n-1)
			}
			pa := portMap[idx]
			pa.startFrac = frac
			pa.outRank = rank
			pa.outTotal = n
			portMap[idx] = pa
		}
	}

	for _, indices := range edgesByTarget {
		sort.Slice(indices, func(a, b int) bool {
			return nodePositions[vg.Edges[indices[a]].Source][0] <
				nodePositions[vg.Edges[indices[b]].Source][0]
		})
		n := len(indices)
		for rank, idx := range indices {
			frac := 0.5
			if n > 1 {
				frac = 0.15 + 0.7*float64(rank)/float64(n-1)
			}
			pa := portMap[idx]
			pa.endFrac = frac
			portMap[idx] = pa
		}
	}

	// Arrows with orthogonal routing and all required Excalidraw properties.
	for i, e := range vg.Edges {
		srcPos := nodePositions[e.Source]
		tgtPos := nodePositions[e.Target]
		pa := portMap[i]

		startX := float64(srcPos[0]) + pa.startFrac*float64(excaliCellW)
		startY := float64(srcPos[1] + excaliCellH)
		endX := float64(tgtPos[0]) + pa.endFrac*float64(excaliCellW)
		endY := float64(tgtPos[1])

		dx := endX - startX
		dy := endY - startY
		arrowID := fmt.Sprintf("arrow_%d", i)

		var points [][2]float64
		absDx := dx
		if absDx < 0 {
			absDx = -absDx
		}
		if absDx < 10 {
			points = [][2]float64{{0, 0}, {0, dy}}
		} else {
			channelDY := 30.0
			if pa.outTotal > 1 {
				channelDY = 30 + float64(excaliGapY-60)*float64(pa.outRank)/float64(pa.outTotal-1)
			}
			points = [][2]float64{
				{0, 0},
				{0, channelDY},
				{dx, channelDY},
				{dx, dy},
			}
		}

		elements = append(elements, excaliArrow(arrowID, startX, startY, points, nextSeed()))
	}

	file := map[string]any{
		"type":     "excalidraw",
		"version":  2,
		"source":   "code-to-arch-mcp",
		"elements": elements,
		"appState": map[string]any{"viewBackgroundColor": "#ffffff"},
		"files":    map[string]any{},
	}

	data, _ := json.MarshalIndent(file, "", "  ")
	return string(data)
}

// excaliRect creates a complete Excalidraw rectangle element.
func excaliRect(id string, x, y, w, h int, bgColor string, seed int) map[string]any {
	return map[string]any{
		"id":              id,
		"type":            "rectangle",
		"x":               x,
		"y":               y,
		"width":           w,
		"height":          h,
		"angle":           0,
		"strokeColor":     "#1e1e1e",
		"backgroundColor": bgColor,
		"fillStyle":       "solid",
		"strokeWidth":     2,
		"strokeStyle":     "solid",
		"roughness":       1,
		"opacity":         100,
		"groupIds":        []string{},
		"frameId":         nil,
		"roundness":       map[string]int{"type": 3},
		"seed":            seed,
		"version":         1,
		"versionNonce":    seed + 1,
		"isDeleted":       false,
		"boundElements":   nil,
		"updated":         1708000000000,
		"link":            nil,
		"locked":          false,
	}
}

// excaliText creates a complete Excalidraw text element.
func excaliText(id string, x, y, w, h int, text string, fontSize int, containerID *string, seed int) map[string]any {
	return map[string]any{
		"id":              id,
		"type":            "text",
		"x":               x,
		"y":               y,
		"width":           w,
		"height":          h,
		"angle":           0,
		"strokeColor":     "#1e1e1e",
		"backgroundColor": "transparent",
		"fillStyle":       "solid",
		"strokeWidth":     2,
		"strokeStyle":     "solid",
		"roughness":       1,
		"opacity":         100,
		"groupIds":        []string{},
		"frameId":         nil,
		"roundness":       nil,
		"seed":            seed,
		"version":         1,
		"versionNonce":    seed + 1,
		"isDeleted":       false,
		"boundElements":   nil,
		"updated":         1708000000000,
		"link":            nil,
		"locked":          false,
		"text":            text,
		"fontSize":        fontSize,
		"fontFamily":      1,
		"textAlign":       "center",
		"verticalAlign":   "middle",
		"containerId":     containerID,
		"originalText":    text,
		"lineHeight":      1.25,
	}
}

// excaliArrow creates a complete Excalidraw arrow element with all
// required properties so Excalidraw doesn't re-normalize the path.
func excaliArrow(id string, x, y float64, points [][2]float64, seed int) map[string]any {
	// Bounding box from points.
	var minX, maxX, minY, maxY float64
	for _, p := range points {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}

	return map[string]any{
		"id":                 id,
		"type":               "arrow",
		"x":                  x,
		"y":                  y,
		"width":              maxX - minX,
		"height":             maxY - minY,
		"angle":              0,
		"strokeColor":        "#868e96",
		"backgroundColor":    "transparent",
		"fillStyle":          "solid",
		"strokeWidth":        1,
		"strokeStyle":        "solid",
		"roughness":          0,
		"opacity":            100,
		"groupIds":           []string{},
		"frameId":            nil,
		"roundness":          map[string]int{"type": 2},
		"seed":               seed,
		"version":            1,
		"versionNonce":       seed + 1,
		"isDeleted":          false,
		"boundElements":      nil,
		"updated":            1708000000000,
		"link":               nil,
		"locked":             false,
		"points":             points,
		"lastCommittedPoint": nil,
		"startBinding":       nil,
		"endBinding":         nil,
		"startArrowhead":     nil,
		"endArrowhead":       "arrow",
	}
}

func excalidrawBgColor(t model.NodeType) string {
	switch t {
	case model.NodeDatabase:
		return "#d5e8d4"
	case model.NodeQueue:
		return "#fff2cc"
	case model.NodeCache:
		return "#f8cecc"
	case model.NodeExternalAPI:
		return "#e1d5e7"
	case model.NodeEndpoint:
		return "#f5f5f5"
	default:
		return "#dae8fc"
	}
}
