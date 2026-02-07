package scanner

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/golang"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestScan_GoProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", `package main

import "fmt"

func main() { fmt.Println("hi") }
`)
	writeFile(t, dir, "handler.go", `package main

import "net/http"

func init() {
	http.HandleFunc("/health", nil)
}
`)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(logger, golang.New())
	graph, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if graph.NodeCount() == 0 {
		t.Fatal("expected nodes from scan")
	}
	if graph.EdgeCount() == 0 {
		t.Fatal("expected edges from scan")
	}
}

func TestScan_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.go", `package main
func main() {}
`)
	// Files in node_modules should be skipped
	writeFile(t, dir, "node_modules/pkg/main.go", `package pkg
import "database/sql"
var _ *sql.DB
`)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(logger, golang.New())
	graph, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should only have nodes from the root main.go, not from node_modules
	for _, n := range graph.Nodes() {
		if n.Name == "pkg" {
			t.Fatal("node_modules directory should be skipped")
		}
	}
}

func TestScan_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(logger, golang.New())
	graph, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if graph.NodeCount() != 0 {
		t.Fatalf("expected 0 nodes in empty dir, got %d", graph.NodeCount())
	}
}

func TestScan_UnsupportedFileSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.md", `# Hello`)
	writeFile(t, dir, "data.json", `{"key":"value"}`)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(logger, golang.New())
	graph, err := s.Scan(dir)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if graph.NodeCount() != 0 {
		t.Fatal("non-Go files should produce no nodes")
	}
}

func TestSupportedExtensions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	s := New(logger, golang.New())
	exts := s.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".go" {
		t.Fatalf("expected [.go], got %v", exts)
	}
}
