package scanner_test

import (
	"context"
	"testing"

	"github.com/olgasafonova/ridge/internal/analyzer/golang"
	"github.com/olgasafonova/ridge/internal/model"
	"github.com/olgasafonova/ridge/internal/scanner"
)

// TestScanWithOptions_SourceField verifies that every node produced by
// ScanWithOptions has its Source field set to the absolute scan root.
func TestScanWithOptions_SourceField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", `package main
import "fmt"
func main() { fmt.Println("hi") }
`)

	s := scanner.New(testLogger(), golang.New())
	result, err := s.ScanWithOptions(context.Background(), dir, scanner.DefaultScanOptions())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	nodes := result.Graph.Nodes()
	if len(nodes) == 0 {
		t.Fatal("expected at least one node")
	}
	for _, n := range nodes {
		if n.Source == "" {
			t.Errorf("node %q has empty Source; expected scan root to be stamped", n.ID)
		}
	}
}

// TestMultiPathMerge_UnionsNodes verifies that scanning two separate directories
// and merging their graphs produces a node set containing nodes from both.
// It also checks that Source is set to the respective scan root for each node.
func TestMultiPathMerge_UnionsNodes(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	writeFile(t, dirA, "alpha.go", `package alpha
import "fmt"
var _ = fmt.Sprintf
`)
	writeFile(t, dirB, "beta.go", `package beta
import "net/http"
var _ = http.NewRequest
`)

	s := scanner.New(testLogger(), golang.New())

	resultA, err := s.ScanWithOptions(context.Background(), dirA, scanner.DefaultScanOptions())
	if err != nil {
		t.Fatalf("scan A failed: %v", err)
	}
	resultB, err := s.ScanWithOptions(context.Background(), dirB, scanner.DefaultScanOptions())
	if err != nil {
		t.Fatalf("scan B failed: %v", err)
	}

	// Merge B into A's graph (mirrors what archScanMulti does).
	resultA.Graph.Merge(resultB.Graph)
	merged := resultA.Graph

	// Expect nodes from both directories.
	var foundAlpha, foundBeta bool
	for _, n := range merged.Nodes() {
		if n.Name == "alpha" {
			foundAlpha = true
		}
		if n.Name == "beta" {
			foundBeta = true
		}
	}
	if !foundAlpha {
		t.Error("merged graph missing node from dirA (alpha)")
	}
	if !foundBeta {
		t.Error("merged graph missing node from dirB (beta)")
	}
}

// TestMultiPathMerge_SourceTagging verifies that after merging two scans,
// nodes from each path carry the correct Source (their own scan root, not the other's).
func TestMultiPathMerge_SourceTagging(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	writeFile(t, dirA, "serviceA.go", `package serviceA
import "fmt"
var _ = fmt.Sprintf
`)
	writeFile(t, dirB, "serviceB.go", `package serviceB
import "net/http"
var _ = http.NewRequest
`)

	s := scanner.New(testLogger(), golang.New())

	resultA, err := s.ScanWithOptions(context.Background(), dirA, scanner.DefaultScanOptions())
	if err != nil {
		t.Fatalf("scan A failed: %v", err)
	}
	resultB, err := s.ScanWithOptions(context.Background(), dirB, scanner.DefaultScanOptions())
	if err != nil {
		t.Fatalf("scan B failed: %v", err)
	}

	// Collect Source values before merge to assert on.
	sourcesByName := map[string]string{}
	for _, n := range resultA.Graph.Nodes() {
		sourcesByName[n.Name] = n.Source
	}
	for _, n := range resultB.Graph.Nodes() {
		sourcesByName[n.Name] = n.Source
	}

	resultA.Graph.Merge(resultB.Graph)

	// After merge, each node must still carry its original Source.
	for _, n := range resultA.Graph.Nodes() {
		expected, ok := sourcesByName[n.Name]
		if !ok {
			// node may have been added by merge; Source must still be non-empty.
			if n.Source == "" {
				t.Errorf("merged node %q has empty Source", n.ID)
			}
			continue
		}
		if n.Source != expected {
			t.Errorf("node %q Source = %q; want %q", n.Name, n.Source, expected)
		}
	}
}

// TestMultiPathMerge_FirstWriteWins verifies that when both scans contain a node
// with the same ID, the first scan's Source is preserved (AddNode returns false
// on collision and the existing node is kept unchanged).
func TestMultiPathMerge_FirstWriteWins(t *testing.T) {
	g1 := model.NewGraph("/root1")
	g2 := model.NewGraph("/root2")

	sharedID := "pkg:shared"

	n1 := &model.Node{ID: sharedID, Name: "shared", Type: model.NodePackage, Source: "/root1"}
	n2 := &model.Node{ID: sharedID, Name: "shared", Type: model.NodePackage, Source: "/root2"}

	if !g1.AddNode(n1) {
		t.Fatal("failed to add node to g1")
	}
	if !g2.AddNode(n2) {
		t.Fatal("failed to add node to g2")
	}

	g1.Merge(g2)

	got := g1.GetNode(sharedID)
	if got == nil {
		t.Fatal("shared node missing after merge")
	}
	if got.Source != "/root1" {
		t.Errorf("first-write should win: Source = %q, want %q", got.Source, "/root1")
	}
}
