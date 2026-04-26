package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/golang"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/markdown"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/python"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/typescript"
	"github.com/olgasafonova/code-to-arch-mcp/internal/detector"
	"github.com/olgasafonova/code-to-arch-mcp/internal/drift"
	"github.com/olgasafonova/code-to-arch-mcp/internal/infra"
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/registry"
	"github.com/olgasafonova/code-to-arch-mcp/internal/render"
	"github.com/olgasafonova/code-to-arch-mcp/internal/safepath"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

// HandlerRegistry holds the state and dependencies for all tool handlers.
type HandlerRegistry struct {
	scanner      *scanner.Scanner
	cache        *infra.Cache[*scanner.ScanResult]
	repoRegistry *registry.Registry
	logger       *slog.Logger
}

// NewHandlerRegistry creates a registry with all dependencies wired.
func NewHandlerRegistry(logger *slog.Logger) *HandlerRegistry {
	goAnalyzer := golang.New()
	tsAnalyzer := typescript.New()
	pyAnalyzer := python.New()
	mdAnalyzer := markdown.New()
	s := scanner.New(logger, goAnalyzer, tsAnalyzer, pyAnalyzer, mdAnalyzer)

	reg, err := registry.Load()
	if err != nil {
		logger.Warn("Failed to load repo registry, starting empty", "error", err)
		reg = nil
	}

	return &HandlerRegistry{
		scanner:      s,
		cache:        infra.NewCache[*scanner.ScanResult](5*time.Minute, 10),
		repoRegistry: reg,
		logger:       logger,
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
		case "ArchBlastRadius":
			register(h, server, spec, h.archBlastRadius)
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
		case "ArchMetrics":
			register(h, server, spec, h.archMetrics)
		case "ArchExplain":
			register(h, server, spec, h.archExplain)
		case "ArchRecommend":
			register(h, server, spec, h.archRecommend)
		case "ArchRegistryAdd":
			register(h, server, spec, h.archRegistryAdd)
		case "ArchRegistryRemove":
			register(h, server, spec, h.archRegistryRemove)
		case "ArchRegistryList":
			register(h, server, spec, h.archRegistryList)
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
	} else {
		opts.Timeout = 120 * time.Second
	}
	if sc.Workers > 0 {
		opts.Workers = min(sc.Workers, 32)
	}
	opts.SkipDirs = sc.SkipDirs
	opts.SkipGlobs = sc.SkipGlobs
	return opts
}

// resolveRepoPath resolves a path from either an explicit path or a registry alias.
// Path takes precedence over repo. Returns the resolved path, alias (empty for ad-hoc), and error.
func (h *HandlerRegistry) resolveRepoPath(path, repo string) (string, string, error) {
	if path != "" {
		return path, "", nil
	}
	if repo == "" {
		return "", "", fmt.Errorf("either path or repo is required")
	}
	if h.repoRegistry == nil {
		return "", "", fmt.Errorf("repo registry not available")
	}
	entry, err := h.repoRegistry.Get(repo)
	if err != nil {
		return "", "", err
	}
	return entry.Path, repo, nil
}

// cachedScan checks the cache before running a full scan.
// When alias is non-empty, loads/saves incremental scan state and updates registry metadata.
// Returns the result even on ErrLimitReached (partial result).
func (h *HandlerRegistry) cachedScan(ctx context.Context, path, alias string, opts scanner.ScanOptions) (*scanner.ScanResult, error) {
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

	// Load persistent scan state when scanning via registry alias.
	if alias != "" && h.repoRegistry != nil {
		statePath := h.repoRegistry.StatePath(alias)
		state, loadErr := infra.LoadJSON[scanner.ScanState](statePath)
		if loadErr != nil {
			if !os.IsNotExist(loadErr) {
				h.logger.Warn("Failed to load scan state, starting fresh", "alias", alias, "error", loadErr)
			}
			state = scanner.NewScanState(absPath)
		}
		opts.State = state
	}

	result, err := h.scanner.ScanWithOptions(ctx, path, opts)
	if err != nil && !errors.Is(err, scanner.ErrLimitReached) {
		return nil, err
	}

	h.cache.Put(key, result)

	// Persist state and update registry when scanning via alias.
	if alias != "" && h.repoRegistry != nil && result.State != nil {
		statePath := h.repoRegistry.StatePath(alias)
		if saveErr := infra.SaveJSON(statePath, result.State); saveErr != nil {
			h.logger.Warn("Failed to save scan state", "alias", alias, "error", saveErr)
		}
		h.repoRegistry.UpdateScanInfo(alias, result.Graph.NodeCount(), result.Graph.EdgeCount(), string(result.Graph.Topology))
		if saveErr := h.repoRegistry.Save(); saveErr != nil {
			h.logger.Warn("Failed to save registry", "error", saveErr)
		}
	}

	return result, err
}

// scanPath runs a cached scan and unwraps the graph.
// Returns the graph even on ErrLimitReached (partial result).
func (h *HandlerRegistry) scanPath(ctx context.Context, path, alias string, sc ScanControl) (*model.ArchGraph, bool, error) {
	result, err := h.cachedScan(ctx, path, alias, sc.toScanOptions())
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
	Repo   string `json:"repo,omitempty"`
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
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	result, err := h.cachedScan(ctx, path, alias, args.toScanOptions())
	if err != nil && !errors.Is(err, scanner.ErrLimitReached) {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}
	graph := result.Graph

	// Detect topology from project structure
	boundaries, bErr := detector.DetectBoundaries(path)
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
	Repo string `json:"repo,omitempty"`
	ScanControl
}

func (h *HandlerRegistry) archFocus(ctx context.Context, args ArchFocusArgs) (*ArchScanResult, error) {
	return h.archScan(ctx, ArchScanArgs{Path: args.Path, Repo: args.Repo, ScanControl: args.ScanControl})
}

// ArchGenerateArgs are the arguments for arch_generate.
type ArchGenerateArgs struct {
	Path           string  `json:"path"`
	Repo           string  `json:"repo,omitempty"`
	Format         string  `json:"format,omitempty"`
	ViewLevel      string  `json:"view_level,omitempty"`
	Title          string  `json:"title,omitempty"`
	Direction      string  `json:"direction,omitempty"`
	ThemeBG        string  `json:"theme_bg,omitempty"`
	ThemeFG        string  `json:"theme_fg,omitempty"`
	PruneThreshold float64 `json:"prune_threshold,omitempty"`
	MinDegree      int     `json:"min_degree,omitempty"`
	ScanControl
}

// ArchGenerateResult is the result of arch_generate.
type ArchGenerateResult struct {
	Format      string   `json:"format"`
	Diagram     string   `json:"diagram"`
	Summary     string   `json:"summary"`
	PrunedNodes []string `json:"pruned_nodes,omitempty"`
}

func (h *HandlerRegistry) archGenerate(ctx context.Context, args ArchGenerateArgs) (*ArchGenerateResult, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	opts := render.DefaultOptions()
	if args.Format != "" {
		opts.Format = render.Format(args.Format)
	}
	if args.ViewLevel != "" {
		opts.ViewLevel = render.ViewLevel(args.ViewLevel)
	} else if opts.Format == render.FormatHTML {
		// Human-facing HTML defaults to the full component view; the
		// container default produces near-empty output for Go MCP
		// servers (packages and endpoints, no service-type nodes).
		opts.ViewLevel = render.ViewComponent
	}
	if args.Title != "" {
		opts.Title = args.Title
	}
	if args.Direction != "" {
		opts.Direction = args.Direction
	}
	if args.ThemeBG != "" || args.ThemeFG != "" {
		opts.Theme = render.Theme{BG: args.ThemeBG, FG: args.ThemeFG}
	}
	if args.PruneThreshold > 0 {
		opts.PruneThreshold = args.PruneThreshold
	}
	if args.MinDegree > 0 {
		opts.MinDegree = args.MinDegree
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
	case render.FormatHTML:
		diagram = render.HTML(graph, opts)
	default:
		return nil, fmt.Errorf("unsupported format: %s (supported: mermaid, plantuml, c4, structurizr, json, drawio, excalidraw, html)", args.Format)
	}

	// Report which nodes were pruned (if any).
	var prunedNodes []string
	if opts.PruneThreshold > 0 {
		vg := render.PrepareGraph(graph, opts)
		prunedNodes = vg.PrunedNodes
	}

	return &ArchGenerateResult{
		Format:      string(opts.Format),
		Diagram:     diagram,
		Summary:     graph.Summary(),
		PrunedNodes: prunedNodes,
	}, nil
}

// ArchDependenciesArgs are the arguments for arch_dependencies.
type ArchDependenciesArgs struct {
	Path string `json:"path"`
	Repo string `json:"repo,omitempty"`
	ScanControl
}

// ArchDependenciesResult is the result of arch_dependencies.
type ArchDependenciesResult struct {
	Internal       []string `json:"internal"`
	External       []string `json:"external"`
	Infrastructure []string `json:"infrastructure"`
}

func (h *HandlerRegistry) archDependencies(ctx context.Context, args ArchDependenciesArgs) (*ArchDependenciesResult, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	modulePath := readModulePath(path)

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

		if isStdlib(target, modulePath) {
			continue // skip stdlib
		}
		external = append(external, target)
	}

	// Internal: package nodes
	for _, n := range graph.NodesByType(model.NodePackage) {
		internal = append(internal, n.Name)
	}

	// Ensure non-nil slices so JSON serializes as [] not null.
	if internal == nil {
		internal = []string{}
	}
	if external == nil {
		external = []string{}
	}
	if infra == nil {
		infra = []string{}
	}

	return &ArchDependenciesResult{
		Internal:       internal,
		External:       external,
		Infrastructure: infra,
	}, nil
}

// ArchBlastRadiusArgs are the arguments for arch_blast_radius.
type ArchBlastRadiusArgs struct {
	Path     string `json:"path"`
	Repo     string `json:"repo,omitempty"`
	Target   string `json:"target"`
	MaxDepth int    `json:"max_depth,omitempty"`
	ScanControl
}

// ArchBlastRadiusResult is the result of arch_blast_radius.
type ArchBlastRadiusResult struct {
	Target      string                      `json:"target"`
	TargetID    string                      `json:"target_id"`
	Direct      int                         `json:"direct"`
	Total       int                         `json:"total"`
	MaxDepthHit bool                        `json:"max_depth_hit"`
	Dependents  []detector.BlastRadiusEntry `json:"dependents"`
}

func (h *HandlerRegistry) archBlastRadius(ctx context.Context, args ArchBlastRadiusArgs) (*ArchBlastRadiusResult, error) {
	if args.Target == "" {
		return nil, fmt.Errorf("target is required")
	}

	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	targetID, ok := detector.ResolveTargetToID(graph, args.Target)
	if !ok {
		return nil, fmt.Errorf("target %q not found in scanned graph (run arch_scan to list available node IDs and paths)", args.Target)
	}

	res := detector.ComputeBlastRadius(graph, targetID, args.MaxDepth)
	if res.Dependents == nil {
		res.Dependents = []detector.BlastRadiusEntry{}
	}

	return &ArchBlastRadiusResult{
		Target:      args.Target,
		TargetID:    res.TargetID,
		Direct:      res.Direct,
		Total:       res.Total,
		MaxDepthHit: res.MaxDepthHit,
		Dependents:  res.Dependents,
	}, nil
}

// readModulePath extracts the module declaration from go.mod in the given directory.
// Returns "" if go.mod doesn't exist or can't be parsed.
func readModulePath(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func isStdlib(importPath, modulePath string) bool {
	// If it belongs to the scanned module, it's internal — not stdlib
	if modulePath != "" && strings.HasPrefix(importPath, modulePath) {
		return false
	}
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
	Repo string `json:"repo,omitempty"`
	ScanControl
}

type ArchDataflowResult struct {
	Endpoints []string             `json:"endpoints"`
	DataPaths []string             `json:"data_paths"`
	Traces    []model.ProcessTrace `json:"traces"`
	Summary   string               `json:"summary"`
}

func (h *HandlerRegistry) archDataflow(ctx context.Context, args ArchDataflowArgs) (*ArchDataflowResult, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
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

	if endpoints == nil {
		endpoints = []string{}
	}
	if dataPaths == nil {
		dataPaths = []string{}
	}

	traces := detector.ComputeTraces(graph)

	return &ArchDataflowResult{
		Endpoints: endpoints,
		DataPaths: dataPaths,
		Traces:    traces,
		Summary:   fmt.Sprintf("Found %d endpoints, %d data paths, %d process traces", len(endpoints), len(dataPaths), len(traces)),
	}, nil
}

type ArchBoundariesArgs struct {
	Path string `json:"path"`
	Repo string `json:"repo,omitempty"`
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
	path, _, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	result, err := detector.DetectBoundaries(path)
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

	if boundaries == nil {
		boundaries = []BoundaryInfo{}
	}

	return &ArchBoundariesResult{
		Topology:   string(result.Topology),
		Boundaries: boundaries,
		Summary:    fmt.Sprintf("Detected %s topology with %d boundaries", result.Topology, len(boundaries)),
	}, nil
}

type ArchDiffArgs struct {
	Path         string `json:"path"`
	Repo         string `json:"repo,omitempty"`
	SnapshotFile string `json:"snapshot_file"`
}

func (h *HandlerRegistry) archDiff(ctx context.Context, args ArchDiffArgs) (*model.DiffReport, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}
	if args.SnapshotFile == "" {
		return nil, fmt.Errorf("snapshot_file is required")
	}
	if err := safepath.ValidateOutputPath(args.SnapshotFile, path); err != nil {
		return nil, fmt.Errorf("invalid snapshot file path: %w", err)
	}

	// Load baseline from snapshot
	snapshot, err := drift.Load(args.SnapshotFile)
	if err != nil {
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}
	baseline := snapshot.ToGraph()

	// Scan current codebase
	result, scanErr := h.cachedScan(ctx, path, alias, scanner.DefaultScanOptions())
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
	Repo    string `json:"repo,omitempty"`
	BaseRef string `json:"base_ref"`
	HeadRef string `json:"head_ref,omitempty"`
}

func (h *HandlerRegistry) archDrift(ctx context.Context, args ArchDriftArgs) (*model.DiffReport, error) {
	path, _, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
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
	basePath, baseCleanup, err := drift.CheckoutRef(ctx, path, args.BaseRef)
	if err != nil {
		return nil, fmt.Errorf("checking out base ref %s: %w", args.BaseRef, err)
	}
	defer baseCleanup()

	baseGraph, err := h.scanner.Scan(basePath)
	if err != nil {
		return nil, fmt.Errorf("scanning base ref: %w", err)
	}

	// Checkout and scan head ref
	headPath, headCleanup, err := drift.CheckoutRef(ctx, path, headRef)
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
	Repo string `json:"repo,omitempty"`
	ScanControl
}

type ArchValidateResult struct {
	Valid      bool     `json:"valid"`
	Violations []string `json:"violations"`
	Summary    string   `json:"summary"`
}

func (h *HandlerRegistry) archValidate(ctx context.Context, args ArchValidateArgs) (*ArchValidateResult, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	// Load custom rules if .arch-rules.yaml exists in the project
	var customRules *detector.RulesConfig
	rulesPath := filepath.Join(path, ".arch-rules.yaml")
	if _, err := os.Stat(rulesPath); err == nil {
		customRules, err = detector.LoadRules(rulesPath)
		if err != nil {
			h.logger.Warn("Failed to load custom rules", "path", rulesPath, "error", err)
		}
	}

	detectedViolations := detector.ValidateGraph(graph, customRules)
	violations := make([]string, 0, len(detectedViolations))
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
	Repo  string `json:"repo,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type ArchHistoryResult struct {
	Entries []drift.HistoryEntry `json:"entries"`
	Summary string               `json:"summary"`
}

// scanResult holds the output of scanning a single commit in parallel.
type scanResult struct {
	graph *model.ArchGraph
	entry drift.HistoryEntry
}

func (h *HandlerRegistry) archHistory(ctx context.Context, args ArchHistoryArgs) (*ArchHistoryResult, error) {
	path, _, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}

	commits, err := drift.GetSignificantCommits(ctx, path, limit)
	if err != nil {
		return nil, fmt.Errorf("getting git history: %w", err)
	}

	// Phase 1: Parallel scan — each goroutine writes only to its own index.
	results := make([]scanResult, len(commits))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4) // cap concurrent git worktrees

	for idx, c := range commits {
		results[idx].entry = drift.HistoryEntry{
			Ref:     c.Hash[:8],
			Date:    c.Date,
			Message: c.Message,
		}
		g.Go(func() error {
			worktree, cleanup, wErr := drift.CheckoutRef(gctx, path, c.Hash)
			if wErr != nil {
				return nil // non-fatal: entry stays with zero counts
			}
			graph, scanErr := h.scanner.Scan(worktree)
			cleanup()
			if scanErr != nil {
				return nil // non-fatal
			}
			results[idx].graph = graph
			results[idx].entry.NodeCount = graph.NodeCount()
			results[idx].entry.EdgeCount = graph.EdgeCount()
			results[idx].entry.Topology = string(graph.Topology)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("scanning commits: %w", err)
	}

	// Phase 2: Sequential comparison (oldest-first).
	// commits[0] is most recent; walk from end to start.
	var prevGraph *model.ArchGraph
	for i := len(results) - 1; i >= 0; i-- {
		r := &results[i]
		if r.graph != nil && prevGraph != nil {
			report := drift.Compare(prevGraph, r.graph)
			r.entry.ChangesFromPrevious = len(report.Changes)
		}
		if r.graph != nil {
			prevGraph = r.graph
		}
	}

	// Collect entries in most-recent-first order (same order as commits).
	entries := make([]drift.HistoryEntry, len(results))
	for i, r := range results {
		entries[i] = r.entry
	}

	return &ArchHistoryResult{
		Entries: entries,
		Summary: fmt.Sprintf("Analyzed %d commits", len(entries)),
	}, nil
}

type ArchSnapshotArgs struct {
	Path       string `json:"path"`
	Repo       string `json:"repo,omitempty"`
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
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	outFile := args.OutputFile
	if outFile == "" {
		outFile = filepath.Join(path, "architecture.snapshot.json")
	}
	if err := safepath.ValidateOutputPath(outFile, path); err != nil {
		return nil, fmt.Errorf("invalid output file path: %w", err)
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

type ArchMetricsArgs struct {
	Path string `json:"path"`
	Repo string `json:"repo,omitempty"`
	ScanControl
}

type ArchMetricsResult struct {
	*detector.Metrics
	Summary string `json:"summary"`
}

func (h *HandlerRegistry) archMetrics(ctx context.Context, args ArchMetricsArgs) (*ArchMetricsResult, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	metrics := detector.ComputeMetrics(graph)

	return &ArchMetricsResult{
		Metrics: metrics,
		Summary: fmt.Sprintf("Analyzed %d components: avg coupling %.1f, avg instability %.2f, max depth %d",
			graph.NodeCount(), metrics.AvgCoupling, metrics.AvgInstability, metrics.MaxDepth),
	}, nil
}

type ArchExplainArgs struct {
	Path     string `json:"path"`
	Repo     string `json:"repo,omitempty"`
	Question string `json:"question,omitempty"`
	ScanControl
}

type ArchExplainResult struct {
	Explanation string   `json:"explanation"`
	Evidence    []string `json:"evidence"`
}

func (h *HandlerRegistry) archExplain(ctx context.Context, args ArchExplainArgs) (*ArchExplainResult, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	boundaries, _ := detector.DetectBoundaries(path)
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

type ArchRecommendArgs struct {
	Path  string `json:"path"`
	Repo  string `json:"repo,omitempty"`
	Focus string `json:"focus,omitempty"` // filter by recommendation category
	ScanControl
}

type ArchRecommendResult struct {
	Recommendations []detector.Recommendation `json:"recommendations"`
	Summary         string                    `json:"summary"`
	MetricsSnapshot *detector.Metrics         `json:"metrics_snapshot"`
}

func (h *HandlerRegistry) archRecommend(ctx context.Context, args ArchRecommendArgs) (*ArchRecommendResult, error) {
	path, alias, err := h.resolveRepoPath(args.Path, args.Repo)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateScanPath(path); err != nil {
		return nil, err
	}

	graph, _, err := h.scanPath(ctx, path, alias, args.ScanControl)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	// Load custom rules if present.
	var customRules *detector.RulesConfig
	rulesPath := filepath.Join(path, ".arch-rules.yaml")
	if _, statErr := os.Stat(rulesPath); statErr == nil {
		customRules, _ = detector.LoadRules(rulesPath)
	}

	violations := detector.ValidateGraph(graph, customRules)
	metrics := detector.ComputeMetrics(graph)
	boundaries, _ := detector.DetectBoundaries(args.Path)
	explanation := detector.ExplainArchitecture(graph, boundaries)

	recs := detector.RecommendArchitecture(graph, violations, metrics, explanation)

	// Filter by category if focus is set.
	if args.Focus != "" {
		filtered := make([]detector.Recommendation, 0, len(recs))
		for _, r := range recs {
			if r.Category == args.Focus {
				filtered = append(filtered, r)
			}
		}
		recs = filtered
	}

	if recs == nil {
		recs = []detector.Recommendation{}
	}

	return &ArchRecommendResult{
		Recommendations: recs,
		Summary:         fmt.Sprintf("Generated %d recommendations for %s", len(recs), path),
		MetricsSnapshot: metrics,
	}, nil
}

// =============================================================================
// Registry handlers
// =============================================================================

type ArchRegistryAddArgs struct {
	Path  string `json:"path"`
	Alias string `json:"alias,omitempty"`
}

type ArchRegistryAddResult struct {
	Alias   string `json:"alias"`
	Path    string `json:"path"`
	Summary string `json:"summary"`
}

func (h *HandlerRegistry) archRegistryAdd(_ context.Context, args ArchRegistryAddArgs) (*ArchRegistryAddResult, error) {
	if h.repoRegistry == nil {
		return nil, fmt.Errorf("repo registry not available")
	}
	if err := safepath.ValidateScanPath(args.Path); err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(args.Path)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}

	alias := args.Alias
	if alias == "" {
		alias = filepath.Base(absPath)
	}

	if err := h.repoRegistry.Add(alias, absPath); err != nil {
		return nil, err
	}
	if err := h.repoRegistry.Save(); err != nil {
		return nil, fmt.Errorf("saving registry: %w", err)
	}

	return &ArchRegistryAddResult{
		Alias:   alias,
		Path:    absPath,
		Summary: fmt.Sprintf("Registered %q -> %s", alias, absPath),
	}, nil
}

type ArchRegistryRemoveArgs struct {
	Alias string `json:"alias"`
}

type ArchRegistryRemoveResult struct {
	Alias   string `json:"alias"`
	Summary string `json:"summary"`
}

func (h *HandlerRegistry) archRegistryRemove(_ context.Context, args ArchRegistryRemoveArgs) (*ArchRegistryRemoveResult, error) {
	if h.repoRegistry == nil {
		return nil, fmt.Errorf("repo registry not available")
	}
	if args.Alias == "" {
		return nil, fmt.Errorf("alias is required")
	}

	if err := h.repoRegistry.Remove(args.Alias); err != nil {
		return nil, err
	}
	if err := h.repoRegistry.Save(); err != nil {
		return nil, fmt.Errorf("saving registry: %w", err)
	}

	return &ArchRegistryRemoveResult{
		Alias:   args.Alias,
		Summary: fmt.Sprintf("Removed %q from registry", args.Alias),
	}, nil
}

type ArchRegistryListArgs struct{}

type ArchRegistryListResult struct {
	Repos   []registry.RepoEntry `json:"repos"`
	Summary string               `json:"summary"`
}

func (h *HandlerRegistry) archRegistryList(_ context.Context, _ ArchRegistryListArgs) (*ArchRegistryListResult, error) {
	if h.repoRegistry == nil {
		return &ArchRegistryListResult{
			Repos:   []registry.RepoEntry{},
			Summary: "No repos registered (registry not available)",
		}, nil
	}

	entries := h.repoRegistry.List()
	if entries == nil {
		entries = []registry.RepoEntry{}
	}

	return &ArchRegistryListResult{
		Repos:   entries,
		Summary: fmt.Sprintf("%d repos registered", len(entries)),
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
