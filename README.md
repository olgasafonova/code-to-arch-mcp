# Code to Arch MCP

An MCP server that reverse-engineers architecture from source code. Point it at any codebase; it returns services, databases, queues, endpoints, and their relationships as a structured graph. Also works on markdown directories (Obsidian vaults, doc trees) — wiki-links and relative `.md` links become dependency edges. Generate diagrams in 9 formats including a self-contained D3 force-directed page for hub-spoke graphs. Detect drift between any two branches, tags, or commits. Validate architecture rules. Track how the system evolves over time.

No configuration files, no manual diagramming. Static analysis builds the architecture model directly from your code.

## Why

Architecture diagrams go stale the day you commit them. When AI generates code faster than teams can comprehend the changes, the gap between system complexity and shared understanding grows. This is cognitive debt, and it compounds silently.

Most teams know they should review architecture regularly, check for circular dependencies, and catch structural drift between branches. In practice, these tasks are manual enough that they don't happen.

- This tool generates architecture from code, so diagrams are always current. No one has to maintain them.
- `arch_validate` turns "check for circular dependencies" from a retro action item into a one-prompt task.
- `arch_drift` compares architecture between any two git refs. It catches structural changes that code review misses: a new database dependency, a service that quietly became a monolith, an endpoint that bypasses the API gateway.
- `arch_drift_explain` wraps that diff in a 2-5 sentence narrative you can paste straight into a PR description, standup channel, or release note. No LLM call; pure templating from the structured diff.

<!-- TODO: Add demo GIF showing arch_scan + arch_generate on a real project -->

## What it does

**Parses source files with language-specific analyzers** (Go via `go/ast`, TypeScript and Python via tree-sitter, markdown via regex-based link extraction) and builds an architecture graph of nodes and edges.

**Nodes** represent components: services, modules, packages, databases, message queues, caches, external APIs, HTTP endpoints, and notes (markdown files).

**Edges** represent relationships with confidence scores: dependencies (0.9), endpoint registrations (0.85), infrastructure links (0.8), HTTP client calls (0.7). Confidence lets consumers filter by reliability; direct AST-resolved imports score higher than heuristic matches.

[MCP](https://modelcontextprotocol.io/) (Model Context Protocol) lets AI assistants call external tools. This server gives your AI assistant 19 architecture analysis tools.

## What you get

Run `arch_scan` on a Go project and get back a structured architecture graph:

```json
{
  "topology": "monorepo",
  "nodes": [
    {"id": "pkg:api/server", "name": "server", "type": "package", "language": "go"},
    {"id": "pkg:worker/processor", "name": "processor", "type": "package", "language": "go"},
    {"id": "infra:postgresql", "name": "PostgreSQL", "type": "database"},
    {"id": "infra:redis", "name": "Redis", "type": "cache"},
    {"id": "infra:nats", "name": "NATS", "type": "queue"}
  ],
  "edges": [
    {"source": "pkg:api/server", "target": "infra:postgresql", "type": "read_write", "confidence": 0.8},
    {"source": "pkg:api/server", "target": "infra:redis", "type": "read_write", "confidence": 0.8},
    {"source": "pkg:worker/processor", "target": "infra:nats", "type": "subscribe", "confidence": 0.8}
  ],
  "stats": {"files_analyzed": 47, "files_cached": 38, "files_changed": 9, "nodes_found": 12, "edges_found": 23, "duration_ms": 340}
}
```

Then ask `arch_generate` for a Mermaid diagram, `arch_validate` to check for circular dependencies, or `arch_dataflow` for structured traces showing how requests flow from endpoints to databases. Infrastructure (databases, queues, caches) is detected automatically from import paths.

## Usage examples

Once configured, ask your LLM:

- "Scan the architecture of ~/Projects/my-app"
- "Generate a C4 diagram of this project"
- "Are there any circular dependencies or layering violations?"
- "Compare architecture between the v1.0 tag and main branch"
- "How has the architecture changed since last month?"
- "What databases does this service connect to?"
- "Export the architecture as Excalidraw"
- "Explain the architecture decisions in this codebase"
- "How should I improve this architecture?"
- "What's the coupling and instability like?"
- "Show me data flow traces from API endpoints to databases"
- "Save this architecture as our v2.0 baseline"
- "Scan this monorepo but limit to 500 files and skip test files"
- "Scan an Obsidian vault and find orphan notes"
- "Render the docs/ directory as a force-directed graph showing hubs at degree>=10"
- "If I change internal/scanner what else needs review?" (uses arch_blast_radius)

## Tools

### Key tools

| Tool | What it does |
|------|-------------|
| `arch_scan` | Scan a codebase or markdown directory and return the full architecture graph with confidence-scored edges |
| `arch_generate` | Generate a diagram (Mermaid, PlantUML, C4, Structurizr, JSON, draw.io, Excalidraw, HTML, forcegraph) |
| `arch_blast_radius` | Find every node that transitively depends on a target — answers "if I change X, what else needs review?" |
| `arch_drift` | Compare architecture between two branches, tags, or commits |
| `arch_drift_explain` | Compare two refs and return a 2-5 sentence narrative summary plus the structured diff — paste-ready for PR descriptions |
| `arch_dataflow` | Trace data flow from endpoints to data stores with structured process traces |
| `arch_validate` | Check for circular dependencies, orphan nodes, and layering violations |
| `arch_recommend` | Produce prioritized architecture improvement recommendations |

### All 19 tools

| Tool | Category | What it does |
|------|----------|-------------|
| `arch_scan` | analysis | Scan a codebase or markdown directory and return the full architecture graph |
| `arch_focus` | analysis | Scan a specific subdirectory or service |
| `arch_dependencies` | analysis | Map internal, external, and infrastructure dependencies |
| `arch_blast_radius` | analysis | Find the transitive set of nodes that depend on a target file or package |
| `arch_dataflow` | analysis | Trace data flow with entry-to-terminal process traces and confidence scores |
| `arch_boundaries` | analysis | Detect service boundaries and topology (monolith, monorepo, microservices) |
| `arch_explain` | analysis | Explain topology, patterns, key decisions, and risks with code evidence |
| `arch_generate` | diagram | Generate a diagram in 9 formats |
| `arch_diff` | drift | Compare current architecture against a saved baseline |
| `arch_drift` | drift | Compare architecture between two git refs |
| `arch_drift_explain` | drift | Narrative summary of drift between two git refs (paste-ready prose) |
| `arch_validate` | validation | Check for circular dependencies, orphan nodes, and layering violations |
| `arch_metrics` | validation | Compute coupling, instability, and dependency depth scores |
| `arch_recommend` | validation | Prioritized improvement recommendations from metrics + violations + patterns |
| `arch_history` | history | Show how architecture evolved over git history |
| `arch_snapshot` | export | Save current architecture as a baseline for drift detection |
| `arch_registry_add` | registry | Register a repo by alias for reuse across tool calls |
| `arch_registry_list` | registry | List all registered repos |
| `arch_registry_remove` | registry | Remove a registered repo alias |

## Supported languages

| Language | Analyzer | Detection |
|----------|---------|-----------|
| Go | `go/ast` (stdlib) | Packages, imports, HTTP handlers, infrastructure |
| TypeScript/TSX | tree-sitter | Modules, imports, Express/Fastify/Koa routes, infrastructure |
| Python | tree-sitter | Modules, imports, Flask/FastAPI routes, infrastructure |
| Markdown | regex link extraction | Notes, Obsidian wiki-links `[[note]]`, relative `[text](./file.md)` links |

## Infrastructure detection

Analyzers recognize common infrastructure packages and classify them automatically:

| Category | Go | TypeScript | Python |
|----------|-----|-----------|--------|
| **Database** | database/sql, gorm, pgx, sqlx | pg, prisma, typeorm, mongoose, sequelize, drizzle-orm | sqlalchemy, django.db, pymongo, psycopg2, peewee, tortoise |
| **Queue** | amqp, kafka, nats | kafkajs, bullmq, amqplib, nats | celery, kombu, pika, kafka, rq |
| **Cache** | redis, memcache | ioredis, redis, keyv | redis, pymemcache, aiocache |
| **HTTP client** | net/http (client) | axios, node-fetch, got, undici | requests, httpx, aiohttp, urllib3 |

## Output formats

| Format | Description |
|--------|------------|
| Mermaid | Flowchart syntax, renders in GitHub, Notion, most markdown viewers |
| PlantUML | Component diagrams with UML notation |
| C4 | C4-PlantUML container diagrams with `!include <C4/C4_Container>` |
| Structurizr DSL | Workspace model for Structurizr tooling |
| JSON | Structured data with nodes, edges, topology metadata |
| draw.io | XML format, open directly in diagrams.net |
| Excalidraw | JSON format, open directly in Excalidraw |
| HTML | Self-contained page with the Mermaid runtime embedded inline (~900 KB output, no network requests) |
| forcegraph | Self-contained D3-driven force-directed page (~290 KB) with drag, zoom, pan; color = connected component; node size scales with degree. Use for hub-spoke graphs (knowledge vaults, dense dependency networks) where Mermaid's hierarchical layout produces a long horizontal stripe. Pair with `min_degree=10` to keep only hubs |

## Setup

### Prerequisites

- Go 1.24+
- C compiler (for tree-sitter CGo bindings; standard on macOS and Linux)

### Install

```bash
go install github.com/olgasafonova/code-to-arch-mcp/cmd/code-to-arch@latest
```

The binary lands in `$GOPATH/bin` (typically `~/go/bin/code-to-arch`).

### Or build from source

```bash
git clone https://github.com/olgasafonova/code-to-arch-mcp.git
cd code-to-arch-mcp
make build
```

### Configure in Claude Code

Add to your `~/.claude.json`:

```json
{
  "mcpServers": {
    "code-to-arch": {
      "command": "/path/to/code-to-arch",
      "args": []
    }
  }
}
```

Or run from source:

```json
{
  "mcpServers": {
    "code-to-arch": {
      "command": "go",
      "args": ["run", "./cmd/code-to-arch"],
      "cwd": "/path/to/code-to-arch-mcp"
    }
  }
}
```

## Scan control

All scan tools accept optional parameters for handling large codebases:

| Parameter | What it does |
|-----------|-------------|
| `max_files` | Stop after analyzing N files (returns partial result) |
| `max_nodes` | Stop after discovering N architecture nodes |
| `timeout_secs` | Cancel scan after N seconds |
| `workers` | Parallel analysis workers (default: CPU count, max 8) |
| `skip_dirs` | Additional directories to skip (beyond defaults like `node_modules`, `.git`, `vendor`) |
| `skip_globs` | File patterns to skip (e.g. `*_test.go`, `*.spec.ts`) |

Partial results include a `truncated: true` flag so you know the graph is incomplete. Sequential tool calls on the same path are cached for 30 seconds.

## Incremental scanning

Repeat scans on the same codebase are fast. The server tracks file modification times and content hashes in `~/.mcp-context/code-to-arch/`. On subsequent scans, only files that actually changed get re-analyzed; unchanged files reuse cached analysis results.

The stats in the response show what happened:
- `files_analyzed` — total files in the codebase
- `files_cached` — files skipped (unchanged since last scan)
- `files_changed` — files re-analyzed (new, modified, or deleted)

First scan of a 500-file project takes a few seconds. Follow-up scans after editing 3 files take milliseconds.

## Development

```bash
make check       # fmt-check + vet + tests (with race detector)
make build       # Build binary
make test        # Tests only
```

### Integration tests

Run against real codebases:

```bash
go test -tags integration -race -v ./tests/
```

Or use the smoke test script:

```bash
bash scripts/smoke-test.sh
```

## Architecture

```
cmd/code-to-arch/          Entry point (stdio + HTTP transport)
internal/
  model/                   ArchGraph, Node, Edge, Diff types
  scanner/                 File walker, incremental change detection, analyzer orchestration
  analyzer/golang/         Go static analysis (go/ast)
  analyzer/typescript/     TypeScript analysis (tree-sitter)
  analyzer/python/         Python analysis (tree-sitter)
  detector/                Boundary detection, topology, validation, metrics, recommendations, process traces
  drift/                   Snapshot comparison, git ref diffing, history
  render/                  Mermaid, PlantUML, C4, Structurizr, JSON, draw.io, Excalidraw
  infra/                   Cache, persistent state (~/.mcp-context/)
tools/                     MCP tool definitions and handlers
```

## License

MIT
