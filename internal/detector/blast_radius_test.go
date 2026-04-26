package detector

import (
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// buildBlastGraph constructs a small dependency graph for blast-radius tests.
//
//	cmd ─────► api ────► db
//	 │          │
//	 └──► utils ┘
//	            ▲
//	           cli
//
// "db" is the leaf; cmd, api, cli all directly or transitively depend on it.
func buildBlastGraph() *model.ArchGraph {
	g := model.NewGraph("/tmp/test")
	for _, n := range []*model.Node{
		{ID: "pkg:cmd", Name: "cmd", Type: model.NodePackage, Path: "cmd"},
		{ID: "pkg:cli", Name: "cli", Type: model.NodePackage, Path: "cli"},
		{ID: "pkg:api", Name: "api", Type: model.NodePackage, Path: "api"},
		{ID: "pkg:utils", Name: "utils", Type: model.NodePackage, Path: "internal/utils"},
		{ID: "pkg:db", Name: "db", Type: model.NodePackage, Path: "internal/db"},
	} {
		g.AddNode(n)
	}
	for _, e := range []*model.Edge{
		{Source: "pkg:cmd", Target: "pkg:api", Type: model.EdgeDependency},
		{Source: "pkg:cmd", Target: "pkg:utils", Type: model.EdgeDependency},
		{Source: "pkg:api", Target: "pkg:utils", Type: model.EdgeDependency},
		{Source: "pkg:api", Target: "pkg:db", Type: model.EdgeDependency},
		{Source: "pkg:cli", Target: "pkg:utils", Type: model.EdgeDependency},
	} {
		g.AddEdge(e)
	}
	return g
}

func TestResolveTargetToID_ExactMatch(t *testing.T) {
	g := buildBlastGraph()

	got, ok := ResolveTargetToID(g, "pkg:utils")
	if !ok || got != "pkg:utils" {
		t.Fatalf("exact ID match: got (%q, %v); want (\"pkg:utils\", true)", got, ok)
	}
}

func TestResolveTargetToID_PathSuffix(t *testing.T) {
	g := buildBlastGraph()

	got, ok := ResolveTargetToID(g, "internal/utils")
	if !ok || got != "pkg:utils" {
		t.Fatalf("path-suffix match: got (%q, %v); want (\"pkg:utils\", true)", got, ok)
	}
}

func TestResolveTargetToID_NotFound(t *testing.T) {
	g := buildBlastGraph()

	if _, ok := ResolveTargetToID(g, "nope"); ok {
		t.Fatal("expected miss for non-existent target")
	}
	if _, ok := ResolveTargetToID(g, ""); ok {
		t.Fatal("empty target should never match")
	}
}

func TestComputeBlastRadius_TransitiveClosure(t *testing.T) {
	g := buildBlastGraph()

	res := ComputeBlastRadius(g, "pkg:utils", 0)

	// utils is depended on by: cmd (direct), api (direct), cli (direct).
	// cmd also depends on api, but cmd already shows up at depth 1 via the
	// direct cmd→utils edge, so depth 1 wins (BFS records the shortest).
	if res.Direct != 3 {
		t.Errorf("direct dependents: got %d; want 3 (cmd, api, cli)", res.Direct)
	}
	if res.Total != 3 {
		t.Errorf("total dependents: got %d; want 3", res.Total)
	}
	if res.MaxDepthHit {
		t.Errorf("did not expect MaxDepthHit on a 4-node graph with default cap")
	}

	// Every direct dependent should have a path-back chain of length 2:
	// [self_id, target_id].
	for _, d := range res.Dependents {
		if d.Depth == 1 && len(d.PathBack) != 2 {
			t.Errorf("depth-1 entry %q PathBack length: got %d; want 2", d.NodeID, len(d.PathBack))
		}
		if d.PathBack[len(d.PathBack)-1] != "pkg:utils" {
			t.Errorf("PathBack should end at target; got %v", d.PathBack)
		}
	}
}

func TestComputeBlastRadius_MultiHop(t *testing.T) {
	g := buildBlastGraph()

	res := ComputeBlastRadius(g, "pkg:db", 0)

	// db is reached only via api. cmd reaches db transitively (cmd→api→db).
	// cli does NOT reach db (cli→utils only).
	got := map[string]int{}
	for _, d := range res.Dependents {
		got[d.NodeID] = d.Depth
	}
	want := map[string]int{
		"pkg:api": 1,
		"pkg:cmd": 2,
	}
	if len(got) != len(want) {
		t.Fatalf("dependent set: got %v; want %v", got, want)
	}
	for id, depth := range want {
		if got[id] != depth {
			t.Errorf("%s depth: got %d; want %d", id, got[id], depth)
		}
	}
	if _, hasCli := got["pkg:cli"]; hasCli {
		t.Error("cli should NOT appear in db's blast radius (no transitive path)")
	}
}

func TestComputeBlastRadius_DepthCap(t *testing.T) {
	g := buildBlastGraph()

	res := ComputeBlastRadius(g, "pkg:db", 1)

	// With max_depth=1, only direct dependents should appear (just api).
	if res.Total != 1 || res.Dependents[0].NodeID != "pkg:api" {
		t.Errorf("depth=1 cap: got %d dependents %v; want 1 (pkg:api)", res.Total, res.Dependents)
	}
	if !res.MaxDepthHit {
		t.Error("MaxDepthHit should be true when there are unexpanded dependents at the cap")
	}
}

func TestComputeBlastRadius_TargetWithNoDependents(t *testing.T) {
	g := buildBlastGraph()

	// Nothing depends on cmd (it sits at the root of every chain).
	res := ComputeBlastRadius(g, "pkg:cmd", 0)
	if res.Total != 0 {
		t.Errorf("cmd has no dependents; got %d", res.Total)
	}
	if res.MaxDepthHit {
		t.Error("MaxDepthHit must be false when nothing was discovered")
	}
}

func TestComputeBlastRadius_CycleSafe(t *testing.T) {
	g := model.NewGraph("/tmp/cycle")
	g.AddNode(&model.Node{ID: "a", Name: "A", Type: model.NodePackage, Path: "a"})
	g.AddNode(&model.Node{ID: "b", Name: "B", Type: model.NodePackage, Path: "b"})
	g.AddEdge(&model.Edge{Source: "a", Target: "b", Type: model.EdgeDependency})
	g.AddEdge(&model.Edge{Source: "b", Target: "a", Type: model.EdgeDependency})

	res := ComputeBlastRadius(g, "a", 0)
	if res.Total != 1 {
		t.Errorf("cycle a→b→a from target a: expected 1 dependent (b), got %d", res.Total)
	}
}
