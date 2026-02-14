package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/golang"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/python"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/typescript"
	"github.com/olgasafonova/code-to-arch-mcp/internal/detector"
	"github.com/olgasafonova/code-to-arch-mcp/internal/drift"
	"github.com/olgasafonova/code-to-arch-mcp/internal/infra"
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/render"
	"github.com/olgasafonova/code-to-arch-mcp/internal/safepath"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

// HandlerRegistry holds the state and dependencies for all tool handlers.
type HandlerRegistry struct {
	scanner *scanner.Scanner
	cache   *infra.Cache[*scanner.ScanResult]
	logger  *slog.Logger
}

// NewHandlerRegistry creates a registry with all dependencies wired.
func NewHandlerRegistry(logger *slog.Logger) *HandlerRegistry {
	goAnalyzer := golang.New()
	tsAnalyzer := typescript.New()
	pyAnalyzer := python.New()
	s := scanner.New(logger, goAnalyzer, tsAnalyzer, pyAnalyzer)

	return &HandlerRegistry{
		scanner: s,
		cache:   infra.NewCache[*scanner.ScanResult](30*time.Second, 10),
		logger:  logger,
	}
}

// RegisterAll registers all tools with the MCP server.
func (h *HandlerRegistry) RegisterAll(server *mcp.Server) {
	for _, spec := range AllTools {
		switch spec.Method {
		case "ArchScan":
			register(h, server, spec, h.archScan)
		case "ArchFocus":
			register(h, server, spec, h.archFocus)
		case "ArchGenerate":
			register(h, server, spec, h.archGenerate)
		case "ArchDependencies":
			register(h, server, spec, h.archDependencies)
		case "ArchDataflow":
			register(h, server, spec, h.archDataflow)
		case "ArchBoundaries":
			register(h, server, spec, h.archBoundaries)
		case "ArchDiff":
			register(h, server, spec, h.archDiff)
		case "ArchDrift":
			register(h, server, spec, h.archDrift)
		case "ArchValidate":
			register(h, server, spec, h.archValidate)
		case "ArchHistory":
			register(h, server, spec, h.archHistory)
		case "ArchSnapshot":
			register(h, server, spec, h.archSnapshot)
		case "ArchExplain":
			register(h, server, spec, h.archExplain)
		}
	}
}

// RegisteredTools returns the tool specs for introspection.
func (h *HandlerRegistry) RegisteredTools() []ToolSpec {
	return AllTools
}

// =============================================================================
// ScanControl — embedded in Args structs that trigger scans
// =============================================================================

// ScanControl contains optional fields to control scan behavior.
type ScanControl struct {
	MaxFiles    int      `json:"max_files,omitempty"`
	MaxNodes    int      `json:"max_nodes,omitempty"`
	TimeoutSecs int      `json:"timeout_secs,omitempty"`
	SkipDirs    []string `json:"skip_dirs,omitempty"`
	SkipGlobs   []string `json:"skip_globs,omitempty"`
	Workers     int      `json:"workers,omitempty"`
}

func (sc ScanControl) toScanOptions() scanner.ScanOptions {
	opts := scanner.DefaultScanOptions()
	if sc.MaxFiles > 0 {
		opts.MaxFiles = sc.MaxFiles
	}
	if sc.MaxNodes > 0 {
		opts.MaxNodes = sc.MaxNodes
	}
	if sc.TimeoutSecs > 0 {
		opts.Timeout = time.Duration(sc.TimeoutSecs) * time.Second
	}
	if sc.Workers > 0 {
		opts.Workers = sc.Workers
	}
	opts.SkipDirs = sc.SkipDirs
	opts.SkipGlobs = sc.SkipGlobs
	return opts
}

// cachedScan checks the cache before running a full scan.
// Returns the result even on ErrLimitReached (partial result).
func (h *HandlerRegistry) cachedScan(ctx context.Context, path string, opts scanner.ScanOptions) (*scanner.ScanResult, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	optsKey := fmt.Sprintf("%d|%d|%d|%s|%s",
		opts.MaxFiles, opts.MaxNodes, opts.Workers,
		strings.Join(opts.SkipDirs, ","),
		strings.Join(opts.SkipGlobs, ","),
	)
	key := infra.CacheKey(absPath, optsKey)

	if cached, ok := h.cache.Get(key); ok {
		h.logger.Debug("Scan cache hit", "path", absPath)
		return cached, nil
	}

	result, err := h.scanner.ScanWithOptions(ctx, path, opts)
	if err != nil && !errors.Is(err, scanner.ErrLimitReached) {
		return nil, err
	}

	h.cache.Put(key, result)
	return result, err
}

// scanPath runs a cached scan and unwraps the graph.
// Returns the graph even on ErrLimitReached (partial result).
func (h *HandlerRegistry) scanPath(ctx context.Context, path string, sc ScanControl) (*model.ArchGraph, bool, error) {
	result, err := h.cachedScan(ctx, path, sc.toScanOptions())
	if err != nil && !errors.Is(err, scanner.ErrLimitReached) {
		return nil, false, err
	}
	return result.Graph, result.Truncated, nil
}

// =============================================================================
// Handler implementations
// =============================================================================

// ArchScanArgs are the arguments for arch_scan.
type ArchScanArgs struct {
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"` // "summary" (default) or "full"
	ScanControl
}

// ArchScanResult is the result of arch_scan.
type ArchScanResult struct {
	RootPath  string             `json:"root_path"`
	Topology  string             `json:"topology"`
	NodeCount int                `json:"node_count"`
	EdgeCount int                `json:"edge_count"`
	Nodes     []*model.Node      `json:"nodes,omitempty"`
	Edges     []*model.Edge      `json:"edges,omitempty"`
	Summary   string             `json:"summary"`
	Stats     *scanner.ScanStats `json:"stats,omitempty"`
	Truncated bool               `json:"truncated,omitempty"`
}

func (h *HandlerRegistry) archScan(ctx context.Context, args ArchScanArgs) (*ArchScanResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	result, err := h.cachedScan(ctx, args.Path, args.toScanOptions())
	if err != nil && !errors.Is(err, scanner.ErrLimitReached) {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}
	graph := result.Graph

	// Detect topology from project structure
	boundaries, bErr := detector.DetectBoundaries(args.Path)
	if bErr == nil {
		graph.Topology = boundaries.Topology
	}

	res := &ArchScanResult{
		RootPath:  graph.RootPath,
		Topology:  string(graph.Topology),
		NodeCount: graph.NodeCount(),
		EdgeCount: graph.EdgeCount(),
		Summary:   graph.Summary(),
		Stats:     &result.Stats,
		Truncated: result.Truncated,
	}

	if strings.EqualFold(args.Detail, "full") {
		graph.RelativePaths()
		res.Nodes = graph.Nodes()
		res.Edges = graph.Edges()
	}

	return res, nil
}

// ArchFocusArgs are the arguments for arch_focus.
type ArchFocusArgs struct {
	Path string `json:"path"`
	ScanControl
}

func (h *HandlerRegistry) archFocus(ctx context.Context, args ArchFocusArgs) (*ArchScanResult, error) {
	return h.archScan(ctx, ArchScanArgs{Path: args.Path, ScanControl: args.ScanControl})
}

// ArchGenerateArgs are the arguments for arch_generate.
type ArchGenerateArgs struct {
	Path      string `json:"path"`
	Format    string `json:"format,omitempty"`
	ViewLevel string `json:"view_level,omitempty"`
	Title     string `json:"title,omitempty"`
	Direction string `json:"direction,omitempty"`
	ScanControl
}

// ArchGenerateResult is the result of arch_generate.
type ArchGenerateResult struct {
	Format  string `json:"format"`
	Diagram string `json:"diagram"`
	Summary string `json:"summary"`
}

func (h *HandlerRegistry) archGenerate(ctx context.Context, args ArchGenerateArgs) (*ArchGenerateResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, args.Path, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	opts := render.DefaultOptions()
	if args.Format != "" {
		opts.Format = render.Format(args.Format)
	}
	if args.ViewLevel != "" {
		opts.ViewLevel = render.ViewLevel(args.ViewLevel)
	}
	if args.Title != "" {
		opts.Title = args.Title
	}
	if args.Direction != "" {
		opts.Direction = args.Direction
	}

	var diagram string
	switch opts.Format {
	case render.FormatMermaid:
		diagram = render.Mermaid(graph, opts)
	case render.FormatPlantUML:
		diagram = render.PlantUML(graph, opts)
	case render.FormatC4:
		diagram = render.C4(graph, opts)
	case render.FormatStructurizr:
		diagram = render.Structurizr(graph, opts)
	case render.FormatJSON:
		diagram = render.JSON(graph, opts)
	case render.FormatDrawIO:
		diagram = render.DrawIO(graph, opts)
	case render.FormatExcalidraw:
		diagram = render.Excalidraw(graph, opts)
	default:
		return nil, fmt.Errorf("unsupported format: %s (supported: mermaid, plantuml, c4, structurizr, json, drawio, excalidraw)", args.Format)
	}

	return &ArchGenerateResult{
		Format:  string(opts.Format),
		Diagram: diagram,
		Summary: graph.Summary(),
	}, nil
}

// ArchDependenciesArgs are the arguments for arch_dependencies.
type ArchDependenciesArgs struct {
	Path string `json:"path"`
	ScanControl
}

// ArchDependenciesResult is the result of arch_dependencies.
type ArchDependenciesResult struct {
	Internal       []string `json:"internal"`
	External       []string `json:"external"`
	Infrastructure []string `json:"infrastructure"`
}

func (h *HandlerRegistry) archDependencies(ctx context.Context, args ArchDependenciesArgs) (*ArchDependenciesResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, args.Path, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	var internal, external, infra []string
	seen := make(map[string]bool)

	for _, e := range graph.Edges() {
		if e.Type != model.EdgeDependency {
			continue
		}
		target := e.Label
		if target == "" {
			target = e.Target
		}
		if seen[target] {
			continue
		}
		seen[target] = true

		// Classify: internal (starts with module path), external, infra
		node := graph.GetNode(e.Target)
		if node != nil {
			switch node.Type {
			case model.NodeDatabase, model.NodeQueue, model.NodeCache:
				infra = append(infra, target)
				continue
			}
		}

		// Simple heuristic: stdlib has no dots in first segment
		if isStdlib(target) {
			continue // skip stdlib
		}
		external = append(external, target)
	}

	// Internal: package nodes
	for _, n := range graph.NodesByType(model.NodePackage) {
		internal = append(internal, n.Name)
	}

	return &ArchDependenciesResult{
		Internal:       internal,
		External:       external,
		Infrastructure: infra,
	}, nil
}

func isStdlib(importPath string) bool {
	// Go stdlib packages don't contain dots in the first path segment.
	// Extended stdlib (golang.org/x/*) also excluded from external deps.
	first, _, _ := strings.Cut(importPath, "/")
	if !strings.Contains(first, ".") {
		return true
	}
	return strings.HasPrefix(importPath, "golang.org/x/")
}

type ArchDataflowArgs struct {
	Path string `json:"path"`
	ScanControl
}

type ArchDataflowResult struct {
	Endpoints []string `json:"endpoints"`
	DataPaths []string `json:"data_paths"`
	Summary   string   `json:"summary"`
}

func (h *HandlerRegistry) archDataflow(ctx context.Context, args ArchDataflowArgs) (*ArchDataflowResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, args.Path, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	var endpoints, dataPaths []string
	for _, n := range graph.NodesByType(model.NodeEndpoint) {
		endpoints = append(endpoints, n.Name)
	}
	for _, e := range graph.Edges() {
		if e.Type == model.EdgeDataFlow || e.Type == model.EdgeReadWrite {
			dataPaths = append(dataPaths, fmt.Sprintf("%s -> %s (%s)", e.Source, e.Target, e.Label))
		}
	}

	return &ArchDataflowResult{
		Endpoints: endpoints,
		DataPaths: dataPaths,
		Summary:   fmt.Sprintf("Found %d endpoints and %d data paths", len(endpoints), len(dataPaths)),
	}, nil
}

type ArchBoundariesArgs struct {
	Path string `json:"path"`
}

type BoundaryInfo struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Type    string   `json:"type"`
	Markers []string `json:"markers"`
}

type ArchBoundariesResult struct {
	Topology   string         `json:"topology"`
	Boundaries []BoundaryInfo `json:"boundaries"`
	Summary    string         `json:"summary"`
}

func (h *HandlerRegistry) archBoundaries(_ context.Context, args ArchBoundariesArgs) (*ArchBoundariesResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	result, err := detector.DetectBoundaries(args.Path)
	if err != nil {
		return nil, fmt.Errorf("detecting boundaries: %w", err)
	}

	var boundaries []BoundaryInfo
	for _, b := range result.Boundaries {
		boundaries = append(boundaries, BoundaryInfo{
			Name:    b.Name,
			Path:    b.Path,
			Type:    b.Type,
			Markers: b.Markers,
		})
	}

	return &ArchBoundariesResult{
		Topology:   string(result.Topology),
		Boundaries: boundaries,
		Summary:    fmt.Sprintf("Detected %s topology with %d boundaries", result.Topology, len(boundaries)),
	}, nil
}

type ArchDiffArgs struct {
	Path         string `json:"path"`
	SnapshotFile string `json:"snapshot_file"`
}

func (h *HandlerRegistry) archDiff(ctx context.Context, args ArchDiffArgs) (*model.DiffReport, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}
	if args.SnapshotFile == "" {
		return nil, fmt.Errorf("snapshot_file is required")
	}

	// Load baseline from snapshot
	snapshot, err := drift.Load(args.SnapshotFile)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}
	baseline := snapshot.ToGraph()

	// Scan current codebase
	result, scanErr := h.cachedScan(ctx, args.Path, scanner.DefaultScanOptions())
	if scanErr != nil && !errors.Is(scanErr, scanner.ErrLimitReached) {
		return nil, fmt.Errorf("scanning current codebase: %w", scanErr)
	}

	report := drift.Compare(baseline, result.Graph)
	report.BaseRef = args.SnapshotFile
	report.CompareRef = "current"
	return report, nil
}

type ArchDriftArgs struct {
	Path    string `json:"path"`
	BaseRef string `json:"base_ref"`
	HeadRef string `json:"head_ref,omitempty"`
}

func (h *HandlerRegistry) archDrift(ctx context.Context, args ArchDriftArgs) (*model.DiffReport, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}
	if args.BaseRef == "" {
		return nil, fmt.Errorf("base_ref is required")
	}
	headRef := args.HeadRef
	if headRef == "" {
		headRef = "HEAD"
	}

	// Checkout and scan base ref
	basePath, baseCleanup, err := drift.CheckoutRef(ctx, args.Path, args.BaseRef)
	if err != nil {
		return nil, fmt.Errorf("checking out base ref %s: %w", args.BaseRef, err)
	}
	defer baseCleanup()

	baseGraph, err := h.scanner.Scan(basePath)
	if err != nil {
		return nil, fmt.Errorf("scanning base ref: %w", err)
	}

	// Checkout and scan head ref
	headPath, headCleanup, err := drift.CheckoutRef(ctx, args.Path, headRef)
	if err != nil {
		return nil, fmt.Errorf("checking out head ref %s: %w", headRef, err)
	}
	defer headCleanup()

	headGraph, err := h.scanner.Scan(headPath)
	if err != nil {
		return nil, fmt.Errorf("scanning head ref: %w", err)
	}

	report := drift.Compare(baseGraph, headGraph)
	report.BaseRef = args.BaseRef
	report.CompareRef = headRef
	return report, nil
}

type ArchValidateArgs struct {
	Path string `json:"path"`
	ScanControl
}

type ArchValidateResult struct {
	Valid      bool     `json:"valid"`
	Violations []string `json:"violations"`
	Summary    string   `json:"summary"`
}

func (h *HandlerRegistry) archValidate(ctx context.Context, args ArchValidateArgs) (*ArchValidateResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, args.Path, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	detectedViolations := detector.ValidateGraph(graph)
	var violations []string
	for _, v := range detectedViolations {
		violations = append(violations, fmt.Sprintf("[%s] %s: %s", v.Severity, v.Rule, v.Detail))
	}

	return &ArchValidateResult{
		Valid:      len(violations) == 0,
		Violations: violations,
		Summary:    fmt.Sprintf("Validation complete: %d violations found", len(violations)),
	}, nil
}

type ArchHistoryArgs struct {
	Path  string `json:"path"`
	Limit int    `json:"limit,omitempty"`
}

type ArchHistoryResult struct {
	Entries []drift.HistoryEntry `json:"entries"`
	Summary string               `json:"summary"`
}

func (h *HandlerRegistry) archHistory(ctx context.Context, args ArchHistoryArgs) (*ArchHistoryResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}

	commits, err := drift.GetSignificantCommits(ctx, args.Path, limit)
	if err != nil {
		return nil, fmt.Errorf("getting git history: %w", err)
	}

	var entries []drift.HistoryEntry
	var prevGraph *model.ArchGraph

	// Walk commits in reverse (oldest first) to compare sequentially
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		worktree, cleanup, err := drift.CheckoutRef(ctx, args.Path, c.Hash)
		if err != nil {
			entries = append(entries, drift.HistoryEntry{
				Ref:     c.Hash[:8],
				Date:    c.Date,
				Message: c.Message,
			})
			continue
		}

		graph, scanErr := h.scanner.Scan(worktree)
		cleanup()

		if scanErr != nil {
			entries = append(entries, drift.HistoryEntry{
				Ref:     c.Hash[:8],
				Date:    c.Date,
				Message: c.Message,
			})
			continue
		}

		entry := drift.HistoryEntry{
			Ref:       c.Hash[:8],
			Date:      c.Date,
			Message:   c.Message,
			NodeCount: graph.NodeCount(),
			EdgeCount: graph.EdgeCount(),
			Topology:  string(graph.Topology),
		}

		if prevGraph != nil {
			report := drift.Compare(prevGraph, graph)
			entry.ChangesFromPrevious = len(report.Changes)
		}

		entries = append(entries, entry)
		prevGraph = graph
	}

	// Reverse back to most-recent-first order
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return &ArchHistoryResult{
		Entries: entries,
		Summary: fmt.Sprintf("Analyzed %d commits", len(entries)),
	}, nil
}

type ArchSnapshotArgs struct {
	Path       string `json:"path"`
	OutputFile string `json:"output_file,omitempty"`
	Label      string `json:"label,omitempty"`
	ScanControl
}

type ArchSnapshotResult struct {
	File      string `json:"file"`
	NodeCount int    `json:"node_count"`
	EdgeCount int    `json:"edge_count"`
	Summary   string `json:"summary"`
}

func (h *HandlerRegistry) archSnapshot(ctx context.Context, args ArchSnapshotArgs) (*ArchSnapshotResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, args.Path, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	outFile := args.OutputFile
	if outFile == "" {
		outFile = filepath.Join(args.Path, "architecture.snapshot.json")
	}

	snap, err := drift.Save(graph, outFile, args.Label)
	if err != nil {
		return nil, fmt.Errorf("saving snapshot: %w", err)
	}

	return &ArchSnapshotResult{
		File:      outFile,
		NodeCount: len(snap.Nodes),
		EdgeCount: len(snap.Edges),
		Summary:   fmt.Sprintf("Saved snapshot with %d nodes and %d edges to %s", len(snap.Nodes), len(snap.Edges), outFile),
	}, nil
}

type ArchExplainArgs struct {
	Path     string `json:"path"`
	Question string `json:"question,omitempty"`
	ScanControl
}

type ArchExplainResult struct {
	Explanation string   `json:"explanation"`
	Evidence    []string `json:"evidence"`
}

func (h *HandlerRegistry) archExplain(ctx context.Context, args ArchExplainArgs) (*ArchExplainResult, error) {
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, args.Path, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	boundaries, _ := detector.DetectBoundaries(args.Path)
	explanation := detector.ExplainArchitecture(graph, boundaries)

	// Build evidence from patterns, decisions, and risks
	var evidence []string
	evidence = append(evidence, "Topology: "+explanation.TopologyReason)
	for _, p := range explanation.Patterns {
		evidence = append(evidence, "Pattern: "+p)
	}
	for _, d := range explanation.KeyDecisions {
		evidence = append(evidence, "Decision: "+d)
	}
	for _, r := range explanation.Risks {
		evidence = append(evidence, "Risk: "+r)
	}

	return &ArchExplainResult{
		Explanation: explanation.Summary,
		Evidence:    evidence,
	}, nil
}

// =============================================================================
// Generic registration helper
// =============================================================================

func register[Args, Result any](
	h *HandlerRegistry,
	server *mcp.Server,
	spec ToolSpec,
	handler func(context.Context, Args) (Result, error),
) {
	tool := &mcp.Tool{
		Name:        spec.Name,
		Description: spec.Description,
		Annotations: &mcp.ToolAnnotations{
			Title:          spec.Title,
			ReadOnlyHint:   spec.ReadOnly,
			IdempotentHint: spec.Idempotent,
		},
	}
	if spec.OpenWorld {
		tool.Annotations.OpenWorldHint = ptr(true)
	}

	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, args Args) (callResult *mcp.CallToolResult, result Result, retErr error) {
		defer func() {
			if r := recover(); r != nil {
				var zero Result
				result = zero
				retErr = fmt.Errorf("%s panicked: %v", spec.Name, r)
			}
		}()

		res, err := handler(ctx, args)
		if err != nil {
			var zero Result
			return nil, zero, fmt.Errorf("%s failed: %w", spec.Name, err)
		}
		return nil, res, nil
	})
}
