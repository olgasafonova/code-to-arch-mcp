package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// ErrLimitReached is returned alongside a partial graph when a scan limit is hit.
var ErrLimitReached = errors.New("scan limit reached")

// ScanOptions controls scan behavior.
type ScanOptions struct {
	MaxFiles  int           // 0 = unlimited
	MaxNodes  int           // 0 = unlimited
	Timeout   time.Duration // 0 = no timeout
	SkipDirs  []string      // additional dirs to skip (merged with defaults)
	SkipGlobs []string      // filepath.Match patterns to skip (e.g. "*_test.go")
	Workers   int           // 0 = runtime.NumCPU(), capped at 8
}

// DefaultScanOptions returns permissive defaults with no limits.
func DefaultScanOptions() ScanOptions {
	return ScanOptions{}
}

// ScanResult holds the graph and statistics from a scan.
type ScanResult struct {
	Graph     *model.ArchGraph
	Stats     ScanStats
	Truncated bool // true if any limit was hit
}

// ScanStats contains metrics about the scan.
type ScanStats struct {
	FilesAnalyzed int   `json:"files_analyzed"`
	FilesSkipped  int   `json:"files_skipped"`
	NodesFound    int   `json:"nodes_found"`
	EdgesFound    int   `json:"edges_found"`
	DurationMs    int64 `json:"duration_ms"`
}

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

// Scan walks the directory tree and returns an ArchGraph. Backwards-compatible wrapper.
func (s *Scanner) Scan(rootPath string) (*model.ArchGraph, error) {
	result, err := s.ScanWithOptions(context.Background(), rootPath, DefaultScanOptions())
	if err != nil && !errors.Is(err, ErrLimitReached) {
		return nil, err
	}
	return result.Graph, nil
}

// fileWork represents a file to be analyzed.
type fileWork struct {
	path string
	ext  string
}

// analyzeResult holds the output of analyzing a single file.
type analyzeResult struct {
	nodes []*model.Node
	edges []*model.Edge
}

// cloneAnalyzers creates independent copies of all registered analyzers.
func (s *Scanner) cloneAnalyzers() map[string]Analyzer {
	cloned := make(map[string]Analyzer, len(s.analyzers))
	for ext, a := range s.analyzers {
		cloned[ext] = a.Clone()
	}
	return cloned
}

// ScanWithOptions walks the directory tree with configurable limits, timeout, and skip patterns.
// Uses a two-phase approach: collect files (single-threaded), then analyze (parallel workers).
func (s *Scanner) ScanWithOptions(ctx context.Context, rootPath string, opts ScanOptions) (*ScanResult, error) {
	start := time.Now()

	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	// Apply timeout if set
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Build merged skip dirs (don't mutate s.skipDirs)
	mergedSkipDirs := make(map[string]bool, len(s.skipDirs)+len(opts.SkipDirs))
	for k, v := range s.skipDirs {
		mergedSkipDirs[k] = v
	}
	for _, d := range opts.SkipDirs {
		mergedSkipDirs[d] = true
	}

	// Phase 1: Collect files (single-threaded walk)
	var files []fileWork
	var stats ScanStats
	truncated := false

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip files we can't access
		}

		// Check context cancellation
		if ctx.Err() != nil {
			truncated = true
			return filepath.SkipAll
		}

		// Check file limit
		if opts.MaxFiles > 0 && len(files) >= opts.MaxFiles {
			truncated = true
			return filepath.SkipAll
		}

		if d.IsDir() {
			if mergedSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check skip globs against base name and relative path
		if len(opts.SkipGlobs) > 0 {
			baseName := d.Name()
			relPath, _ := filepath.Rel(absRoot, path)
			for _, pattern := range opts.SkipGlobs {
				if matched, _ := filepath.Match(pattern, baseName); matched {
					stats.FilesSkipped++
					return nil
				}
				if matched, _ := filepath.Match(pattern, relPath); matched {
					stats.FilesSkipped++
					return nil
				}
			}
		}

		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := s.analyzers[ext]; !ok {
			return nil
		}

		files = append(files, fileWork{path: path, ext: ext})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	if ctx.Err() != nil {
		truncated = true
	}

	// Phase 2: Analyze files
	graph := model.NewGraph(absRoot)

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > 8 {
		workers = 8
	}
	if workers > len(files) {
		workers = len(files)
	}

	if len(files) == 0 {
		// Nothing to analyze
	} else if workers <= 1 {
		// Single-threaded path: simpler, avoids clone overhead
		for _, f := range files {
			if ctx.Err() != nil {
				truncated = true
				break
			}
			nodes, edges, analyzeErr := s.analyzers[f.ext].Analyze(f.path)
			if analyzeErr != nil {
				s.logger.Warn("Analyzer error", "path", f.path, "error", analyzeErr)
				continue
			}
			stats.FilesAnalyzed++
			for _, n := range nodes {
				if opts.MaxNodes > 0 && stats.NodesFound >= opts.MaxNodes {
					truncated = true
					break
				}
				if graph.AddNode(n) {
					stats.NodesFound++
				}
			}
			if truncated {
				break
			}
			for _, e := range edges {
				graph.AddEdge(e)
				stats.EdgesFound++
			}
		}
	} else {
		// Multi-worker path: fan out analysis to goroutines
		workCh := make(chan fileWork, len(files))
		resultCh := make(chan analyzeResult, len(files))

		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cloned := s.cloneAnalyzers()
				for f := range workCh {
					if ctx.Err() != nil {
						continue // drain channel
					}
					nodes, edges, err := cloned[f.ext].Analyze(f.path)
					if err != nil {
						s.logger.Warn("Analyzer error", "path", f.path, "error", err)
						continue
					}
					resultCh <- analyzeResult{nodes: nodes, edges: edges}
				}
			}()
		}

		// Send all work
		for _, f := range files {
			workCh <- f
		}
		close(workCh)

		// Close results when all workers finish
		go func() {
			wg.Wait()
			close(resultCh)
		}()

		// Collect results in main goroutine (single-threaded merge)
		for r := range resultCh {
			stats.FilesAnalyzed++
			for _, n := range r.nodes {
				if opts.MaxNodes > 0 && stats.NodesFound >= opts.MaxNodes {
					truncated = true
					break
				}
				if graph.AddNode(n) {
					stats.NodesFound++
				}
			}
			for _, e := range r.edges {
				graph.AddEdge(e)
				stats.EdgesFound++
			}
		}
	}

	stats.DurationMs = time.Since(start).Milliseconds()

	// Context cancellation produces a partial result, not a hard error
	if ctx.Err() != nil {
		truncated = true
	}

	s.logger.Info("Scan complete",
		"root", absRoot,
		"files", stats.FilesAnalyzed,
		"nodes", stats.NodesFound,
		"edges", stats.EdgesFound,
		"workers", workers,
		"truncated", truncated,
		"duration_ms", stats.DurationMs,
	)

	result := &ScanResult{
		Graph:     graph,
		Stats:     stats,
		Truncated: truncated,
	}

	if truncated {
		return result, ErrLimitReached
	}
	return result, nil
}

// SupportedExtensions returns all file extensions the scanner handles.
func (s *Scanner) SupportedExtensions() []string {
	exts := make([]string, 0, len(s.analyzers))
	for ext := range s.analyzers {
		exts = append(exts, ext)
	}
	return exts
}
