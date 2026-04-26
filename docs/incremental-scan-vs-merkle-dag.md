# Incremental scan state: ridge vs zilliztech/claude-context

Comparison of two incremental-indexing approaches for codebase analysis.
Filed under bead `ridge-ycx` Thread B. No code change here; this is a design read.

## What this compares

Both systems walk a codebase, persist per-file state, and reuse that state on the next run to avoid re-doing work for unchanged files. They serve different consumers, so the comparison has to pin down where their needs diverge.

- `ridge` reuses cached *analysis output* (Nodes and Edges in `internal/model`) for unchanged files, so the next `arch_scan` skips re-parsing.
- `zilliztech/claude-context` reuses cached *embedding state* (chunks already in a Zilliz vector DB) for unchanged files, so the next `index` call skips re-embedding.

The downstream caches differ; the upstream question is the same in both: what changed since last time?

## Side-by-side

| Dimension | ridge | zilliztech/claude-context |
|---|---|---|
| Hash function | SHA-256 of file content | SHA-256 of file content |
| Change detection unit | File | File |
| Stat fast path | Yes: `mtime` checked before reading the file | No: every file is read and hashed every run |
| Sub-file granularity | None | None |
| Persistence format | JSON via `infra.LoadJSON` / `infra.SaveJSON` | JSON, hand-rolled |
| State location | `~/.mcp-context/code-to-arch/state/<alias>.json` | `~/.context/merkle/<md5(path)>.json` |
| What is cached alongside file state | Analysis output (Nodes, Edges) | Just the path → hash map |
| Rename detection | None | None |
| Concurrency / locking | None | None |
| "Merkle DAG" structure | None | Yes, but degenerate (see below) |

## What "Merkle DAG" actually means in claude-context

The headline claim is "Merkle DAG for incremental indexing." Reading `packages/core/src/sync/merkle.ts` and `synchronizer.ts`, the actual structure is a flat 2-level graph: one root node and N file leaves, no directory layer, no AST or chunk layer.

Each leaf node's ID is `sha256(path + ":" + fileHash)`. The root node's data is the concatenation of all child file hashes, hashed again. Pure Merkle in shape.

The reveal is in `MerkleDAG.compare`. It does not exploit the recursive hashing property. It collects every node ID from both DAGs into two sets and computes the symmetric difference. A modified file shows up as one removed leaf (old ID) plus one added leaf (new ID). The root hash exists, but the comparison code never compares roots first. There is no O(1) "nothing changed" fast path.

So the practical change-detection algorithm reduces to: hash every file fresh, compare the hash maps. The DAG layer adds no behavior beyond what a `Map<path, hash>` already gives you. It is a Merkle DAG in name and topology, not in operational benefit.

This matters because it removes the apparent reason to port the structure. The `ridge` design (`internal/scanner/filestate.go::DetectChanges`) already does the same hash-map comparison without the ceremony.

## Where the implementations meaningfully differ

### mtime fast path

`filestate.go:71-95` checks `os.Stat().ModTime()` against the saved mtime before reading the file. Equal mtime, file is unchanged, no read, no hash. Different mtime, hash to confirm (cheap touch detection). This is a real optimization for repeated scans where most files are untouched between runs. Re-scanning a 50k-file repo on which only one file changed reads exactly one file.

`claude-context` always calls `fs.readFile` and computes SHA-256 for every file. On the same 50k-file repo it reads 50k files every run. The cost is hidden by the much heavier embedding step that follows, so it does not show up in their hot path. For a Go repo with a static analyzer, where the analysis step is fast, the mtime check is the difference between sub-second and multi-second scans.

### Cache payload

`filestate.go:13-19` stores `Nodes []*model.Node` and `Edges []*model.Edge` directly inside each `FileEntry`. Cached analysis output rides along with the file state, so an unchanged file produces zero work on the next scan: the result is right there in the JSON.

`claude-context` only caches the path → hash map. The actual cached artifacts (embeddings + chunk metadata) live in the vector DB, not in the snapshot file. That is appropriate for their model (embeddings are big, vector DB is the right home), but it means the snapshot alone is not a self-contained cache. Restoring on a fresh machine requires the vector DB to be intact too.

### Concurrency posture

Neither system has any locking. Both write the snapshot via a single `WriteFile` call. The blast radius differs: `claude-context` runs as a long-lived MCP server process with one Context per codebase, so concurrent calls within the same process race. `ridge` writes state at the end of each `arch_scan` invocation, with each invocation being a discrete tool call; the race surface is between concurrent tool calls on the same alias, which is rare in practice. Neither would survive a serious multi-process scenario without changes.

## Honest verdict on porting

**Worth porting from claude-context: nothing immediate.** The "Merkle DAG" they advertise does not behave like a Merkle DAG at runtime; it is a hash set with extra steps. The change-detection logic in `filestate.go::DetectChanges` is already the simpler and faster version of what their `synchronizer.checkForChanges` does.

**Not worth porting:** the root-hash node, the parent/child link metadata, the snapshot's redundant DAG-plus-flat-map storage. All overhead, no behavior.

**Real gaps in both systems:**

- **Sub-file granularity.** Neither hashes per-symbol or per-AST-node. A one-line change in a 2000-line file invalidates the whole file's cache. For `ridge` this means re-parsing and re-emitting all Nodes and Edges from that file, even though most are stable. The interesting Merkle design that would matter is per-symbol: hash each top-level declaration, build a small per-file DAG of declarations, propagate hashes upward only when a declaration changes. That would let `arch_scan` reuse stable Nodes from a partially-changed file.
- **Rename detection.** Both treat a rename as delete + add. For `ridge`, a rename triggers full re-analysis of the file even when content is byte-identical. A content-keyed index alongside the path-keyed one would let renames update metadata without re-analysis.
- **Concurrency.** Both will lose work under genuine concurrent writes. A simple advisory file lock around the snapshot save would close the obvious race.

## If sub-file change detection becomes interesting

The claude-context implementation is not a useful starting point for that work. A better reference would be a real Merkle tree that exploits the recursive property: child hash change forces parent re-hash, root comparison is O(1) and short-circuits. Tools like `git`'s tree objects do this; the structural inspiration there is closer to what would actually pay off.

## References

`ridge`:

- `internal/scanner/filestate.go` (full file, 156 lines)
- `internal/scanner/scanner.go:30,58` (ScanState integration)
- `internal/infra/persist.go` (JSON load/save helpers)
- `tools/handlers.go:183-188` (state load on `arch_scan`)

`zilliztech/claude-context`:

- `packages/core/src/sync/merkle.ts` (DAG type and compare logic)
- `packages/core/src/sync/synchronizer.ts` (file walking, hashing, snapshot I/O)
- `packages/core/src/context.ts` (orchestrator calling synchronizer + reindex pipeline)
- README claims vs. observed behavior: divergence noted in the "What 'Merkle DAG' actually means" section above.
