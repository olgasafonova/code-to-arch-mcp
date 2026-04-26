package render

import (
	"encoding/json"

	"github.com/olgasafonova/ridge/internal/model"
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

	vg := PrepareGraph(graph, opts)

	out := jsonOutput{
		Title:     title,
		ViewLevel: string(opts.ViewLevel),
		RootPath:  graph.RootPath,
		Topology:  string(graph.Topology),
		Nodes:     vg.Nodes,
		Edges:     vg.Edges,
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
