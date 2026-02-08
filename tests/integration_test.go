//go:build integration

package tests

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/golang"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/python"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/typescript"
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/render"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

func newScanner(t *testing.T) *scanner.Scanner {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return scanner.New(logger, golang.New(), typescript.New(), python.New())
}

func requireDir(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Skipf("cannot resolve %s: %v", path, err)
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		t.Skipf("directory not found, skipping: %s", abs)
	}
	return abs
}

// ---------------------------------------------------------------------------
// Tier 1: Self-scan
// ---------------------------------------------------------------------------

func TestScanSelf(t *testing.T) {
	root := requireDir(t, "..")
	s := newScanner(t)

	graph, err := s.Scan(root)
	if err != nil {
		t.Fatalf("self-scan failed: %v", err)
	}

	if graph.NodeCount() < 5 {
		t.Fatalf("expected 5+ nodes, got %d", graph.NodeCount())
	}
	if graph.EdgeCount() < 10 {
		t.Fatalf("expected 10+ edges, got %d", graph.EdgeCount())
	}

	// Verify Go packages detected
	pkgs := graph.NodesByType(model.NodePackage)
	if len(pkgs) == 0 {
		t.Fatal("expected at least one package node")
	}

	// Verify Mermaid output
	mermaidOut := render.Mermaid(graph, render.DefaultOptions())
	if !strings.Contains(mermaidOut, "graph TB") {
		t.Fatal("Mermaid output missing graph directive")
	}

	// Verify PlantUML output
	plantOut := render.PlantUML(graph, render.Options{ViewLevel: render.ViewContainer})
	if !strings.Contains(plantOut, "@startuml") {
		t.Fatal("PlantUML output missing @startuml")
	}
	if !strings.Contains(plantOut, "@enduml") {
		t.Fatal("PlantUML output missing @enduml")
	}

	t.Logf("Self-scan: %d nodes, %d edges", graph.NodeCount(), graph.EdgeCount())
	t.Logf("Summary:\n%s", graph.Summary())
}

// ---------------------------------------------------------------------------
// Tier 2: Local Go projects
// ---------------------------------------------------------------------------

func TestScanLocalGoRepos(t *testing.T) {
	repos := []struct {
		name string
		path string
	}{
		{"mcp-registry", "~/Projects/mcp-registry"},
		{"miro-mcp-server", "~/Projects/miro-mcp-server"},
		{"skillcheck", "~/Projects/skillcheck"},
	}

	s := newScanner(t)

	for _, repo := range repos {
		t.Run(repo.name, func(t *testing.T) {
			expanded := expandHome(repo.path)
			root := requireDir(t, expanded)

			graph, err := s.Scan(root)
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}

			if graph.NodeCount() == 0 {
				t.Fatal("expected at least 1 node")
			}

			pkgs := graph.NodesByType(model.NodePackage)
			t.Logf("%s: %d nodes (%d packages), %d edges",
				repo.name, graph.NodeCount(), len(pkgs), graph.EdgeCount())

			// Verify both renderers produce output
			mOut := render.Mermaid(graph, render.DefaultOptions())
			if len(mOut) < 20 {
				t.Fatal("Mermaid output too short")
			}
			pOut := render.PlantUML(graph, render.Options{ViewLevel: render.ViewContainer})
			if len(pOut) < 20 {
				t.Fatal("PlantUML output too short")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tier 3: Local TypeScript projects
// ---------------------------------------------------------------------------

func TestScanLocalTSRepos(t *testing.T) {
	repos := []struct {
		name string
		path string
	}{
		{"sif-explorer", "~/Projects/sif-explorer"},
		{"nosefolio", "~/Projects/nosefolio"},
	}

	s := newScanner(t)

	for _, repo := range repos {
		t.Run(repo.name, func(t *testing.T) {
			expanded := expandHome(repo.path)
			root := requireDir(t, expanded)

			graph, err := s.Scan(root)
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}

			if graph.NodeCount() == 0 {
				t.Fatal("expected at least 1 node")
			}

			modules := graph.NodesByType(model.NodeModule)
			t.Logf("%s: %d nodes (%d modules), %d edges",
				repo.name, graph.NodeCount(), len(modules), graph.EdgeCount())
		})
	}
}

// ---------------------------------------------------------------------------
// Tier 4: Cloned open-source TypeScript repos
// ---------------------------------------------------------------------------

func TestScanClonedTSRepos(t *testing.T) {
	repos := []struct {
		name     string
		cloneURL string
		wantDB   bool
		wantEP   bool
	}{
		{
			name:     "express-mongoose-boilerplate",
			cloneURL: "https://github.com/saisilinus/node-express-mongoose-typescript-boilerplate",
			wantDB:   true,
			wantEP:   true,
		},
		{
			name:     "nestjs-realworld",
			cloneURL: "https://github.com/lujakob/nestjs-realworld-example-app",
			wantDB:   true,
			wantEP:   false, // NestJS uses decorators, not app.get()
		},
	}

	s := newScanner(t)

	for _, repo := range repos {
		t.Run(repo.name, func(t *testing.T) {
			// Clone to temp directory
			tmpDir := t.TempDir()
			cloneDir := filepath.Join(tmpDir, repo.name)

			t.Logf("Cloning %s ...", repo.cloneURL)
			if err := gitCloneShallow(cloneDir, repo.cloneURL); err != nil {
				t.Skipf("clone failed (network?): %v", err)
			}

			graph, err := s.Scan(cloneDir)
			if err != nil {
				t.Fatalf("scan failed: %v", err)
			}

			modules := graph.NodesByType(model.NodeModule)
			dbs := graph.NodesByType(model.NodeDatabase)
			endpoints := graph.NodesByType(model.NodeEndpoint)

			t.Logf("%s: %d nodes (%d modules, %d DBs, %d endpoints), %d edges",
				repo.name, graph.NodeCount(), len(modules), len(dbs), len(endpoints), graph.EdgeCount())

			if len(modules) == 0 {
				t.Fatal("expected at least 1 module node")
			}
			if repo.wantDB && len(dbs) == 0 {
				t.Error("expected database node but found none")
			}
			if repo.wantEP && len(endpoints) == 0 {
				t.Error("expected endpoint nodes but found none")
			}

			// Verify render doesn't panic
			render.Mermaid(graph, render.DefaultOptions())
			render.PlantUML(graph, render.Options{ViewLevel: render.ViewComponent})
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func gitCloneShallow(dest, url string) error {
	return exec.Command("git", "clone", "--depth", "1", url, dest).Run()
}
