package scanner

import (
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Scanner walks a codebase directory and delegates files to registered analyzers.
type Scanner struct {
	analyzers map[string]Analyzer // extension -> analyzer
	logger    *slog.Logger
	skipDirs  map[string]bool
}

// New creates a Scanner with the given analyzers.
func New(logger *slog.Logger, analyzers ...Analyzer) *Scanner {
	extMap := make(map[string]Analyzer)
	for _, a := range analyzers {
		for _, ext := range a.Extensions() {
			extMap[ext] = a
		}
	}

	return &Scanner{
		analyzers: extMap,
		logger:    logger,
		skipDirs: map[string]bool{
			"node_modules": true,
			".git":         true,
			"vendor":       true,
			"dist":         true,
			"build":        true,
			"__pycache__":  true,
			".venv":        true,
			"venv":         true,
			".next":        true,
			".nuxt":        true,
			"target":       true, // Rust/Java build output
		},
	}
}

// Scan walks the directory tree and returns an ArchGraph with all discovered architecture.
func (s *Scanner) Scan(rootPath string) (*model.ArchGraph, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	graph := model.NewGraph(absRoot)
	var fileCount, nodeCount, edgeCount int

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip files we can't access
		}

		if d.IsDir() {
			if s.skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		analyzer, ok := s.analyzers[ext]
		if !ok {
			return nil
		}

		nodes, edges, analyzeErr := analyzer.Analyze(path)
		if analyzeErr != nil {
			s.logger.Warn("Analyzer error", "path", path, "error", analyzeErr)
			return nil // continue scanning other files
		}

		fileCount++
		for _, n := range nodes {
			if graph.AddNode(n) {
				nodeCount++
			}
		}
		for _, e := range edges {
			graph.AddEdge(e)
			edgeCount++
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	s.logger.Info("Scan complete",
		"root", absRoot,
		"files", fileCount,
		"nodes", nodeCount,
		"edges", edgeCount,
	)

	return graph, nil
}

// SupportedExtensions returns all file extensions the scanner handles.
func (s *Scanner) SupportedExtensions() []string {
	exts := make([]string, 0, len(s.analyzers))
	for ext := range s.analyzers {
		exts = append(exts, ext)
	}
	return exts
}
