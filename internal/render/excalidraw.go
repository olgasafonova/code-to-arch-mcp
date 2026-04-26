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

// excaliPortAssign carries the start/end attachment fractions for one edge.
type excaliPortAssign struct {
	startFrac, endFrac float64
	outRank, outTotal  int
}

// excaliBounds tracks the rectangular bounds of a node-type group.
type excaliBounds struct {
	minX, minY, maxX, maxY int
}

// excaliSeed produces deterministic, monotonically-increasing seeds so the
// same graph renders identically across runs.
type excaliSeed struct{ v int }

func (s *excaliSeed) next() int {
	s.v += 7
	return s.v
}

const excaliGroupPad = 16

// Excalidraw renders an ArchGraph as Excalidraw JSON.
// Nodes are arranged in topological layers: root packages at the top,
// leaf dependencies at the bottom. Arrows use orthogonal routing.
func Excalidraw(graph *model.ArchGraph, opts Options) string {
	vg := PrepareGraph(graph, opts)
	vg.TransitiveReduce()

	depths, maxDepth := excaliComputeDepths(vg)
	layers := excaliGroupByDepth(vg, depths, maxDepth)
	BarycenterOrder(layers, vg.Edges)

	seed := &excaliSeed{v: 1000000}
	maxWidth := excaliLayoutWidth(layers)

	nodeElements, positions, groupBounds := excaliLayoutNodes(layers, maxDepth, maxWidth, seed)
	groupElements := excaliBuildGroupFrames(groupBounds, seed)

	portMap := excaliDistributePorts(vg, positions)
	arrowElements := excaliBuildArrows(vg, portMap, positions, seed)

	elements := make([]map[string]any, 0, len(groupElements)+len(nodeElements)+len(arrowElements))
	elements = append(elements, groupElements...)
	elements = append(elements, nodeElements...)
	elements = append(elements, arrowElements...)

	return excaliMarshal(elements)
}

// excaliComputeDepths returns each node's depth (longest outgoing path) and
// the overall maximum depth.
func excaliComputeDepths(vg *VisibleGraph) (map[string]int, int) {
	nodeIDs := make(map[string]bool, len(vg.Nodes))
	for _, n := range vg.Nodes {
		nodeIDs[n.ID] = true
	}

	outgoing := make(map[string][]string)
	for _, e := range vg.Edges {
		if !nodeIDs[e.Source] || !nodeIDs[e.Target] {
			continue
		}
		outgoing[e.Source] = append(outgoing[e.Source], e.Target)
	}

	depths := make(map[string]int, len(vg.Nodes))
	computing := make(map[string]bool)

	var compute func(string) int
	compute = func(id string) int {
		if d, ok := depths[id]; ok {
			return d
		}
		if computing[id] {
			return 0 // cycle: tie-break at depth 0
		}
		computing[id] = true
		maxChild := -1
		for _, t := range outgoing[id] {
			if cd := compute(t); cd > maxChild {
				maxChild = cd
			}
		}
		d := maxChild + 1
		depths[id] = d
		delete(computing, id)
		return d
	}

	maxDepth := 0
	for _, n := range vg.Nodes {
		if d := compute(n.ID); d > maxDepth {
			maxDepth = d
		}
	}
	return depths, maxDepth
}

// excaliGroupByDepth buckets nodes into depth layers (index 0 = leaves).
func excaliGroupByDepth(vg *VisibleGraph, depths map[string]int, maxDepth int) [][]*model.Node {
	layers := make([][]*model.Node, maxDepth+1)
	for _, n := range vg.Nodes {
		layers[depths[n.ID]] = append(layers[depths[n.ID]], n)
	}
	return layers
}

// excaliLayoutWidth returns the canvas width needed for the widest layer.
func excaliLayoutWidth(layers [][]*model.Node) int {
	maxCount := 0
	for _, nodes := range layers {
		if len(nodes) > maxCount {
			maxCount = len(nodes)
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}
	return maxCount*excaliCellW + (maxCount-1)*excaliGapX
}

// excaliLayoutNodes assigns a position to each node, emits its rect+text
// elements, and tracks the bounds of each NodeType group.
func excaliLayoutNodes(layers [][]*model.Node, maxDepth, maxWidth int, seed *excaliSeed) ([]map[string]any, map[string][2]int, map[model.NodeType]*excaliBounds) {
	elements := make([]map[string]any, 0)
	positions := make(map[string][2]int)
	groupBounds := make(map[model.NodeType]*excaliBounds)

	for d := maxDepth; d >= 0; d-- {
		nodes := layers[d]
		row := maxDepth - d
		layerWidth := len(nodes)*excaliCellW + (len(nodes)-1)*excaliGapX
		offsetX := (maxWidth - layerWidth) / 2

		for j, n := range nodes {
			x := excaliMarginX + offsetX + j*(excaliCellW+excaliGapX)
			y := excaliMarginY + row*(excaliCellH+excaliGapY)

			rectID := SanitizeID(n.ID)
			positions[n.ID] = [2]int{x, y}

			bgColor := excalidrawBgColor(n.Type)
			elements = append(elements, excaliRect(rectID, x, y, excaliCellW, excaliCellH, bgColor, seed.next()))
			elements = append(elements, excaliText(rectID+"_text", x+10, y+20, excaliCellW-20, 25, n.Name, 16, &rectID, seed.next()))

			expandGroupBounds(groupBounds, n.Type, x, y)
		}
	}
	return elements, positions, groupBounds
}

// expandGroupBounds widens (or initializes) the bounds for a NodeType group
// to enclose a cell at (x, y).
func expandGroupBounds(groupBounds map[model.NodeType]*excaliBounds, t model.NodeType, x, y int) {
	b := groupBounds[t]
	if b == nil {
		groupBounds[t] = &excaliBounds{minX: x, minY: y, maxX: x + excaliCellW, maxY: y + excaliCellH}
		return
	}
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

// excaliBuildGroupFrames emits a translucent backdrop rect + label for each
// NodeType cluster. Returned in iteration order (Excalidraw renders in the
// order it gets, so callers must prepend to keep frames behind nodes).
func excaliBuildGroupFrames(groupBounds map[model.NodeType]*excaliBounds, seed *excaliSeed) []map[string]any {
	if len(groupBounds) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, 2*len(groupBounds))
	for nt, b := range groupBounds {
		gx := b.minX - excaliGroupPad
		gy := b.minY - excaliGroupPad - 24
		gw := b.maxX - b.minX + 2*excaliGroupPad
		gh := b.maxY - b.minY + 2*excaliGroupPad + 24

		frameID := fmt.Sprintf("group_%s", SanitizeID(string(nt)))
		frame := excaliRect(frameID, gx, gy, gw, gh, excalidrawBgColor(nt), seed.next())
		frame["opacity"] = 20
		frame["strokeColor"] = excalidrawBgColor(nt)
		out = append(out, frame)
		out = append(out, excaliText(frameID+"_label", gx+8, gy+4, gw-16, 20, drawIOGroupLabel(nt), 12, nil, seed.next()))
	}
	return out
}

// excaliDistributePorts assigns start/end attachment fractions across each
// node's width so multiple edges sharing a source or target are visually
// separated rather than overlapping at the cell midpoint.
func excaliDistributePorts(vg *VisibleGraph, positions map[string][2]int) map[int]excaliPortAssign {
	portMap := make(map[int]excaliPortAssign, len(vg.Edges))

	edgesBySource := make(map[string][]int)
	edgesByTarget := make(map[string][]int)
	for i, e := range vg.Edges {
		edgesBySource[e.Source] = append(edgesBySource[e.Source], i)
		edgesByTarget[e.Target] = append(edgesByTarget[e.Target], i)
	}

	excaliAssignFractions(portMap, edgesBySource, vg, positions, true)
	excaliAssignFractions(portMap, edgesByTarget, vg, positions, false)
	return portMap
}

// excaliAssignFractions sorts edges sharing a source (or target) by the
// position of the opposite endpoint, then spreads start (or end) fractions
// across [0.15, 0.85].
func excaliAssignFractions(portMap map[int]excaliPortAssign, groups map[string][]int, vg *VisibleGraph, positions map[string][2]int, isSource bool) {
	for _, indices := range groups {
		sort.Slice(indices, func(a, b int) bool {
			otherA := vg.Edges[indices[a]].Target
			otherB := vg.Edges[indices[b]].Target
			if !isSource {
				otherA = vg.Edges[indices[a]].Source
				otherB = vg.Edges[indices[b]].Source
			}
			return positions[otherA][0] < positions[otherB][0]
		})
		n := len(indices)
		for rank, idx := range indices {
			frac := 0.5
			if n > 1 {
				frac = 0.15 + 0.7*float64(rank)/float64(n-1)
			}
			pa := portMap[idx]
			if isSource {
				pa.startFrac = frac
				pa.outRank = rank
				pa.outTotal = n
			} else {
				pa.endFrac = frac
			}
			portMap[idx] = pa
		}
	}
}

// excaliBuildArrows produces the arrow elements with orthogonal routing.
func excaliBuildArrows(vg *VisibleGraph, portMap map[int]excaliPortAssign, positions map[string][2]int, seed *excaliSeed) []map[string]any {
	out := make([]map[string]any, 0, len(vg.Edges))
	for i, e := range vg.Edges {
		srcPos := positions[e.Source]
		tgtPos := positions[e.Target]
		pa := portMap[i]

		startX := float64(srcPos[0]) + pa.startFrac*float64(excaliCellW)
		startY := float64(srcPos[1] + excaliCellH)
		endX := float64(tgtPos[0]) + pa.endFrac*float64(excaliCellW)
		endY := float64(tgtPos[1])

		out = append(out, excaliArrow(fmt.Sprintf("arrow_%d", i), startX, startY, excaliRoutePoints(endX-startX, endY-startY, pa), seed.next()))
	}
	return out
}

// excaliRoutePoints returns the orthogonal-routing waypoints for an arrow.
// Short horizontal deltas use a single straight segment; longer ones detour
// through a fan-out channel keyed off the source's outgoing rank.
func excaliRoutePoints(dx, dy float64, pa excaliPortAssign) [][2]float64 {
	absDx := dx
	if absDx < 0 {
		absDx = -absDx
	}
	if absDx < 10 {
		return [][2]float64{{0, 0}, {0, dy}}
	}
	channelDY := 30.0
	if pa.outTotal > 1 {
		channelDY = 30 + float64(excaliGapY-60)*float64(pa.outRank)/float64(pa.outTotal-1)
	}
	return [][2]float64{
		{0, 0},
		{0, channelDY},
		{dx, channelDY},
		{dx, dy},
	}
}

// excaliMarshal wraps a complete element list in the Excalidraw envelope.
func excaliMarshal(elements []map[string]any) string {
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
