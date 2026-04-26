package detector

import (
	"math"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
)

// buildTestGraph creates a graph that mirrors real scanner output:
//
//	main → tools → golang → common → model
//
// Edges use "import:" prefixed targets (as produced by analyzers),
// not direct pkg node IDs.
func buildTestGraph() *model.ArchGraph {
	g := model.NewGraph("/projects/myapp")

	g.AddNode(&model.Node{ID: "pkg:main/main", Name: "main", Type: model.NodePackage, Path: "cmd/main"})
	g.AddNode(&model.Node{ID: "pkg:tools/tools", Name: "tools", Type: model.NodePackage, Path: "tools"})
	g.AddNode(&model.Node{ID: "pkg:golang/golang", Name: "golang", Type: model.NodePackage, Path: "internal/analyzer/golang"})
	g.AddNode(&model.Node{ID: "pkg:common/common", Name: "common", Type: model.NodePackage, Path: "internal/analyzer/common"})
	g.AddNode(&model.Node{ID: "pkg:model/model", Name: "model", Type: model.NodePackage, Path: "internal/model"})

	// Edges use import: targets, matching real analyzer output.
	g.AddEdge(&model.Edge{Source: "pkg:main/main", Target: "import:github.com/user/myapp/tools", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:tools/tools", Target: "import:github.com/user/myapp/internal/analyzer/golang", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:tools/tools", Target: "import:github.com/user/myapp/internal/model", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:golang/golang", Target: "import:github.com/user/myapp/internal/analyzer/common", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:golang/golang", Target: "import:github.com/user/myapp/internal/model", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:common/common", Target: "import:github.com/user/myapp/internal/model", Type: model.EdgeDependency})

	// Stdlib imports that should not resolve to any node.
	g.AddEdge(&model.Edge{Source: "pkg:main/main", Target: "import:fmt", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "pkg:model/model", Target: "import:strings", Type: model.EdgeDependency})

	return g
}

func TestInstability_ModelIsStable(t *testing.T) {
	g := buildTestGraph()
	m := ComputeMetrics(g)

	// model has 3 incoming deps (tools, golang, common) and 0 outgoing internal deps.
	// Instability = 0 / (3+0) = 0.0
	inst, ok := m.Instability["pkg:model/model"]
	if !ok {
		t.Fatal("model missing from instability map")
	}
	if inst != 0.0 {
		t.Errorf("model instability = %v, want 0.0", inst)
	}
}

func TestInstability_MainIsUnstable(t *testing.T) {
	g := buildTestGraph()
	m := ComputeMetrics(g)

	// main has 1 outgoing dep (tools) and 0 incoming → instability = 1.0
	inst, ok := m.Instability["pkg:main/main"]
	if !ok {
		t.Fatal("main missing from instability map")
	}
	if inst != 1.0 {
		t.Errorf("main instability = %v, want 1.0", inst)
	}
}

func TestInstability_ToolsIsMixed(t *testing.T) {
	g := buildTestGraph()
	m := ComputeMetrics(g)

	// tools: 2 outgoing (golang, model), 1 incoming (main)
	// Instability = 2 / (1+2) ≈ 0.667
	inst, ok := m.Instability["pkg:tools/tools"]
	if !ok {
		t.Fatal("tools missing from instability map")
	}
	want := 2.0 / 3.0
	if math.Abs(inst-want) > 0.001 {
		t.Errorf("tools instability = %v, want ~%v", inst, want)
	}
}

func TestMaxDepth_ChainIsTraversed(t *testing.T) {
	g := buildTestGraph()
	m := ComputeMetrics(g)

	// Chain: main → tools → golang → common → model = depth 4
	if m.MaxDepth != 4 {
		t.Errorf("MaxDepth = %d, want 4", m.MaxDepth)
	}
}

func TestCoupling_FanOutCounts(t *testing.T) {
	g := buildTestGraph()
	m := ComputeMetrics(g)

	cases := map[string]float64{
		"pkg:main/main":     1, // → tools
		"pkg:tools/tools":   2, // → golang, model
		"pkg:golang/golang": 2, // → common, model
		"pkg:common/common": 1, // → model
		"pkg:model/model":   0, // leaf
	}
	for id, want := range cases {
		got, ok := m.Coupling[id]
		if !ok {
			t.Errorf("%s missing from coupling map", id)
			continue
		}
		if got != want {
			t.Errorf("coupling[%s] = %v, want %v", id, got, want)
		}
	}
}

func TestEdgeCounts_OnlyResolvedEdges(t *testing.T) {
	g := buildTestGraph()
	m := ComputeMetrics(g)

	// 6 internal edges resolve; 2 stdlib edges drop.
	got := m.EdgeCounts["dependency"]
	if got != 6 {
		t.Errorf("EdgeCounts[dependency] = %d, want 6", got)
	}
}
