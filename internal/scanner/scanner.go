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
	MaxFiles     int           // 0 = unlimited
	MaxNodes     int           // 0 = unlimited
	Timeout      time.Duration // 0 = no timeout
	SkipDirs     []string      // additional dirs to skip (merged with defaults)
	SkipGlobs    []string      // filepath.Match patterns to skip (e.g. "*_test.go")
	IncludeTests bool          // if true, don't skip test files (default: skip them)
	Workers      int           // 0 = runtime.NumCPU(), capped at 8
	State        *ScanState    // non-nil enables incremental scanning (reuse cached results for unchanged files)
}

// defaultTestGlobs are file patterns excluded by default to avoid test fixtures
// polluting the architecture graph. Set IncludeTests=true to override.
var defaultTestGlobs = []string{
	"*_test.go",
	"*.test.ts",
	"*.test.tsx",
	"*.spec.ts",
	"*.spec.tsx",
	"*.test.js",
	"*.spec.js",
	"test_*.py",
	"*_test.py",
	"conftest.py",
}

// DefaultScanOptions returns permissive defaults with no limits.
func DefaultScanOptions() ScanOptions {
	return ScanOptions{}
}

// ScanResult holds the graph and statistics from a scan.
type ScanResult struct {
	Graph     *model.ArchGraph
	Stats     ScanStats
	Truncated bool       // true if any limit was hit
	State     *ScanState // updated state after scan (for persistence)
}

// ScanStats contains metrics about the scan.
type ScanStats struct {
	FilesAnalyzed int   `json:"files_analyzed"`
	FilesSkipped  int   `json:"files_skipped"`
	FilesCached   int   `json:"files_cached,omitempty"`  // files reused from cache
	FilesChanged  int   `json:"files_changed,omitempty"` // files re-analyzed due to changes
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
	path  string // source file path (for state updates)
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
// Uses a three-phase approach: collect files, detect changes (if incremental), then analyze.
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

	// Merge default test globs unless IncludeTests is set
	skipGlobs := opts.SkipGlobs
	if !opts.IncludeTests {
		skipGlobs = append(skipGlobs, defaultTestGlobs...)
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
		if len(skipGlobs) > 0 {
			baseName := d.Name()
			relPath, _ := filepath.Rel(absRoot, path)
			for _, pattern := range skipGlobs {
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

	// Phase 1.5: Detect changes (incremental mode)
	// Separate files into "needs analysis" vs "use cached results"
	var toAnalyze []fileWork
	var cachedResults []analyzeResult
	state := opts.State

	if state != nil {
		walkedPaths := make([]string, len(files))
		for i, f := range files {
			walkedPaths[i] = f.path
		}

		changes, detectErr := state.DetectChanges(walkedPaths)
		if detectErr != nil {
			s.logger.Warn("Change detection failed, falling back to full scan", "error", detectErr)
			toAnalyze = files
		} else {
			// Build path->ext lookup for changed files
			extByPath := make(map[string]string, len(files))
			for _, f := range files {
				extByPath[f.path] = f.ext
			}

			// Queue changed files for analysis
			for _, path := range changes.Added {
				toAnalyze = append(toAnalyze, fileWork{path: path, ext: extByPath[path]})
			}
			for _, path := range changes.Modified {
				toAnalyze = append(toAnalyze, fileWork{path: path, ext: extByPath[path]})
			}
			stats.FilesChanged = len(toAnalyze)

			// Collect cached results for unchanged files
			for _, path := range changes.Unchanged {
				nodes, edges, ok := state.CachedResult(path)
				if ok {
					cachedResults = append(cachedResults, analyzeResult{nodes: nodes, edges: edges, path: path})
					stats.FilesCached++
				} else {
					// State inconsistency: re-analyze
					toAnalyze = append(toAnalyze, fileWork{path: path, ext: extByPath[path]})
				}
			}

			// Clean up deleted files from state
			for _, path := range changes.Deleted {
				state.RemoveFile(path)
			}

			s.logger.Info("Incremental change detection",
				"unchanged", len(changes.Unchanged),
				"added", len(changes.Added),
				"modified", len(changes.Modified),
				"deleted", len(changes.Deleted),
			)
		}
	} else {
		toAnalyze = files
	}

	// Phase 2: Analyze files (only changed ones in incremental mode)
	graph := model.NewGraph(absRoot)

	// First, merge cached results into the graph
	for _, r := range cachedResults {
		for _, n := range r.nodes {
			if graph.AddNode(n) {
				stats.NodesFound++
			}
		}
		for _, e := range r.edges {
			graph.AddEdge(e)
			stats.EdgesFound++
		}
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > 8 {
		workers = 8
	}
	if workers > len(toAnalyze) {
		workers = len(toAnalyze)
	}

	if len(toAnalyze) == 0 {
		// Nothing to analyze (all cached or empty)
	} else if workers <= 1 {
		// Single-threaded path: simpler, avoids clone overhead
		for _, f := range toAnalyze {
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

			// Update state with fresh results
			if state != nil {
				if updateErr := state.UpdateFile(f.path, nodes, edges); updateErr != nil {
					s.logger.Warn("State update error", "path", f.path, "error", updateErr)
				}
			}

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
		workCh := make(chan fileWork, len(toAnalyze))
		resultCh := make(chan analyzeResult, len(toAnalyze))

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
					resultCh <- analyzeResult{nodes: nodes, edges: edges, path: f.path}
				}
			}()
		}

		// Send all work
		for _, f := range toAnalyze {
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

			// Update state with fresh results
			if state != nil {
				if updateErr := state.UpdateFile(r.path, r.nodes, r.edges); updateErr != nil {
					s.logger.Warn("State update error", "path", r.path, "error", updateErr)
				}
			}

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

	// Update state timestamp
	if state != nil {
		state.LastScan = time.Now()
	}

	stats.DurationMs = time.Since(start).Milliseconds()

	// Context cancellation produces a partial result, not a hard error
	if ctx.Err() != nil {
		truncated = true
	}

	s.logger.Info("Scan complete",
		"root", absRoot,
		"files_analyzed", stats.FilesAnalyzed,
		"files_cached", stats.FilesCached,
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
		State:     state,
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
