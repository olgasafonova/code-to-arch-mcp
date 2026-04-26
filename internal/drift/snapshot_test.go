package drift

import (
	"path/filepath"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
)

func TestSaveAndLoad(t *testing.T) {
	g := model.NewGraph("/tmp/project")
	g.Topology = model.TopologyMonolith
	g.AddNode(&model.Node{ID: "svc:api", Name: "API", Type: model.NodeService, Language: "go"})
	g.AddNode(&model.Node{ID: "db:pg", Name: "PostgreSQL", Type: model.NodeDatabase})
	g.AddEdge(&model.Edge{Source: "svc:api", Target: "db:pg", Type: model.EdgeReadWrite, Label: "queries"})

	outFile := filepath.Join(t.TempDir(), "arch.snapshot.json")
	snap, err := Save(g, outFile, "v1.0")
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if snap.Label != "v1.0" {
		t.Fatalf("expected label v1.0, got %s", snap.Label)
	}

	loaded, err := Load(outFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Label != "v1.0" {
		t.Fatalf("expected label v1.0, got %s", loaded.Label)
	}
	if len(loaded.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(loaded.Nodes))
	}
	if len(loaded.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(loaded.Edges))
	}
}

func TestToGraph(t *testing.T) {
	snap := &Snapshot{
		RootPath: "/tmp",
		Topology: "monolith",
		Nodes: []*model.Node{
			{ID: "a", Name: "A", Type: model.NodeService},
			{ID: "b", Name: "B", Type: model.NodeDatabase},
		},
		Edges: []*model.Edge{
			{Source: "a", Target: "b", Type: model.EdgeReadWrite},
		},
	}

	g := snap.ToGraph()
	if g.NodeCount() != 2 {
		t.Fatalf("expected 2 nodes, got %d", g.NodeCount())
	}
	if g.EdgeCount() != 1 {
		t.Fatalf("expected 1 edge, got %d", g.EdgeCount())
	}
	if g.Topology != model.TopologyMonolith {
		t.Fatalf("expected monolith, got %s", g.Topology)
	}
}

func TestLoadNonexistent(t *testing.T) {
	_, err := Load("/nonexistent/snapshot.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
