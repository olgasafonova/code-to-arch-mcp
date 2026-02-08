# Code to Arch MCP

An MCP server that reverse-engineers architecture from source code. Point it at any codebase; it returns services, databases, queues, endpoints, and their relationships as a structured graph. Generate diagrams in 7 formats. Detect drift between any two branches, tags, or commits. Validate architecture rules. Track how the system evolves over time.

No configuration files, no manual diagramming. Static analysis builds the architecture model directly from your code.

## What it does

**Parses source files with language-specific analyzers** (Go via `go/ast`, TypeScript and Python via tree-sitter) and builds an architecture graph of nodes and edges.

**Nodes** represent components: services, modules, packages, databases, message queues, caches, external APIs, HTTP endpoints.

**Edges** represent relationships: dependencies, API calls, data flows, publish/subscribe, read/write.

**12 MCP tools** let an LLM analyze, visualize, compare, and validate architecture on demand.

## Tools

| Tool | What it does |
|------|-------------|
| `arch_scan` | Scan a codebase and return the full architecture graph |
| `arch_focus` | Scan a specific subdirectory or service |
| `arch_generate` | Generate a diagram (Mermaid, PlantUML, C4, Structurizr, JSON, draw.io, Excalidraw) |
| `arch_dependencies` | Map internal, external, and infrastructure dependencies |
| `arch_dataflow` | Trace data flow from endpoints to data stores |
| `arch_boundaries` | Detect service boundaries and topology (monolith, monorepo, microservices) |
| `arch_diff` | Compare current architecture against a saved baseline |
| `arch_drift` | Compare architecture between two branches, tags, or commits |
| `arch_validate` | Check for circular dependencies, orphan nodes, and layering violations |
| `arch_history` | Show how architecture evolved over git history |
| `arch_snapshot` | Save current architecture as a baseline |
| `arch_explain` | Explain topology, patterns, key decisions, and risks with code evidence |

## Supported languages

| Language | Analyzer | Detection |
|----------|---------|-----------|
| Go | `go/ast` (stdlib) | Packages, imports, HTTP handlers, infrastructure |
| TypeScript/TSX | tree-sitter | Modules, imports, Express/Fastify/Koa routes, infrastructure |
| Python | tree-sitter | Modules, imports, Flask/FastAPI routes, infrastructure |

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

## Setup

### Prerequisites

- Go 1.24+
- C compiler (for tree-sitter CGo bindings; standard on macOS and Linux)

### Build

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

## Usage examples

Once configured, ask your LLM:

- "Scan the architecture of ~/Projects/my-app"
- "Generate a C4 diagram of this project"
- "Export the architecture as Excalidraw"
- "What databases does this service connect to?"
- "Are there any circular dependencies or layering violations?"
- "How has the architecture changed since last month?"
- "Compare architecture between the v1.0 tag and main branch"
- "Save this architecture as our v2.0 baseline"
- "Explain the architecture decisions in this codebase"
- "Scan this monorepo but limit to 500 files and skip test files"

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
  scanner/                 File walker + analyzer orchestration
  analyzer/golang/         Go static analysis (go/ast)
  analyzer/typescript/     TypeScript analysis (tree-sitter)
  analyzer/python/         Python analysis (tree-sitter)
  detector/                Boundary detection, topology, validation, explanation
  drift/                   Snapshot comparison, git ref diffing, history
  render/                  Mermaid, PlantUML, C4, Structurizr, JSON, draw.io, Excalidraw
  infra/                   Scan result cache
tools/                     MCP tool definitions and handlers
```

## License

MIT
