package render_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/markdown"
	"github.com/olgasafonova/code-to-arch-mcp/internal/render"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

// TestVaultRender scans a markdown directory and writes Mermaid + HTML
// renderings to /tmp so we can eyeball them in a browser. Skipped unless
// MARKDOWN_INTEGRATION_VAULT is set.
//
//	MARKDOWN_INTEGRATION_VAULT=~/Documents/obsidian-home/AI-Knowledge \
//	  go test -v -run TestVaultRender ./internal/render/
func TestVaultRender(t *testing.T) {
	root := os.Getenv("MARKDOWN_INTEGRATION_VAULT")
	if root == "" {
		t.Skip("MARKDOWN_INTEGRATION_VAULT not set")
	}
	if _, err := os.Stat(root); err != nil {
		t.Skipf("vault root unreachable: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	s := scanner.New(logger, markdown.New())

	res, err := s.ScanWithOptions(context.Background(), root, scanner.ScanOptions{
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	outDir := filepath.Join(os.TempDir(), "code-to-arch-vault-render")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		viewLevel render.ViewLevel
		threshold float64
		file      string
	}{
		{"container", render.ViewContainer, 0, "vault-container.mmd"},
		{"component-pruned", render.ViewComponent, 0.05, "vault-pruned.mmd"},
	}

	for _, c := range cases {
		opts := render.Options{
			Format:         render.FormatMermaid,
			ViewLevel:      c.viewLevel,
			Title:          "AI Knowledge",
			Direction:      "LR",
			PruneThreshold: c.threshold,
		}
		out := render.Mermaid(res.Graph, opts)
		path := filepath.Join(outDir, c.file)
		if err := os.WriteFile(path, []byte(out), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s (%d bytes): %s", c.name, len(out), path)
	}

	htmlOpts := render.Options{
		Format:    render.FormatHTML,
		ViewLevel: render.ViewComponent,
		Title:     "AI Knowledge (hubs, degree>=10)",
		Direction: "LR",
		MinDegree: 10,
	}
	html := render.HTML(res.Graph, htmlOpts)
	htmlPath := filepath.Join(outDir, "vault.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("html (%d bytes): %s", len(html), htmlPath)
}
