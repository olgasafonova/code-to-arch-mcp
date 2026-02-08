package render

import (
	"encoding/json"
	"fmt"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

const (
	excaliCellWidth  = 200
	excaliCellHeight = 60
	excaliGapX       = 60
	excaliGapY       = 100
	excaliColsPerRow = 4
	excaliMarginX    = 40
	excaliMarginY    = 40
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
func Excalidraw(graph *model.ArchGraph, opts Options) string {
	nodes := FilterNodesByViewLevel(graph.Nodes(), opts.ViewLevel)
	visibleIDs := make(map[string]bool)
	nodePositions := make(map[string][2]int) // id -> [x, y]

	for _, n := range nodes {
		visibleIDs[n.ID] = true
	}

	elements := make([]excalidrawElement, 0)

	// Create rectangle + text elements for each node
	for i, n := range nodes {
		col := i % excaliColsPerRow
		row := i / excaliColsPerRow
		x := excaliMarginX + col*(excaliCellWidth+excaliGapX)
		y := excaliMarginY + row*(excaliCellHeight+excaliGapY)

		rectID := SanitizeID(n.ID)
		textID := rectID + "_text"
		nodePositions[n.ID] = [2]int{x, y}

		bgColor := excalidrawBgColor(n.Type)

		elements = append(elements, excalidrawElement{
			ID:              rectID,
			Type:            "rectangle",
			X:               x,
			Y:               y,
			Width:           excaliCellWidth,
			Height:          excaliCellHeight,
			StrokeColor:     "#1e1e1e",
			BackgroundColor: bgColor,
			FillStyle:       "solid",
		})

		elements = append(elements, excalidrawElement{
			ID:              textID,
			Type:            "text",
			X:               x + 10,
			Y:               y + 20,
			Width:           excaliCellWidth - 20,
			Height:          25,
			Text:            n.Name,
			FontSize:        16,
			StrokeColor:     "#1e1e1e",
			BackgroundColor: "transparent",
			FillStyle:       "solid",
			ContainerID:     rectID,
		})
	}

	// Create arrow elements for edges
	for i, e := range graph.Edges() {
		if !visibleIDs[e.Source] || !visibleIDs[e.Target] {
			continue
		}

		srcPos := nodePositions[e.Source]
		tgtPos := nodePositions[e.Target]
		arrowID := fmt.Sprintf("arrow_%d", i)

		// Arrow points relative to start position
		dx := tgtPos[0] - srcPos[0]
		dy := tgtPos[1] - srcPos[1]

		elements = append(elements, excalidrawElement{
			ID:              arrowID,
			Type:            "arrow",
			X:               srcPos[0] + excaliCellWidth/2,
			Y:               srcPos[1] + excaliCellHeight/2,
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
