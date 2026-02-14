package render

import (
	"encoding/json"
	"fmt"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

const (
	excaliCellW   = 200
	excaliCellH   = 60
	excaliGapX    = 60
	excaliGapY    = 120
	excaliMarginX = 40
	excaliMarginY = 40
)

// excalidrawFile is the top-level Excalidraw JSON structure.
type excalidrawFile struct {
	Type     string              `json:"type"`
	Version  int                 `json:"version"`
	Source   string              `json:"source"`
	Elements []excalidrawElement `json:"elements"`
	AppState map[string]any      `json:"appState"`
	Files    map[string]any      `json:"files"`
}

// excalidrawElement represents an Excalidraw element (rectangle, text, arrow).
type excalidrawElement struct {
	ID              string             `json:"id"`
	Type            string             `json:"type"`
	X               int                `json:"x"`
	Y               int                `json:"y"`
	Width           int                `json:"width"`
	Height          int                `json:"height"`
	Text            string             `json:"text,omitempty"`
	FontSize        int                `json:"fontSize,omitempty"`
	StrokeColor     string             `json:"strokeColor"`
	BackgroundColor string             `json:"backgroundColor"`
	FillStyle       string             `json:"fillStyle"`
	RoundRound      int                `json:"roundness,omitempty"`
	StartBinding    *excalidrawBinding `json:"startBinding,omitempty"`
	EndBinding      *excalidrawBinding `json:"endBinding,omitempty"`
	Points          [][2]int           `json:"points,omitempty"`
	ContainerID     string             `json:"containerId,omitempty"`
}

type excalidrawBinding struct {
	ElementID string  `json:"elementId"`
	Focus     float64 `json:"focus"`
	Gap       int     `json:"gap"`
}

// Excalidraw renders an ArchGraph as Excalidraw JSON.
// Nodes are arranged in topological layers: root packages at the top,
// leaf dependencies at the bottom.
func Excalidraw(graph *model.ArchGraph, opts Options) string {
	vg := FilterGraph(graph, opts.ViewLevel)
	nodePositions := make(map[string][2]int) // id -> [x, y]

	elements := make([]excalidrawElement, 0)

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

			elements = append(elements, excalidrawElement{
				ID:              rectID,
				Type:            "rectangle",
				X:               x,
				Y:               y,
				Width:           excaliCellW,
				Height:          excaliCellH,
				StrokeColor:     "#1e1e1e",
				BackgroundColor: bgColor,
				FillStyle:       "solid",
			})

			elements = append(elements, excalidrawElement{
				ID:              textID,
				Type:            "text",
				X:               x + 10,
				Y:               y + 20,
				Width:           excaliCellW - 20,
				Height:          25,
				Text:            n.Name,
				FontSize:        16,
				StrokeColor:     "#1e1e1e",
				BackgroundColor: "transparent",
				FillStyle:       "solid",
				ContainerID:     rectID,
			})
		}
	}

	// Create arrow elements for edges
	for i, e := range vg.Edges {
		srcPos := nodePositions[e.Source]
		tgtPos := nodePositions[e.Target]
		arrowID := fmt.Sprintf("arrow_%d", i)

		// Arrow points relative to start position
		dx := tgtPos[0] - srcPos[0]
		dy := tgtPos[1] - srcPos[1]

		elements = append(elements, excalidrawElement{
			ID:              arrowID,
			Type:            "arrow",
			X:               srcPos[0] + excaliCellW/2,
			Y:               srcPos[1] + excaliCellH/2,
			Width:           dx,
			Height:          dy,
			StrokeColor:     "#1e1e1e",
			BackgroundColor: "transparent",
			FillStyle:       "solid",
			Points:          [][2]int{{0, 0}, {dx, dy}},
			StartBinding: &excalidrawBinding{
				ElementID: SanitizeID(e.Source),
				Focus:     0,
				Gap:       8,
			},
			EndBinding: &excalidrawBinding{
				ElementID: SanitizeID(e.Target),
				Focus:     0,
				Gap:       8,
			},
		})
	}

	file := excalidrawFile{
		Type:     "excalidraw",
		Version:  2,
		Source:   "code-to-arch-mcp",
		Elements: elements,
		AppState: map[string]any{"viewBackgroundColor": "#ffffff"},
		Files:    map[string]any{},
	}

	data, _ := json.MarshalIndent(file, "", "  ")
	return string(data)
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
