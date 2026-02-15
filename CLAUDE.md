# Code to Arch MCP

## Project
Go MCP server that scans codebases, generates architecture diagrams, and detects when code drifts from documented architecture. Supports Go (go/ast), TypeScript, and Python (tree-sitter). Outputs Mermaid, PlantUML, C4, Structurizr DSL, draw.io XML, and Excalidraw JSON.

## Architecture
- `cmd/code-to-arch/main.go` - entry point with stdio transport
- `internal/model/` - ArchGraph, Node, Edge, Diff types (core data model)
- `internal/scanner/` - File walker, orchestrator (discovers files and delegates to analyzers)
- `internal/analyzer/golang/` - Go AST-based analysis (import-based deps, stdlib HTTP endpoints, infra classification)
- `internal/analyzer/typescript/` - tree-sitter TypeScript analysis (import-based deps, Express endpoints, infra classification)
- `internal/analyzer/python/` - tree-sitter Python analysis (import-based deps, Flask/FastAPI endpoints, infra classification)
- `internal/detector/` - Boundary detection, topology inference, dataflow tracing, rule validation, recommendations
- `internal/render/` - Output renderers: Mermaid, PlantUML, C4, Structurizr, draw.io, Excalidraw
- `internal/drift/` - Drift detection: graph comparison (exact ID match), severity classification, reports
- `internal/infra/` - Cache, persistent state (persist.go for ~/.mcp-context/)
- `tools/` - MCP tool definitions and handlers
- `tracing/` - OpenTelemetry setup

## Tool Categories
- **analysis** (6 tools): arch_scan, arch_focus, arch_dependencies, arch_dataflow, arch_boundaries, arch_explain
- **diagram** (1 tool): arch_generate
- **drift** (2 tools): arch_diff, arch_drift
- **validation** (3 tools): arch_validate, arch_metrics, arch_recommend
- **history** (1 tool): arch_history
- **export** (1 tool): arch_snapshot

## Key Patterns
- ArchGraph is the central model; all analyzers produce Nodes and Edges into the same graph
- Language analyzers implement the Analyzer interface with `Analyze(path) ([]*Node, []*Edge, error)`
- Scanner orchestrates: walk files -> detect changes (incremental) -> delegate to analyzer -> merge graphs
- Incremental scanning: ScanState tracks per-file mtime + content hash; unchanged files reuse cached analysis results
- State persists to ~/.mcp-context/code-to-arch/ via infra.StateDir() convention
- Renderers implement `Render(graph *ArchGraph, opts RenderOptions) (string, error)`
- Drift detection uses exact node ID matching + edge key comparison (no fuzzy matching)
- "USE WHEN" description pattern for optimal LLM tool selection

## Build & Test
```bash
make build       # Build binary
make test        # Run tests with race detector
make check       # fmt-check + vet + test
```

## Adding New Language Analyzers
1. Create `internal/analyzer/<lang>/analyzer.go` implementing the Analyzer interface
2. Register in `internal/scanner/scanner.go` file extension mapping
3. Add test fixtures in `testdata/`
