package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/olgasafonova/ridge/internal/model"
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

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	mergedSkipDirs := s.mergeSkipDirs(opts.SkipDirs)
	skipGlobs := opts.SkipGlobs
	if !opts.IncludeTests {
		skipGlobs = append(skipGlobs, defaultTestGlobs...)
	}

	var stats ScanStats
	files, walkSkipped, truncated, err := s.walkAndCollect(ctx, absRoot, mergedSkipDirs, skipGlobs, opts.MaxFiles)
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}
	stats.FilesSkipped = walkSkipped

	state := opts.State
	toAnalyze, cachedResults := s.partitionForIncremental(files, state, &stats)

	graph := model.NewGraph(absRoot)
	mergeCachedResults(graph, cachedResults, &stats)

	workers := chooseWorkers(opts.Workers, len(toAnalyze))
	if analyzeTruncated := s.runAnalysis(ctx, toAnalyze, workers, state, graph, &stats, opts.MaxNodes); analyzeTruncated {
		truncated = true
	}

	if state != nil {
		state.LastScan = time.Now()
	}
	stats.DurationMs = time.Since(start).Milliseconds()

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

	result := &ScanResult{Graph: graph, Stats: stats, Truncated: truncated, State: state}
	if truncated {
		return result, ErrLimitReached
	}
	return result, nil
}

// mergeSkipDirs combines the scanner's defaults with any extras from opts,
// returning a fresh map so the scanner's own skipDirs is never mutated.
func (s *Scanner) mergeSkipDirs(extra []string) map[string]bool {
	merged := make(map[string]bool, len(s.skipDirs)+len(extra))
	maps.Copy(merged, s.skipDirs)
	for _, d := range extra {
		merged[d] = true
	}
	return merged
}

// walkAndCollect performs Phase 1: a single-threaded WalkDir that produces the
// file work list, applying skip-dirs, skip-globs, extension filtering, and the
// MaxFiles cap. Truncation is true if context cancellation or MaxFiles ended
// the walk early.
func (s *Scanner) walkAndCollect(ctx context.Context, absRoot string, skipDirs map[string]bool, skipGlobs []string, maxFiles int) (files []fileWork, skipped int, truncated bool, err error) {
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			truncated = true
			return filepath.SkipAll
		}
		if maxFiles > 0 && len(files) >= maxFiles {
			truncated = true
			return filepath.SkipAll
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if matchesAnyGlob(skipGlobs, d.Name(), path, absRoot) {
			skipped++
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := s.analyzers[ext]; !ok {
			return nil
		}
		files = append(files, fileWork{path: path, ext: ext})
		return nil
	})
	if ctx.Err() != nil {
		truncated = true
	}
	return files, skipped, truncated, err
}

// matchesAnyGlob returns true if name or relative path matches any of the patterns.
func matchesAnyGlob(patterns []string, baseName, fullPath, absRoot string) bool {
	if len(patterns) == 0 {
		return false
	}
	relPath, _ := filepath.Rel(absRoot, fullPath)
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, baseName); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
	}
	return false
}

// partitionForIncremental performs Phase 1.5: split walked files into work
// that needs analysis and cached results that can be reused. Without state,
// every file goes into toAnalyze. Updates stats.FilesChanged / FilesCached.
func (s *Scanner) partitionForIncremental(files []fileWork, state *ScanState, stats *ScanStats) (toAnalyze []fileWork, cached []analyzeResult) {
	if state == nil {
		return files, nil
	}

	walkedPaths := make([]string, len(files))
	for i, f := range files {
		walkedPaths[i] = f.path
	}

	changes, detectErr := state.DetectChanges(walkedPaths)
	if detectErr != nil {
		s.logger.Warn("Change detection failed, falling back to full scan", "error", detectErr)
		return files, nil
	}

	extByPath := make(map[string]string, len(files))
	for _, f := range files {
		extByPath[f.path] = f.ext
	}

	for _, path := range changes.Added {
		toAnalyze = append(toAnalyze, fileWork{path: path, ext: extByPath[path]})
	}
	for _, path := range changes.Modified {
		toAnalyze = append(toAnalyze, fileWork{path: path, ext: extByPath[path]})
	}
	stats.FilesChanged = len(toAnalyze)

	for _, path := range changes.Unchanged {
		nodes, edges, ok := state.CachedResult(path)
		if ok {
			cached = append(cached, analyzeResult{nodes: nodes, edges: edges, path: path})
			stats.FilesCached++
		} else {
			toAnalyze = append(toAnalyze, fileWork{path: path, ext: extByPath[path]})
		}
	}

	for _, path := range changes.Deleted {
		state.RemoveFile(path)
	}

	s.logger.Info("Incremental change detection",
		"unchanged", len(changes.Unchanged),
		"added", len(changes.Added),
		"modified", len(changes.Modified),
		"deleted", len(changes.Deleted),
	)
	return toAnalyze, cached
}

// mergeCachedResults adds previously-analyzed nodes and edges into the new graph.
func mergeCachedResults(graph *model.ArchGraph, cached []analyzeResult, stats *ScanStats) {
	for _, r := range cached {
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
}

// chooseWorkers picks the worker count: opts override or NumCPU, capped at 8
// and clamped to the size of the work queue.
func chooseWorkers(requested, queueSize int) int {
	w := requested
	if w <= 0 {
		w = runtime.NumCPU()
	}
	if w > 8 {
		w = 8
	}
	if w > queueSize {
		w = queueSize
	}
	return w
}

// runAnalysis dispatches to either the sequential or parallel analyzer path.
// Returns true if a limit was hit during analysis.
func (s *Scanner) runAnalysis(ctx context.Context, toAnalyze []fileWork, workers int, state *ScanState, graph *model.ArchGraph, stats *ScanStats, maxNodes int) bool {
	switch {
	case len(toAnalyze) == 0:
		return false
	case workers <= 1:
		return s.analyzeSequential(ctx, toAnalyze, state, graph, stats, maxNodes)
	default:
		return s.analyzeParallel(ctx, toAnalyze, workers, state, graph, stats, maxNodes)
	}
}

// analyzeSequential runs analyzers on the calling goroutine. Used when workers <= 1.
func (s *Scanner) analyzeSequential(ctx context.Context, toAnalyze []fileWork, state *ScanState, graph *model.ArchGraph, stats *ScanStats, maxNodes int) bool {
	for _, f := range toAnalyze {
		if ctx.Err() != nil {
			return true
		}
		nodes, edges, err := s.analyzers[f.ext].Analyze(f.path)
		if err != nil {
			s.logger.Warn("Analyzer error", "path", f.path, "error", err)
			continue
		}
		stats.FilesAnalyzed++
		s.maybeUpdateState(state, f.path, nodes, edges)
		if mergeNodesAndEdges(graph, nodes, edges, stats, maxNodes) {
			return true
		}
	}
	return false
}

// analyzeParallel fans out analyzer work across N goroutines, with a single
// goroutine collecting results to keep graph mutation single-threaded.
func (s *Scanner) analyzeParallel(ctx context.Context, toAnalyze []fileWork, workers int, state *ScanState, graph *model.ArchGraph, stats *ScanStats, maxNodes int) bool {
	workCh := make(chan fileWork, len(toAnalyze))
	resultCh := make(chan analyzeResult, len(toAnalyze))

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go s.worker(ctx, &wg, workCh, resultCh)
	}
	for _, f := range toAnalyze {
		workCh <- f
	}
	close(workCh)
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	truncated := false
	for r := range resultCh {
		stats.FilesAnalyzed++
		s.maybeUpdateState(state, r.path, r.nodes, r.edges)
		if !truncated && mergeNodesAndEdges(graph, r.nodes, r.edges, stats, maxNodes) {
			truncated = true
			// keep draining resultCh so workers can finish; max-nodes bail-out
			// must not deadlock
		}
	}
	return truncated
}

// worker is one fan-out goroutine: clones analyzers (for thread-safety) and
// drains workCh, posting analyzeResults to resultCh.
func (s *Scanner) worker(ctx context.Context, wg *sync.WaitGroup, workCh <-chan fileWork, resultCh chan<- analyzeResult) {
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
}

// mergeNodesAndEdges adds an analyzer's output into the graph, honoring maxNodes.
// Returns true if maxNodes stopped the merge mid-way.
func mergeNodesAndEdges(graph *model.ArchGraph, nodes []*model.Node, edges []*model.Edge, stats *ScanStats, maxNodes int) bool {
	for _, n := range nodes {
		if maxNodes > 0 && stats.NodesFound >= maxNodes {
			return true
		}
		if graph.AddNode(n) {
			stats.NodesFound++
		}
	}
	for _, e := range edges {
		graph.AddEdge(e)
		stats.EdgesFound++
	}
	return false
}

// maybeUpdateState writes fresh analyzer output into the persistent ScanState
// if state is non-nil; logs and continues on error.
func (s *Scanner) maybeUpdateState(state *ScanState, path string, nodes []*model.Node, edges []*model.Edge) {
	if state == nil {
		return
	}
	if err := state.UpdateFile(path, nodes, edges); err != nil {
		s.logger.Warn("State update error", "path", path, "error", err)
	}
}

// SupportedExtensions returns all file extensions the scanner handles.
func (s *Scanner) SupportedExtensions() []string {
	exts := make([]string, 0, len(s.analyzers))
	for ext := range s.analyzers {
		exts = append(exts, ext)
	}
	return exts
}
