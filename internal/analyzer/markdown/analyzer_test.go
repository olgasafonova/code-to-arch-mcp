package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
)

func writeMD(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtensions(t *testing.T) {
	a := New()
	exts := a.Extensions()
	if len(exts) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(exts))
	}
	want := map[string]bool{".md": true, ".markdown": true}
	for _, e := range exts {
		if !want[e] {
			t.Fatalf("unexpected extension %q", e)
		}
	}
}

func TestLanguage(t *testing.T) {
	a := New()
	if got := a.Language(); got != "markdown" {
		t.Fatalf("expected markdown, got %s", got)
	}
}

func TestAnalyze_NoteNode(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "topic.md", "# Topic\n\nNo links.\n")

	nodes, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != model.NodeNote {
		t.Fatalf("want NodeNote, got %s", n.Type)
	}
	if n.Name != "topic" {
		t.Fatalf("want name=topic, got %s", n.Name)
	}
	if n.Language != "markdown" {
		t.Fatalf("want language=markdown, got %s", n.Language)
	}
	if len(edges) != 0 {
		t.Fatalf("want 0 edges, got %d", len(edges))
	}
}

func TestAnalyze_WikiLink(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "a.md", "See [[other-note]] for context.\n")

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(edges))
	}
	if edges[0].Target != "wikilink:other-note" {
		t.Fatalf("want wikilink:other-note, got %s", edges[0].Target)
	}
	if edges[0].Type != model.EdgeDependency {
		t.Fatalf("want EdgeDependency, got %s", edges[0].Type)
	}
}

func TestAnalyze_WikiLinkWithDisplayAndAnchor(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "a.md", "Check [[other#section|display text]] please.\n")

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(edges))
	}
	if edges[0].Target != "wikilink:other" {
		t.Fatalf("want wikilink:other, got %s", edges[0].Target)
	}
	if edges[0].Label != "display text" {
		t.Fatalf("want label=display text, got %q", edges[0].Label)
	}
}

func TestAnalyze_FolderQualifiedWikiLink(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "a.md", "Cross-ref: [[topics/Foo]] and [[topics/Bar|alias]].\n")

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	targets := make(map[string]bool)
	for _, e := range edges {
		targets[e.Target] = true
	}
	if !targets["wikilink:topics/Foo"] {
		t.Fatal("missing wikilink:topics/Foo")
	}
	if !targets["wikilink:topics/Bar"] {
		t.Fatal("missing wikilink:topics/Bar")
	}
}

func TestAnalyze_RelativeMarkdownLink(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "a.md", "See [the doc](./folder/other.md) for details.\n")

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(edges))
	}
	if edges[0].Target != "wikilink:other" {
		t.Fatalf("want wikilink:other, got %s", edges[0].Target)
	}
	if edges[0].Label != "the doc" {
		t.Fatalf("want label=the doc, got %q", edges[0].Label)
	}
}

func TestAnalyze_SkipsExternalAndAnchorLinks(t *testing.T) {
	dir := t.TempDir()
	content := `# Skip these
See [Google](https://google.com).
Anchor: [section](#intro).
Email: [me](mailto:me@example.com).
Image: [pic](image.png).
Local doc: [other](other.md).
`
	path := writeMD(t, dir, "a.md", content)

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 edge (only other.md), got %d", len(edges))
	}
	if edges[0].Target != "wikilink:other" {
		t.Fatalf("want wikilink:other, got %s", edges[0].Target)
	}
}

func TestAnalyze_SkipsLinksInFencedCodeBlocks(t *testing.T) {
	dir := t.TempDir()
	content := "Real: [[real-note]]\n" +
		"```\n" +
		"[[fake-note]] should be ignored\n" +
		"[skip](skip.md)\n" +
		"```\n" +
		"After: [[after-note]]\n"
	path := writeMD(t, dir, "a.md", content)

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	targets := make(map[string]bool)
	for _, e := range edges {
		targets[e.Target] = true
	}
	if !targets["wikilink:real-note"] || !targets["wikilink:after-note"] {
		t.Fatalf("missing real-note or after-note: %v", targets)
	}
	if targets["wikilink:fake-note"] || targets["wikilink:skip"] {
		t.Fatalf("found link from inside code block: %v", targets)
	}
}

func TestAnalyze_SkipsLinksInTildeFencedBlocks(t *testing.T) {
	dir := t.TempDir()
	content := "Before: [[real]]\n~~~\n[[fake]]\n~~~\nAfter: [[also-real]]\n"
	path := writeMD(t, dir, "a.md", content)

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	for _, e := range edges {
		if e.Target == "wikilink:fake" {
			t.Fatal("link inside ~~~ block should be ignored")
		}
	}
}

func TestAnalyze_SkipsLinksInInlineCode(t *testing.T) {
	dir := t.TempDir()
	content := "Real: [[real]]. In code: `[[fake]]` and `[skip](skip.md)`.\n"
	path := writeMD(t, dir, "a.md", content)

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	for _, e := range edges {
		if strings.Contains(e.Target, "fake") || strings.Contains(e.Target, "skip") {
			t.Fatalf("link inside inline code should be ignored: %s", e.Target)
		}
	}
}

func TestAnalyze_DeduplicatesWithinFile(t *testing.T) {
	dir := t.TempDir()
	content := "First: [[same]]. Second: [[same]]. Third: [[same|different label]].\n"
	path := writeMD(t, dir, "a.md", content)

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 deduped edge, got %d", len(edges))
	}
}

func TestAnalyze_MultipleDistinctLinks(t *testing.T) {
	dir := t.TempDir()
	content := `# Hub
- [[note-a]]
- [[note-b]]
- [[note-c]]
- [doc](./folder/other.md)
`
	path := writeMD(t, dir, "hub.md", content)

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 4 {
		t.Fatalf("want 4 edges, got %d", len(edges))
	}
}

func TestAnalyze_MarkdownExtension(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "x.markdown", "Has [[link]].\n")

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 edge, got %d", len(edges))
	}
}

func TestAnalyze_EmptyAndEdgeCases(t *testing.T) {
	dir := t.TempDir()
	path := writeMD(t, dir, "edge.md", "[[]] [[ ]] []() [text]() [text](    )\n")

	_, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("want 0 edges from malformed input, got %d: %+v", len(edges), edges)
	}
}

func TestAnalyze_MissingFile(t *testing.T) {
	_, _, err := New().Analyze("/no/such/path.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestAnalyze_OversizedFileSkipsLinkExtraction(t *testing.T) {
	dir := t.TempDir()
	big := make([]byte, maxFileBytes+1024)
	for i := range big {
		big[i] = 'x'
	}
	// Insert a wiki-link that should be ignored because the file is over cap.
	copy(big[100:], []byte("[[should-not-be-extracted]]"))
	path := filepath.Join(dir, "big.md")
	if err := os.WriteFile(path, big, 0644); err != nil {
		t.Fatal(err)
	}

	nodes, edges, err := New().Analyze(path)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node (the note itself), got %d", len(nodes))
	}
	if nodes[0].Properties["skipped"] != "size" {
		t.Fatalf("expected skipped=size property, got %v", nodes[0].Properties)
	}
	if len(edges) != 0 {
		t.Fatalf("want 0 edges (over cap), got %d", len(edges))
	}
}

func TestStripCodeBlocks_NestedFencesDifferentSymbols(t *testing.T) {
	in := "before\n```\n~~~ inside backticks\n```\nafter\n"
	got := stripCodeBlocks(in)
	if strings.Contains(got, "inside backticks") {
		t.Fatalf("inner content should be stripped: %q", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("outer content should be preserved: %q", got)
	}
}
