// Package markdown provides markdown analysis for architecture graphs.
// Each .md file becomes a NodeNote; wiki-links and relative markdown links become EdgeDependency edges.
package markdown

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/olgasafonova/ridge/internal/model"
	"github.com/olgasafonova/ridge/internal/scanner"
)

// Analyzer implements the scanner.Analyzer interface for markdown files.
type Analyzer struct{}

// New creates a markdown analyzer.
func New() *Analyzer {
	return &Analyzer{}
}

// Extensions returns markdown file extensions.
func (a *Analyzer) Extensions() []string {
	return []string{".md", ".markdown"}
}

// Language returns "markdown".
func (a *Analyzer) Language() string {
	return "markdown"
}

// Clone returns an independent copy. The markdown analyzer is stateless.
func (a *Analyzer) Clone() scanner.Analyzer {
	return New()
}

var (
	// Wiki-link: [[target]] or [[target|display]] or [[target#section]] or [[target#section|display]].
	// Captures the full inside content; we split on # and | downstream.
	wikiLinkRe = regexp.MustCompile(`\[\[([^\[\]\n]+?)\]\]`)

	// Standard markdown link: [text](target) — target stops at whitespace or close paren.
	// Title attribute "..." after target is tolerated by the trailing optional group.
	mdLinkRe = regexp.MustCompile(`\[([^\]\n]*)\]\(([^\s)]+)(?:\s+"[^"]*")?\)`)

	// Fenced code block: ``` or ~~~ on its own line, possibly with a language tag.
	fenceOpenRe = regexp.MustCompile("(?m)^[ \\t]*(```+|~~~+)[^\\n]*$")
)

// maxFileBytes caps how much of any one markdown file we read. Vaults
// occasionally contain pasted full-text articles or image-embedded notes that
// can balloon to multi-MB; reading them entirely into memory and then running
// regexes over them stalls the scanner with no useful link signal in return.
const maxFileBytes = 5 * 1024 * 1024 // 5 MB

// Analyze parses a markdown file and emits a note node + link edges.
func (a *Analyzer) Analyze(path string) ([]*model.Node, []*model.Edge, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s: %w", path, err)
	}

	dir := filepath.Base(filepath.Dir(path))
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	noteID := fmt.Sprintf("note:%s/%s", dir, stem)

	if info.Size() > maxFileBytes {
		// Emit the note node so the file is visible in the graph but skip
		// link extraction. Properties record the reason for downstream tools.
		return []*model.Node{{
			ID:       noteID,
			Name:     stem,
			Type:     model.NodeNote,
			Language: "markdown",
			Path:     path,
			Properties: map[string]string{
				"skipped":      "size",
				"size_bytes":   fmt.Sprintf("%d", info.Size()),
				"max_capacity": fmt.Sprintf("%d", maxFileBytes),
			},
		}}, nil, nil
	}

	src, err := os.ReadFile(path) //nolint:gosec // G304: path is from filesystem walker, not user input
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	nodes := []*model.Node{
		{
			ID:       noteID,
			Name:     stem,
			Type:     model.NodeNote,
			Language: "markdown",
			Path:     path,
		},
	}

	stripped := stripCodeBlocks(string(src))
	stripped = stripInlineCode(stripped)

	var edges []*model.Edge
	seen := make(map[string]bool) // dedupe identical link targets within one file

	for _, m := range wikiLinkRe.FindAllStringSubmatch(stripped, -1) {
		inner := strings.TrimSpace(m[1])
		if inner == "" {
			continue
		}
		linkTarget, display := splitWikiTarget(inner)
		if linkTarget == "" {
			continue
		}
		key := "wiki|" + linkTarget
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, &model.Edge{
			Source:     noteID,
			Target:     "wikilink:" + linkTarget,
			Type:       model.EdgeDependency,
			Label:      display,
			Confidence: 0.9,
		})
	}

	for _, m := range mdLinkRe.FindAllStringSubmatch(stripped, -1) {
		display := strings.TrimSpace(m[1])
		raw := strings.TrimSpace(m[2])
		linkTarget, ok := relativeMarkdownTarget(raw)
		if !ok {
			continue
		}
		key := "md|" + linkTarget
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, &model.Edge{
			Source:     noteID,
			Target:     "wikilink:" + linkTarget,
			Type:       model.EdgeDependency,
			Label:      display,
			Confidence: 0.85,
		})
	}

	return nodes, edges, nil
}

// splitWikiTarget parses an Obsidian wiki-link inner string.
// Forms: "name", "name|display", "name#section", "name#section|display", "folder/name".
// Returns the link target (stripped of #section, |display) and the display text if present.
func splitWikiTarget(inner string) (target, display string) {
	if i := strings.Index(inner, "|"); i >= 0 {
		display = strings.TrimSpace(inner[i+1:])
		inner = inner[:i]
	}
	if i := strings.Index(inner, "#"); i >= 0 {
		inner = inner[:i]
	}
	target = strings.TrimSpace(inner)
	if display == "" {
		display = target
	}
	return target, display
}

// relativeMarkdownTarget extracts the basename (without extension) of a relative markdown link.
// Skips external URLs (scheme://...), mailto, same-page anchors, and non-.md targets.
func relativeMarkdownTarget(raw string) (string, bool) {
	if raw == "" || strings.HasPrefix(raw, "#") {
		return "", false
	}
	if strings.Contains(raw, "://") || strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "tel:") {
		return "", false
	}
	// Strip any fragment / query.
	if i := strings.IndexAny(raw, "#?"); i >= 0 {
		raw = raw[:i]
	}
	if raw == "" {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(raw))
	if ext != ".md" && ext != ".markdown" {
		return "", false
	}
	stem := strings.TrimSuffix(filepath.Base(raw), filepath.Ext(raw))
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return "", false
	}
	return stem, true
}

// stripCodeBlocks removes fenced code blocks (``` or ~~~) so links inside them are ignored.
// Matching is fence-symbol aware: a ``` block can only be closed by ``` (and ~~~ by ~~~),
// preventing a stray fence inside a code block from prematurely re-opening parsing.
func stripCodeBlocks(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inBlock := false
	var fenceChar byte
	for _, line := range lines {
		if !inBlock {
			if loc := fenceOpenRe.FindStringIndex(line); loc != nil {
				trimmed := strings.TrimLeft(line, " \t")
				if len(trimmed) > 0 {
					fenceChar = trimmed[0]
				}
				inBlock = true
				out = append(out, "")
				continue
			}
			out = append(out, line)
			continue
		}
		// In a block: only the matching fence character closes it.
		trimmed := strings.TrimLeft(line, " \t")
		if (fenceChar == '`' && strings.HasPrefix(trimmed, "```")) ||
			(fenceChar == '~' && strings.HasPrefix(trimmed, "~~~")) {
			inBlock = false
		}
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// stripInlineCode replaces backtick-delimited inline code spans with spaces of equal length,
// so links inside `code` are ignored without shifting line/col positions.
func stripInlineCode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '`' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// Count opening backticks.
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		runLen := j - i
		// Find matching closing run of the same length.
		end := -1
		k := j
		for k < len(s) {
			if s[k] != '`' {
				k++
				continue
			}
			m := k
			for m < len(s) && s[m] == '`' {
				m++
			}
			if m-k == runLen {
				end = k
				break
			}
			k = m
		}
		if end == -1 {
			// No closing fence; emit literally.
			b.WriteString(s[i:j])
			i = j
			continue
		}
		// Replace span (including fences) with spaces, preserving newlines.
		for p := i; p < end+runLen; p++ {
			if s[p] == '\n' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		}
		i = end + runLen
	}
	return b.String()
}
