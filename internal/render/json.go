package render

import (
	"encoding/json"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// jsonOutput is the structured output for JSON rendering.
type jsonOutput struct {
	Title     string        `json:"title"`
	ViewLevel string        `json:"view_level"`
	RootPath  string        `json:"root_path"`
	Topology  string        `json:"topology"`
	Nodes     []*model.Node `json:"nodes"`
	Edges     []*model.Edge `json:"edges"`
}

// JSON renders an ArchGraph as structured JSON.
func JSON(graph *model.ArchGraph, opts Options) string {
	title := opts.Title
	if title == "" {
		title = "Architecture"
	}

	nodes := FilterNodesByViewLevel(graph.Nodes(), opts.ViewLevel)
	visibleIDs := make(map[string]bool)
	for _, n := range nodes {
		visibleIDs[n.ID] = true
	}

	// Filter edges to only include those between visible nodes
	var edges []*model.Edge
	for _, e := range graph.Edges() {
		if visibleIDs[e.Source] && visibleIDs[e.Target] {
			edges = append(edges, e)
		}
	}

	out := jsonOutput{
		Title:     title,
		ViewLevel: string(opts.ViewLevel),
		RootPath:  graph.RootPath,
		Topology:  string(graph.Topology),
		Nodes:     nodes,
		Edges:     edges,
	}

	// Ensure empty slices serialize as [] not null
	if out.Nodes == nil {
		out.Nodes = []*model.Node{}
	}
	if out.Edges == nil {
		out.Edges = []*model.Edge{}
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data)
}
