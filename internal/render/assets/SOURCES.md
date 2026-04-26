# Embedded asset provenance

## mermaid.min.js

| Field | Value |
|---|---|
| Library | Mermaid (https://github.com/mermaid-js/mermaid) |
| Version | 9.3.0 |
| Source URL | https://cdn.jsdelivr.net/npm/mermaid@9.3.0/dist/mermaid.min.js |
| SHA-256 | `41d4c6d584cb2d3c03ddbf798cba85c69417f6e62e24c340b55660c0a5f8a185` |
| Size | 878 KB |
| License | MIT |
| Embedded by | `internal/render/html.go` via `go:embed` |

Why pinned to v9.3.0: later patch releases of v9 ballooned to 2.7 MB, and v10 is over 3 MB. v9.3.0 is the size-stable line that still supports the flowchart syntax `internal/render/mermaid.go` produces.

## Refresh procedure

```bash
curl -sL -o internal/render/assets/mermaid.min.js \
  "https://cdn.jsdelivr.net/npm/mermaid@<version>/dist/mermaid.min.js"
shasum -a 256 internal/render/assets/mermaid.min.js
# Update version, SHA-256, and size fields above.
```

Run the test suite after refresh; `internal/render/html_test.go` includes a presence check on the embedded library.
