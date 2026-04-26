# Rename: code-to-arch-mcp → ridge

Bead: `code-to-arch-mcp-mjl`. Run top-to-bottom; each phase is reversible up to the GitHub repo rename.

## Pre-flight

- [ ] Confirm `github.com/olgasafonova/ridge` still available
- [ ] Confirm no in-flight PRs against `code-to-arch-mcp` (would need rebasing post-rename)
- [ ] Notify any external users (if any exist beyond own machine)
- [ ] Cut a final `code-to-arch-mcp` release tag for fallback

## Phase 1: Local source tree

- [ ] Update `go.mod` module path: `github.com/olgasafonova/code-to-arch-mcp` → `github.com/olgasafonova/ridge`
- [ ] Find/replace import paths across all Go files: `goimports -w` after sed
- [ ] Rename binary directory: `cmd/code-to-arch/` → `cmd/ridge/`
- [ ] Update `Makefile` build targets (binary name, install target)
- [ ] Run `make check` — expect green
- [ ] Run `golangci-lint run ./...` — expect clean

## Phase 2: Documentation

- [ ] `README.md`: title, install snippet, all `code-to-arch` references → `ridge`
- [ ] `CLAUDE.md`: rewrite project section with new name
- [ ] `AGENTS.md`: update if it references old name
- [ ] All `docs/*.md`: search/replace
- [ ] Badges: Go Report Card URL, license badge URL
- [ ] Add a `RENAME.md` short note at repo root explaining the rename for redirected visitors

## Phase 3: Persistent state migration

- [ ] State directory: `~/.mcp-context/code-to-arch/` → `~/.mcp-context/ridge/`
  - [ ] Add migration code in `internal/infra/persist.go` to copy old → new on first run if old exists
  - [ ] Or: ship a one-time `ridge migrate-state` subcommand
- [ ] Snapshot files: existing `*.snapshot.json` in old location need to move

## Phase 4: MCP config

- [ ] Update own `~/.claude.json`: server name `code-to-arch` → `ridge`, command path `code-to-arch` → `ridge`
- [ ] Restart Claude Code to pick up new binary
- [ ] Verify all 19 tools still register; test with `arch_scan` on a small repo

## Phase 5: Beads

- [ ] All future beads create with `ridge-` namespace (auto via bd config)
- [ ] Existing `code-to-arch-mcp-*` beads stay as-is (history preservation)
- [ ] Cross-reference table at bottom of this doc maps old → new bead IDs if/when needed

## Phase 6: GitHub

- [ ] Repo settings → rename `code-to-arch-mcp` → `ridge`
  - GitHub auto-redirects old URL to new for 1+ year (per docs)
- [ ] Update repo description and topics (replace `code-to-architecture` with `architecture-extraction` etc.)
- [ ] Update local git remote: `git remote set-url origin git@github.com:olgasafonova/ridge.git`
- [ ] Force-push `main` after merge — wait, no, just `git push` — remote redirect handles it
- [ ] Reset Go module cache for self-test: `go clean -modcache` (drastic) or wait for cache TTL

## Phase 7: Release

- [ ] Tag `v0.x.0` (next semver) on `ridge` repo
- [ ] Run `/ship` to publish release with rename note in changelog
- [ ] `go install github.com/olgasafonova/ridge/cmd/ridge@latest` — verify clean install works

## Phase 8: External surfaces (post-rename)

- [ ] Anthropic plugin marketplace submission (different bead: `code-to-arch-mcp-k07`) now targets `ridge`
- [ ] Substack post "I sifted my own MCP server" can lead with the rename arc as the second-act move
- [ ] Update LinkedIn / personal portfolio mentions

## Rollback

If rename causes user-visible breakage:

1. GitHub repo can be renamed back (GitHub keeps the old URL redirected; just rename `ridge` → `code-to-arch-mcp` in settings)
2. Go module path: revert `go.mod`, find/replace imports back
3. State dir migration: copy `~/.mcp-context/ridge/` back to `~/.mcp-context/code-to-arch/`
4. ~/.claude.json: revert server name + binary path

Cost of rollback is low because rename window is also low-traffic (1 star, 0 external users). Worst case: a few hours of revert + republish.

## Bead cross-references

| Old bead | New bead (post-rename) | Status |
|---|---|---|
| code-to-arch-mcp-mjl | ridge-? | this rename |
| code-to-arch-mcp-vzk | ridge-? (multi-path scan) | open |
| code-to-arch-mcp-dqz | ridge-? (demo GIF) | open |
| code-to-arch-mcp-hva | ridge-? (Why not X README) | open |
| code-to-arch-mcp-3n6 | ridge-? (benchmark file) | open |
| code-to-arch-mcp-k07 | ridge-? (marketplace submission) | open |
| code-to-arch-mcp-f5p | ridge-? (min_degree warn) | closed |
