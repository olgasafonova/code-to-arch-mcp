package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/golang"
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/render"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

// HandlerRegistry holds the state and dependencies for all tool handlers.
type HandlerRegistry struct {
	scanner *scanner.Scanner
	logger  *slog.Logger
}

// NewHandlerRegistry creates a registry with all dependencies wired.
func NewHandlerRegistry(logger *slog.Logger) *HandlerRegistry {
	goAnalyzer := golang.New()
	s := scanner.New(logger, goAnalyzer)

	return &HandlerRegistry{
		scanner: s,
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
// Handler implementations
// =============================================================================

// ArchScanArgs are the arguments for arch_scan.
type ArchScanArgs struct {
	Path string `json:"path"`
}

// ArchScanResult is the result of arch_scan.
type ArchScanResult struct {
	RootPath  string        `json:"root_path"`
	Topology  string        `json:"topology"`
	NodeCount int           `json:"node_count"`
	EdgeCount int           `json:"edge_count"`
	Nodes     []*model.Node `json:"nodes"`
	Edges     []*model.Edge `json:"edges"`
	Summary   string        `json:"summary"`
}

func (h *HandlerRegistry) archScan(ctx context.Context, args ArchScanArgs) (*ArchScanResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	graph, err := h.scanner.Scan(args.Path)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	return &ArchScanResult{
		RootPath:  graph.RootPath,
		Topology:  string(graph.Topology),
		NodeCount: graph.NodeCount(),
		EdgeCount: graph.EdgeCount(),
		Nodes:     graph.Nodes(),
		Edges:     graph.Edges(),
		Summary:   graph.Summary(),
	}, nil
}

// ArchFocusArgs are the arguments for arch_focus.
type ArchFocusArgs struct {
	Path string `json:"path"`
}

func (h *HandlerRegistry) archFocus(ctx context.Context, args ArchFocusArgs) (*ArchScanResult, error) {
	// Same as scan but for a subdirectory
	return h.archScan(ctx, ArchScanArgs{Path: args.Path})
}

// ArchGenerateArgs are the arguments for arch_generate.
type ArchGenerateArgs struct {
	Path      string `json:"path"`
	Format    string `json:"format,omitempty"`
	ViewLevel string `json:"view_level,omitempty"`
	Title     string `json:"title,omitempty"`
	Direction string `json:"direction,omitempty"`
}

// ArchGenerateResult is the result of arch_generate.
type ArchGenerateResult struct {
	Format  string `json:"format"`
	Diagram string `json:"diagram"`
	Summary string `json:"summary"`
}

func (h *HandlerRegistry) archGenerate(ctx context.Context, args ArchGenerateArgs) (*ArchGenerateResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	graph, err := h.scanner.Scan(args.Path)
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
	default:
		return nil, fmt.Errorf("unsupported format: %s (supported: mermaid)", args.Format)
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
}

// ArchDependenciesResult is the result of arch_dependencies.
type ArchDependenciesResult struct {
	Internal       []string `json:"internal"`
	External       []string `json:"external"`
	Infrastructure []string `json:"infrastructure"`
}

func (h *HandlerRegistry) archDependencies(ctx context.Context, args ArchDependenciesArgs) (*ArchDependenciesResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	graph, err := h.scanner.Scan(args.Path)
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
	// Go stdlib packages don't contain dots in the first path segment
	parts := splitFirst(importPath, "/")
	return len(parts) > 0 && !containsDot(parts[0])
}

func splitFirst(s, sep string) []string {
	idx := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			return []string{s[:i], s[i+1:]}
		}
		idx = i
	}
	_ = idx
	return []string{s}
}

func containsDot(s string) bool {
	for _, c := range s {
		if c == '.' {
			return true
		}
	}
	return false
}

// Placeholder implementations for remaining tools.
// These will be fully implemented in subsequent development weeks.

type ArchDataflowArgs struct {
	Path string `json:"path"`
}

type ArchDataflowResult struct {
	Endpoints []string `json:"endpoints"`
	DataPaths []string `json:"data_paths"`
	Summary   string   `json:"summary"`
}

func (h *HandlerRegistry) archDataflow(ctx context.Context, args ArchDataflowArgs) (*ArchDataflowResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	graph, err := h.scanner.Scan(args.Path)
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

type ArchBoundariesResult struct {
	Topology   string   `json:"topology"`
	Boundaries []string `json:"boundaries"`
	Summary    string   `json:"summary"`
}

func (h *HandlerRegistry) archBoundaries(ctx context.Context, args ArchBoundariesArgs) (*ArchBoundariesResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	graph, err := h.scanner.Scan(args.Path)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	var boundaries []string
	for _, n := range graph.NodesByType(model.NodeService) {
		boundaries = append(boundaries, n.Name)
	}
	for _, n := range graph.NodesByType(model.NodeModule) {
		boundaries = append(boundaries, n.Name)
	}

	return &ArchBoundariesResult{
		Topology:   string(graph.Topology),
		Boundaries: boundaries,
		Summary:    fmt.Sprintf("Detected %s topology with %d boundaries", graph.Topology, len(boundaries)),
	}, nil
}

type ArchDiffArgs struct {
	Path         string `json:"path"`
	SnapshotFile string `json:"snapshot_file"`
}

func (h *HandlerRegistry) archDiff(_ context.Context, args ArchDiffArgs) (*model.DiffReport, error) {
	return &model.DiffReport{
		Summary:     "Diff detection not yet implemented. Save a snapshot first with arch_snapshot.",
		MaxSeverity: model.SeverityNone,
	}, nil
}

type ArchDriftArgs struct {
	Path    string `json:"path"`
	BaseRef string `json:"base_ref"`
	HeadRef string `json:"head_ref,omitempty"`
}

func (h *HandlerRegistry) archDrift(_ context.Context, args ArchDriftArgs) (*model.DiffReport, error) {
	return &model.DiffReport{
		BaseRef:     args.BaseRef,
		CompareRef:  args.HeadRef,
		Summary:     "Git ref drift detection not yet implemented.",
		MaxSeverity: model.SeverityNone,
	}, nil
}

type ArchValidateArgs struct {
	Path string `json:"path"`
}

type ArchValidateResult struct {
	Valid      bool     `json:"valid"`
	Violations []string `json:"violations"`
	Summary    string   `json:"summary"`
}

func (h *HandlerRegistry) archValidate(ctx context.Context, args ArchValidateArgs) (*ArchValidateResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	graph, err := h.scanner.Scan(args.Path)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	var violations []string
	if graph.HasCycle() {
		violations = append(violations, "circular dependency detected")
	}

	return &ArchValidateResult{
		Valid:      len(violations) == 0,
		Violations: violations,
		Summary:    fmt.Sprintf("Validation complete: %d violations found", len(violations)),
	}, nil
}

type ArchHistoryArgs struct {
	Path string `json:"path"`
}

type ArchHistoryResult struct {
	Summary string `json:"summary"`
}

func (h *HandlerRegistry) archHistory(_ context.Context, args ArchHistoryArgs) (*ArchHistoryResult, error) {
	return &ArchHistoryResult{
		Summary: "Architecture history tracking not yet implemented.",
	}, nil
}

type ArchSnapshotArgs struct {
	Path       string `json:"path"`
	OutputFile string `json:"output_file,omitempty"`
	Label      string `json:"label,omitempty"`
}

type ArchSnapshotResult struct {
	File    string `json:"file"`
	Summary string `json:"summary"`
}

func (h *HandlerRegistry) archSnapshot(ctx context.Context, args ArchSnapshotArgs) (*ArchSnapshotResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	_, err := h.scanner.Scan(args.Path)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	return &ArchSnapshotResult{
		Summary: "Snapshot saving not yet implemented. Use arch_scan + arch_generate for now.",
	}, nil
}

type ArchExplainArgs struct {
	Path     string `json:"path"`
	Question string `json:"question,omitempty"`
}

type ArchExplainResult struct {
	Explanation string   `json:"explanation"`
	Evidence    []string `json:"evidence"`
}

func (h *HandlerRegistry) archExplain(ctx context.Context, args ArchExplainArgs) (*ArchExplainResult, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	graph, err := h.scanner.Scan(args.Path)
	if err != nil {
		return nil, fmt.Errorf("scanning codebase: %w", err)
	}

	return &ArchExplainResult{
		Explanation: graph.Summary(),
		Evidence:    []string{"Based on static analysis of source files"},
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

	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, args Args) (*mcp.CallToolResult, Result, error) {
		result, err := handler(ctx, args)
		if err != nil {
			var zero Result
			return nil, zero, fmt.Errorf("%s failed: %w", spec.Name, err)
		}
		return nil, result, nil
	})
}
