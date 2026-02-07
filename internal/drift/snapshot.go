// Package drift provides architecture drift detection: snapshot, compare, and report.
package drift

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Snapshot is a serializable baseline of an architecture graph.
type Snapshot struct {
	Version   string        `json:"version"`
	Label     string        `json:"label,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	RootPath  string        `json:"root_path"`
	Topology  string        `json:"topology"`
	Nodes     []*model.Node `json:"nodes"`
	Edges     []*model.Edge `json:"edges"`
}

// Save serializes a snapshot to a JSON file.
func Save(graph *model.ArchGraph, outputFile, label string) (*Snapshot, error) {
	snap := &Snapshot{
		Version:   "1",
		Label:     label,
		CreatedAt: time.Now(),
		RootPath:  graph.RootPath,
		Topology:  string(graph.Topology),
		Nodes:     graph.Nodes(),
		Edges:     graph.Edges(),
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serializing snapshot: %w", err)
	}

	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return nil, fmt.Errorf("writing snapshot file: %w", err)
	}

	return snap, nil
}

// Load reads a snapshot from a JSON file.
func Load(file string) (*Snapshot, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot file: %w", err)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing snapshot: %w", err)
	}

	return &snap, nil
}

// ToGraph reconstructs an ArchGraph from a snapshot.
func (s *Snapshot) ToGraph() *model.ArchGraph {
	g := model.NewGraph(s.RootPath)
	g.Topology = model.TopologyType(s.Topology)
	for _, n := range s.Nodes {
		g.AddNode(n)
	}
	for _, e := range s.Edges {
		g.AddEdge(e)
	}
	return g
}
