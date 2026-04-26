package markdown_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/markdown"
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

// TestVaultIntegration scans a real markdown directory and reports orphans/hubs.
// Skipped unless the MARKDOWN_INTEGRATION_VAULT env var points at a directory.
//
// Run with:
//
//	MARKDOWN_INTEGRATION_VAULT=~/Documents/obsidian-home/AI-Knowledge \
//	  go test -v -run TestVaultIntegration ./internal/analyzer/markdown/
func TestVaultIntegration(t *testing.T) {
	root := os.Getenv("MARKDOWN_INTEGRATION_VAULT")
	if root == "" {
		t.Skip("MARKDOWN_INTEGRATION_VAULT not set")
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("vault root unreachable: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := scanner.New(logger, markdown.New())

	start := time.Now()
	res, err := s.ScanWithOptions(context.Background(), root, scanner.ScanOptions{
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if res == nil || res.Graph == nil {
		t.Fatal("nil scan result")
	}

	notes := res.Graph.NodesByType(model.NodeNote)
	if len(notes) == 0 {
		t.Fatalf("no notes found in %s", root)
	}

	resolved := res.Graph.ResolvedEdges()
	in := make(map[string]int)
	out := make(map[string]int)
	for _, e := range resolved {
		out[e.Source]++
		in[e.Target]++
	}

	var hubs, orphans []string
	for _, n := range notes {
		if in[n.ID] == 0 && out[n.ID] == 0 {
			orphans = append(orphans, n.Name)
		}
		if in[n.ID] >= 5 {
			hubs = append(hubs, fmt.Sprintf("%s (in=%d, out=%d)", n.Name, in[n.ID], out[n.ID]))
		}
	}
	sort.Strings(hubs)
	sort.Strings(orphans)

	rawEdges := res.Graph.EdgeCount()
	resolutionRate := 0.0
	if rawEdges > 0 {
		resolutionRate = 100 * float64(len(resolved)) / float64(rawEdges)
	}

	t.Logf("Vault: %s", root)
	t.Logf("Duration: %v", time.Since(start))
	t.Logf("Notes: %d", len(notes))
	t.Logf("Files analyzed: %d", res.Stats.FilesAnalyzed)
	t.Logf("Edges raw=%d resolved=%d (%.0f%% resolution rate)", rawEdges, len(resolved), resolutionRate)
	t.Logf("Hubs (in-degree >= 5): %d", len(hubs))
	for i, h := range hubs {
		if i >= 15 {
			t.Logf("  ...and %d more", len(hubs)-15)
			break
		}
		t.Logf("  - %s", h)
	}
	t.Logf("Orphans (zero in/out): %d", len(orphans))
	for i, o := range orphans {
		if i >= 10 {
			t.Logf("  ...and %d more", len(orphans)-10)
			break
		}
		t.Logf("  - %s", o)
	}
}
