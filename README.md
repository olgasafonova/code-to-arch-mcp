# Code to Arch MCP

An MCP server that scans codebases and extracts architecture as structured data. Point it at a directory; it returns services, databases, queues, endpoints, and their relationships. Generate diagrams. Detect when code drifts from documented architecture.

## What it does

**Static analysis, not guesswork.** Parses source files with language-specific analyzers (Go via `go/ast`, TypeScript via tree-sitter) and builds an architecture graph of nodes and edges.

**Nodes** represent components: services, modules, packages, databases, message queues, caches, external APIs, HTTP endpoints.

**Edges** represent relationships: dependencies, API calls, data flows, publish/subscribe, read/write.

The server exposes 12 MCP tools that an LLM can call to analyze, visualize, and validate architecture.

## Tools

| Tool | What it does |
|------|-------------|
| `arch_scan` | Scan a codebase and return the full architecture graph |
| `arch_focus` | Scan a specific subdirectory or service |
| `arch_generate` | Generate a diagram (Mermaid, PlantUML) |
| `arch_dependencies` | Map internal, external, and infrastructure dependencies |
| `arch_dataflow` | Trace data flow from endpoints to data stores |
| `arch_boundaries` | Detect service boundaries and topology (monolith, monorepo, microservices) |
| `arch_diff` | Compare current architecture against a saved baseline |
| `arch_drift` | Compare architecture between two git refs |
| `arch_validate` | Check for circular dependencies and layering violations |
| `arch_history` | Show how architecture evolved over git history |
| `arch_snapshot` | Save current architecture as a baseline |
| `arch_explain` | Explain architecture decisions with code evidence |

## Infrastructure detection

The analyzers recognize common infrastructure packages and classify them automatically:

| Category | Go packages | TypeScript packages |
|----------|------------|-------------------|
| **Database** | database/sql, gorm, pgx, sqlx | pg, prisma, typeorm, mongoose, sequelize, drizzle-orm |
| **Queue** | amqp, kafka, nats | kafkajs, bullmq, amqplib, nats |
| **Cache** | redis, memcache | ioredis, redis, keyv |
| **HTTP client** | net/http (client) | axios, node-fetch, got, undici |

TypeScript route detection covers Express, Fastify, and Koa patterns (`app.get('/path', handler)`).

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

## Usage examples

Once configured, ask your LLM:

- "Scan the architecture of ~/Projects/my-app"
- "Generate a Mermaid diagram of this project"
- "What databases does this service connect to?"
- "Are there any circular dependencies?"
- "Save this architecture as our v1.0 baseline"
- "Has the architecture changed since the last snapshot?"

## Output formats

| Format | Status |
|--------|--------|
| Mermaid | Implemented |
| PlantUML | Implemented |
| C4 | Planned |
| Structurizr DSL | Planned |
| JSON | Planned |

## Supported languages

| Language | Analyzer | Detection |
|----------|---------|-----------|
| Go | `go/ast` (stdlib) | Packages, imports, HTTP handlers, infrastructure |
| TypeScript/TSX | tree-sitter | Modules, imports, Express/Fastify routes, infrastructure |
| Python | tree-sitter | Planned |

## Development

```bash
make check       # fmt-check + vet + tests (with race detector)
make build       # Build binary
make test        # Tests only
```

### Integration tests

Run against real codebases (requires network for open-source repo cloning):

```bash
go test -tags integration -race -v ./tests/
```

Or use the smoke test script:

```bash
bash scripts/smoke-test.sh
```

## Architecture

```
cmd/code-to-arch/       Entry point (stdio + HTTP transport)
internal/
  model/                ArchGraph, Node, Edge, Diff types
  scanner/              File walker + analyzer orchestration
  analyzer/golang/      Go static analysis (go/ast)
  analyzer/typescript/  TypeScript analysis (tree-sitter)
  detector/             Boundary detection, topology inference
  drift/                Snapshot comparison, severity classification
  render/               Mermaid, PlantUML renderers
tools/                  MCP tool definitions and handlers
```

## License

MIT
