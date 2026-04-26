# Ridge

## Project
Go MCP server that scans codebases, generates architecture diagrams, and detects when code drifts from documented architecture. Supports Go (go/ast), TypeScript, and Python (tree-sitter). Outputs Mermaid, PlantUML, C4, Structurizr DSL, draw.io XML, and Excalidraw JSON.

## Architecture
- `cmd/ridge/main.go` - entry point with stdio transport
- `internal/model/` - ArchGraph, Node, Edge, Diff types (core data model)
- `internal/scanner/` - File walker, orchestrator (discovers files and delegates to analyzers)
- `internal/analyzer/golang/` - Go AST-based analysis (import-based deps, stdlib HTTP endpoints, infra classification)
- `internal/analyzer/typescript/` - tree-sitter TypeScript analysis (import-based deps, Express endpoints, infra classification)
- `internal/analyzer/python/` - tree-sitter Python analysis (import-based deps, Flask/FastAPI endpoints, infra classification)
- `internal/detector/` - Boundary detection, topology inference, dataflow tracing, rule validation, recommendations
- `internal/render/` - Output renderers: Mermaid, PlantUML, C4, Structurizr, draw.io, Excalidraw
- `internal/drift/` - Drift detection: graph comparison (exact ID match), severity classification, reports
- `internal/registry/` - Persistent repo registry (aliases, scan metadata, state paths)
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
- **registry** (3 tools): arch_registry_add, arch_registry_remove, arch_registry_list

## Key Patterns
- ArchGraph is the central model; all analyzers produce Nodes and Edges into the same graph
- Language analyzers implement the Analyzer interface with `Analyze(path) ([]*Node, []*Edge, error)`
- Scanner orchestrates: walk files -> detect changes (incremental) -> delegate to analyzer -> merge graphs
- Incremental scanning: ScanState tracks per-file mtime + content hash; unchanged files reuse cached analysis results
- State persists to ~/.mcp-context/ridge/ via infra.StateDir() convention
- Renderers implement `Render(graph *ArchGraph, opts RenderOptions) (string, error)`
- Drift detection uses exact node ID matching + edge key comparison (no fuzzy matching)
- "USE WHEN" description pattern for optimal LLM tool selection
- Registry aliases enable `repo` param on all tools; resolveRepoPath resolves alias to path
- Registry-based scans persist incremental ScanState to ~/.mcp-context/ridge/state/<alias>.json

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


<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:ca08a54f -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

## Session Completion

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd dolt push
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
<!-- END BEADS INTEGRATION -->
