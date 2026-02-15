package detector

import (
	"slices"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// helper to build a minimal graph with nodes and edges.
func buildGraph(nodes []*model.Node, edges []*model.Edge) *model.ArchGraph {
	g := model.NewGraph("/test")
	for _, n := range nodes {
		g.AddNode(n)
	}
	for _, e := range edges {
		g.AddEdge(e)
	}
	return g
}

func TestRecommend_NoCycles_Clean(t *testing.T) {
	g := buildGraph(
		[]*model.Node{
			{ID: "a", Name: "a", Type: model.NodePackage},
			{ID: "b", Name: "b", Type: model.NodePackage},
			{ID: "c", Name: "c", Type: model.NodePackage},
		},
		[]*model.Edge{
			{Source: "a", Target: "b", Type: model.EdgeDependency},
			{Source: "b", Target: "c", Type: model.EdgeDependency},
		},
	)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	if len(recs) != 0 {
		t.Errorf("expected 0 recommendations for clean graph, got %d: %+v", len(recs), recs)
	}
}

func TestRecommend_CycleDetected(t *testing.T) {
	g := buildGraph(
		[]*model.Node{
			{ID: "a", Name: "a", Type: model.NodePackage},
			{ID: "b", Name: "b", Type: model.NodePackage},
			{ID: "c", Name: "c", Type: model.NodePackage},
		},
		[]*model.Edge{
			{Source: "a", Target: "b", Type: model.EdgeDependency},
			{Source: "b", Target: "c", Type: model.EdgeDependency},
			{Source: "c", Target: "a", Type: model.EdgeDependency},
		},
	)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	found := false
	for _, r := range recs {
		if r.Category == "break_cycle" {
			found = true
			if r.Priority != "high" {
				t.Errorf("break_cycle should be high priority, got %s", r.Priority)
			}
			if len(r.Subject) != 3 {
				t.Errorf("expected 3 nodes in cycle subject, got %d", len(r.Subject))
			}
		}
	}
	if !found {
		t.Errorf("expected break_cycle recommendation, got %+v", recs)
	}
}

func TestRecommend_HighCoupling(t *testing.T) {
	// Build a dense graph where most nodes have coupling ~3, and "hub" has 10.
	// 11 nodes total, no zero-coupling leaves → avg ≈ 3.6, threshold ≈ 7.2.
	// Hub at 10 exceeds that.
	nodes := []*model.Node{
		{ID: "hub", Name: "hub", Type: model.NodePackage},
	}
	targetIDs := make([]string, 10)
	var edges []*model.Edge
	for i := range 10 {
		id := string(rune('a' + i))
		targetIDs[i] = id
		nodes = append(nodes, &model.Node{ID: id, Name: id, Type: model.NodePackage})
		edges = append(edges, &model.Edge{Source: "hub", Target: id, Type: model.EdgeDependency})
	}
	// Each target depends on 3 other targets (circular deps among targets).
	for i, src := range targetIDs {
		for j := 1; j <= 3; j++ {
			dst := targetIDs[(i+j)%len(targetIDs)]
			edges = append(edges, &model.Edge{Source: src, Target: dst, Type: model.EdgeDependency})
		}
	}

	g := buildGraph(nodes, edges)
	metrics := ComputeMetrics(g)
	violations := ValidateGraph(g, nil)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	found := false
	for _, r := range recs {
		if r.Category == "reduce_coupling" {
			for _, s := range r.Subject {
				if s == "hub" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected reduce_coupling for hub node, got %+v", recs)
	}
}

func TestRecommend_UnstableCore(t *testing.T) {
	// Node "core" has high fan-in (4 dependents) and high fan-out (4 deps) → instability ~0.5
	// To get instability > 0.7 with fan-in > 3, we need more fan-out than fan-in.
	// fan-in=4, fan-out=10 → instability = 10/14 ≈ 0.71
	nodes := []*model.Node{
		{ID: "core", Name: "core", Type: model.NodePackage},
	}
	var edges []*model.Edge

	// 4 dependents pointing to core
	for i := range 4 {
		id := string(rune('A' + i))
		nodes = append(nodes, &model.Node{ID: id, Name: id, Type: model.NodePackage})
		edges = append(edges, &model.Edge{Source: id, Target: "core", Type: model.EdgeDependency})
	}
	// core depends on 10 others
	for i := range 10 {
		id := string(rune('a' + i))
		nodes = append(nodes, &model.Node{ID: id, Name: id, Type: model.NodePackage})
		edges = append(edges, &model.Edge{Source: "core", Target: id, Type: model.EdgeDependency})
	}

	g := buildGraph(nodes, edges)
	metrics := ComputeMetrics(g)
	violations := ValidateGraph(g, nil)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	found := false
	for _, r := range recs {
		if r.Category == "stabilize_core" {
			for _, s := range r.Subject {
				if s == "core" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected stabilize_core for core node, got %+v", recs)
	}
}

func TestRecommend_EndpointToDatabase(t *testing.T) {
	g := buildGraph(
		[]*model.Node{
			{ID: "ep", Name: "GET /users", Type: model.NodeEndpoint},
			{ID: "db", Name: "postgres", Type: model.NodeDatabase},
		},
		[]*model.Edge{
			{Source: "ep", Target: "db", Type: model.EdgeDependency},
		},
	)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	found := false
	for _, r := range recs {
		if r.Category == "add_layer" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected add_layer recommendation, got %+v", recs)
	}
}

func TestRecommend_SharedDatabase(t *testing.T) {
	g := buildGraph(
		[]*model.Node{
			{ID: "svc1", Name: "svc1", Type: model.NodeService},
			{ID: "svc2", Name: "svc2", Type: model.NodeService},
			{ID: "svc3", Name: "svc3", Type: model.NodeService},
			{ID: "db", Name: "postgres", Type: model.NodeDatabase},
		},
		[]*model.Edge{
			{Source: "svc1", Target: "db", Type: model.EdgeReadWrite},
			{Source: "svc2", Target: "db", Type: model.EdgeReadWrite},
			{Source: "svc3", Target: "db", Type: model.EdgeReadWrite},
		},
	)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	found := false
	for _, r := range recs {
		if r.Category == "split_database" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected split_database recommendation, got %+v", recs)
	}
}

func TestRecommend_MissingCache(t *testing.T) {
	nodes := []*model.Node{
		{ID: "svc", Name: "svc", Type: model.NodeService},
	}
	var edges []*model.Edge
	// 6 endpoints connected to the service
	for i := range 6 {
		id := string(rune('a' + i))
		nodes = append(nodes, &model.Node{ID: id, Name: "GET /" + id, Type: model.NodeEndpoint})
		edges = append(edges, &model.Edge{Source: id, Target: "svc", Type: model.EdgeAPICall, Label: "serves"})
	}

	g := buildGraph(nodes, edges)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	found := false
	for _, r := range recs {
		if r.Category == "add_caching" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected add_caching recommendation, got %+v", recs)
	}
}

func TestRecommend_Orphan(t *testing.T) {
	g := buildGraph(
		[]*model.Node{
			{ID: "a", Name: "a", Type: model.NodePackage},
			{ID: "b", Name: "b", Type: model.NodePackage},
			{ID: "orphan", Name: "orphan", Type: model.NodePackage},
		},
		[]*model.Edge{
			{Source: "a", Target: "b", Type: model.EdgeDependency},
		},
	)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	found := false
	for _, r := range recs {
		if r.Category == "remove_orphan" {
			for _, s := range r.Subject {
				if s == "orphan" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected remove_orphan for orphan node, got %+v", recs)
	}
}

func TestRecommend_MultipleIssues(t *testing.T) {
	// Graph with a cycle and an orphan: should produce multiple recommendations
	// sorted high before low.
	g := buildGraph(
		[]*model.Node{
			{ID: "a", Name: "a", Type: model.NodePackage},
			{ID: "b", Name: "b", Type: model.NodePackage},
			{ID: "orphan", Name: "orphan", Type: model.NodePackage},
		},
		[]*model.Edge{
			{Source: "a", Target: "b", Type: model.EdgeDependency},
			{Source: "b", Target: "a", Type: model.EdgeDependency},
		},
	)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)
	if len(recs) < 2 {
		t.Fatalf("expected at least 2 recommendations, got %d: %+v", len(recs), recs)
	}

	// Verify sorting: first recommendation should be high priority.
	if recs[0].Priority != "high" {
		t.Errorf("first recommendation should be high priority, got %s (category: %s)", recs[0].Priority, recs[0].Category)
	}

	// Last recommendation should be low priority (orphan).
	last := recs[len(recs)-1]
	if last.Priority != "low" {
		t.Errorf("last recommendation should be low priority, got %s (category: %s)", last.Priority, last.Category)
	}
}

func TestRecommend_PriorityBoosting(t *testing.T) {
	// Node "x" is in a cycle AND has high fan-in → multiple rules flag it.
	// The orphan rule for "orphan" should get boosted if "orphan" appears in
	// multiple rules. But here we test: node "x" is in a cycle (high already),
	// and we add a medium-priority recommendation for "x" via split_module
	// (fan-out > 8). The boosting should promote that to high because
	// subjectCount["x"] > 1.
	nodes := []*model.Node{
		{ID: "x", Name: "x", Type: model.NodePackage},
		{ID: "y", Name: "y", Type: model.NodePackage},
	}
	var edges []*model.Edge
	// Cycle between x and y
	edges = append(edges,
		&model.Edge{Source: "x", Target: "y", Type: model.EdgeDependency},
		&model.Edge{Source: "y", Target: "x", Type: model.EdgeDependency},
	)
	// x also depends on 9 other nodes (fan-out > 8 → split_module)
	for i := range 9 {
		id := string(rune('a' + i))
		nodes = append(nodes, &model.Node{ID: id, Name: id, Type: model.NodePackage})
		edges = append(edges, &model.Edge{Source: "x", Target: id, Type: model.EdgeDependency})
	}
	// Give those 9 nodes some outgoing deps so avg coupling > 2
	for i := range 9 {
		src := string(rune('a' + i))
		for j := range 3 {
			depID := src + string(rune('0'+j))
			nodes = append(nodes, &model.Node{ID: depID, Name: depID, Type: model.NodePackage})
			edges = append(edges, &model.Edge{Source: src, Target: depID, Type: model.EdgeDependency})
		}
	}

	g := buildGraph(nodes, edges)
	violations := ValidateGraph(g, nil)
	metrics := ComputeMetrics(g)
	explanation := ExplainArchitecture(g, nil)

	recs := RecommendArchitecture(g, violations, metrics, explanation)

	// Find the split_module recommendation for "x" and check it was boosted.
	for _, r := range recs {
		if r.Category == "split_module" && slices.Contains(r.Subject, "x") {
			if r.Priority != "high" {
				t.Errorf("split_module for x should be boosted to high (compound risk), got %s", r.Priority)
			}
			return
		}
	}
	t.Errorf("expected split_module recommendation for x, got %+v", recs)
}
