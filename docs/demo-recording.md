# Demo recording guide

Companion to [bead `eh1`](../README.md#why) (Add demo GIF to README).
The README has a TODO at line 17 reserving space for a 30-second demo GIF.
This file documents exactly what to record and how to convert it to `docs/demo.gif`.

The recording itself is a manual step — a side-by-side terminal + browser
screen capture is not scriptable from this repo without GUI automation that
isn't worth the engineering effort.

## What to record

A 30-second clip showing the tool turning code into a graph in real time.

1. Pick the target. A medium repo (50-500 files) plus a markdown directory
   makes the dual-substrate story clear. Suggestions:
   - Code: this repo (`~/Projects/code-to-arch-mcp`) or any public Go project
   - Markdown: a small Obsidian vault or `docs/` of an OSS project
2. Open Claude Code (or another MCP client) on the left half of the screen.
3. Open a browser on the right half. Keep it on a blank tab to start.
4. Start the screen recording (`Cmd+Shift+5` on macOS, choose "Record selected portion").
5. In Claude Code, type one of:
   - `Scan ~/Projects/<target> and generate a forcegraph diagram`
   - `Scan this repo and render a forcegraph showing hubs at degree>=5`
6. Wait for `arch_scan` and `arch_generate` to finish. Claude will write
   the rendered HTML somewhere (typically `/tmp/`) and tell you the path.
7. Drag-and-drop the HTML file into the browser, or `open <path>` in another
   terminal. The forcegraph appears with drag/zoom interaction.
8. Drag a couple of nodes around to show interactivity. Don't over-explore.
9. Stop the recording.

Aim for under 30 seconds total. The clip should answer "what does this
tool do?" in one viewing.

## Converting to GIF

macOS `Cmd+Shift+5` produces an `.mov` file. Convert with `ffmpeg`:

```bash
# Single-step conversion at 12fps, scaled to 1200px wide
ffmpeg -i screen-recording.mov \
  -vf "fps=12,scale=1200:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" \
  -loop 0 \
  docs/demo.gif

# If the GIF is over ~5 MB, drop fps to 8 or scale to 900px
gifsicle -O3 docs/demo.gif -o docs/demo.gif
```

Target file size: under 5 MB so it loads quickly on the README.

## Wiring the GIF into the README

After `docs/demo.gif` exists, replace the TODO comment in `README.md`:

```markdown
<!-- TODO: Add demo GIF showing arch_scan + arch_generate on a real project -->
```

with:

```markdown
![demo](docs/demo.gif)
```

Then update bead `eh1`:

```bash
bd close code-to-arch-mcp-eh1 --reason="Recorded demo GIF on <date>, embedded in README"
```

## Why the recording is manual

The demo's value is the visual contrast between the JSON output (left) and
the rendered force-directed graph (right). Capturing both requires a screen
recorder; terminal-only recorders (vhs, asciinema) cannot show the browser.
A Playwright-driven recording could capture the browser side but not the
live MCP tool call. The manual approach takes 5 minutes and produces a
better artifact.
