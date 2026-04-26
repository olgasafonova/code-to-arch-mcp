# Cross-substrate composition: spike findings

Companion to bead `code-to-arch-mcp-edf`. Status: exploration complete,
graduation decision pending.

## What the spike did

Implemented one cross-substrate edge type: notes → Go packages. Lives in
`internal/detector/notes_to_packages.go` as `LinkNotesToPackages(graph) int`.

The function walks all `NodeNote` entries in an `ArchGraph`, reads each
file, extracts inline-code spans (single-backtick `` `...` `` only), and
matches normalized path-like tokens against an index of Go package
directories. Matches produce `EdgeDependency` edges with label `"documents"`
and confidence `0.6` (lower than the 0.9 confidence given to AST-resolved
imports).

The spike is *not wired into any MCP tool*. It is a callable function in
the detector package, exercised by tests only.

## What the spike found on this repo

Running against the project root (Go + markdown analyzers, then
`LinkNotesToPackages`) produced **15 cross-substrate edges**:

```
note:assets/SOURCES → pkg:render/render
note:code-to-arch-mcp/CLAUDE → pkg:code-to-arch/main, pkg:model/model,
                               pkg:scanner/scanner, pkg:golang/golang,
                               pkg:typescript/typescript, pkg:python/python,
                               pkg:detector/detector, pkg:render/render,
                               pkg:drift/drift, pkg:registry/registry,
                               pkg:infra/infra
note:docs/incremental-scan-vs-merkle-dag → pkg:model/model,
                                           pkg:scanner/scanner,
                                           pkg:infra/infra
```

Reproduce with `go test -race -run LinkNotesToPackages_ThisRepo ./internal/detector/ -v`.

These are real, useful documentation links. CLAUDE.md is the project's
guide and references nearly every internal package; the spike turned that
prose into structured edges. The Merkle-DAG design note's references to
`internal/scanner/filestate.go` produced a clean edge from the design doc
to the package it documents.

The README produced **zero** cross-substrate edges, even though it talks
about scanner, render, etc. The reason: the README mentions those packages
in plain prose (e.g., `"If I change internal/scanner what else needs review?"`)
not in inline code spans. Decision-doc-style documents (CLAUDE.md, design
notes) consistently use code spans for path references; user-facing README
prose does not.

This is the spike's most actionable finding: **inline-code-span matching
is sufficient for the design-doc / decision-record use case the bead
called out, and insufficient for general README documentation**. If we
want to also catch package references in prose, we need a second matching
pass with stricter false-positive controls.

## Open questions from the bead, with spike answers

> 1. How does a markdown note reference a Go package? Inline code spans?
>    An explicit annotation syntax? File path matching?

**Spike answer:** inline code spans work well for the most valuable corpus
(design docs, decision records, CLAUDE.md-style guides). They produce
high-precision edges with low engineering cost. An explicit annotation
syntax (e.g., YAML frontmatter `references: [internal/scanner]`) would
improve recall on prose-heavy notes, but costs author effort. File path
matching against bare prose tokens is too noisy: words like "client" or
"server" appear constantly without referring to a package.

Recommendation: keep code-span matching as the primary signal. Add a
frontmatter-driven secondary signal if the prose-heavy use case becomes
important.

> 2. What's the right unit on the OKR / decision-doc side — frontmatter
>    tags? A YAML manifest?

**Spike answer:** for decision docs, *the document itself is the unit*.
Each `.md` file becomes a `NodeNote` already, and the spike connects it
to packages it references. Frontmatter is unnecessary unless we want to
encode metadata that doesn't naturally appear in prose (status, owner,
review date).

For OKRs, we did not test this. OKR documents tend to use higher-level
language ("ship feature X", "improve metric Y") that doesn't reference
package paths at all. OKR ↔ commit linking is a different problem and
would need a different primitive.

> 3. Does this stay one MCP server or split out (each substrate-bridge
>    as its own analyzer)?

**Spike answer:** stay in one MCP server, but factor cross-substrate
linking as a *post-processing pass* rather than as a per-file analyzer.
Per-file analyzers are the wrong shape because cross-substrate matching
needs the *complete graph* (all package nodes must exist before we can
match note tokens against them). The current spike runs after Scan, which
is the right place.

If we add more substrate bridges later (config files → services, OKR
docs → commits, decision records → endpoints), each gets its own
post-processing function in the detector package. The naming convention
"`LinkXToY`" makes the source and target substrates explicit.

## Naming the move

The bead noted the framing is currently nameless. The spike used the term
"cross-substrate edges" internally, but for a user-facing tool name, the
question becomes: what does the user actually ask for?

Three candidate framings:

- **"Architecture-as-substrate-graph"** — accurate but academic
- **"Documentation-aware architecture"** — captures the design-doc case
  but understates the OKR / config potential
- **"Architecture-aware documentation"** — flipped framing; might land
  better with the audience that writes the docs rather than reads them

For the eventual MCP tool name: `arch_orphan_decisions` (suggested in the
bead) is good for the inverse query ("which decision notes reference
nothing in code?"). The forward query needs its own name —
`arch_doc_links`, `arch_documents`, or `arch_explain_with_docs` are
candidates.

## Recommended next steps if this graduates

If the team decides to ship this, the smallest viable graduation is:

1. Add a `LinkAllSubstrates(graph) int` aggregator in the detector
   package. Today it just calls `LinkNotesToPackages`; future bridges
   plug in here.
2. Wire that aggregator into the scanner orchestration so cross-substrate
   edges are present in every `arch_scan` result. Confidence stays at 0.6
   so existing consumers can filter them out.
3. Add an MCP tool `arch_orphan_decisions` (or similar) that walks the
   graph and finds `NodeNote` entries with no outgoing edges — the
   "documents nothing in code" case the bead called out.
4. Add an MCP tool / argument for "show notes documenting this package",
   the inverse query.

What we do *not* recommend doing on graduation day:

- Adding plain-prose path-matching with confidence < 0.6. The
  false-positive rate is too high to be useful in a default scan;
  consumers will get noise without an obvious off-switch.
- Adding YAML frontmatter parsing as a hard requirement. It's an
  optional enhancement, not a baseline.
- Adding more substrate bridges (configs, OKRs, commits) before the
  notes-↔-packages bridge has been in real use for a while. Each bridge
  has its own false-positive failure modes; adding three at once
  produces a graph where consumers can't tell which edges to trust.

## What to read

- `internal/detector/notes_to_packages.go` — the spike implementation
- `internal/detector/notes_to_packages_test.go` — unit tests for the
  matcher and the path-suffix indexer
- `internal/detector/notes_to_packages_integration_test.go` — the
  this-repo integration test that produced the 15 edges above
