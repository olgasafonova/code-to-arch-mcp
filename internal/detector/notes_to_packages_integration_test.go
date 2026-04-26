package detector_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/golang"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/markdown"
	"github.com/olgasafonova/code-to-arch-mcp/internal/detector"
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

// TestLinkNotesToPackages_ThisRepo runs the cross-substrate spike against the
// project root, satisfying the bead acceptance criterion "test on this repo's
// own README ↔ internal/ tree." Emits the discovered Note→Package edges via
// t.Log so the test doubles as the spike's findings record.
func TestLinkNotesToPackages_ThisRepo(t *testing.T) {
	root := projectRoot(t)
	if root == "" {
		t.Skip("could not locate project root (no go.mod found)")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := scanner.New(logger, golang.New(), markdown.New())

	res, err := s.ScanWithOptions(context.Background(), root, scanner.ScanOptions{
		Timeout:  60 * time.Second,
		SkipDirs: []string{".git", ".beads", "node_modules", "vendor"},
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	added := detector.LinkNotesToPackages(res.Graph)
	t.Logf("cross-substrate edges added: %d", added)

	for _, e := range res.Graph.Edges() {
		if e.Label == "documents" {
			t.Logf("  %s -> %s (confidence %.2f)", e.Source, e.Target, e.Confidence)
		}
	}

	// Validate the spike actually finds something. This repo's CLAUDE.md
	// references many internal/* packages in code spans; if we get zero
	// matches, the heuristic is broken.
	if added < 3 {
		t.Errorf("expected at least 3 cross-substrate edges in this repo, got %d", added)
	}

	// Sanity check: at least one edge should target an internal/ Go package.
	var foundInternal bool
	for _, e := range res.Graph.Edges() {
		if e.Label == "documents" {
			target := lookupNode(res.Graph, e.Target)
			if target != nil && target.Type == model.NodePackage && containsPath(target.Path, "internal") {
				foundInternal = true
				break
			}
		}
	}
	if !foundInternal {
		t.Errorf("no Note→internal/* package edge found; CLAUDE.md should produce several")
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for range 6 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

func lookupNode(g *model.ArchGraph, id string) *model.Node {
	for _, n := range g.Nodes() {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func containsPath(p, segment string) bool {
	cleaned := filepath.Clean(p)
	sep := string(filepath.Separator)
	return strings.Contains(cleaned, sep+segment+sep) || strings.HasSuffix(cleaned, sep+segment)
}
