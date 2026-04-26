package detector

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
)

func TestNormalizeCandidate(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"internal/scanner", "internal/scanner"},
		{"internal/scanner/", "internal/scanner"},
		{"internal/scanner/scanner.go", "internal/scanner"},
		{"tools/handlers.go:183-188", ""}, // strips to "tools", single-segment, rejected
		{"tools/handlers.go:42", ""},      // same
		{"single", ""},
		{"foo bar/baz", ""},
		{"a/b", "a/b"},
		{"src/feature.ts", ""}, // strips to "src", single-segment, rejected
		{"path/to/something/file.py", "path/to/something"},
		{"deep/nested/scanner/scanner.go", "deep/nested/scanner"},
	}
	for _, c := range cases {
		got := normalizeCandidate(c.in)
		if got != c.want {
			t.Errorf("normalizeCandidate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPathSuffixes(t *testing.T) {
	got := pathSuffixes("/abs/repo/internal/scanner")
	wantContains := []string{"internal/scanner", "repo/internal/scanner", "abs/repo/internal/scanner"}
	for _, w := range wantContains {
		if !slices.Contains(got, w) {
			t.Errorf("expected suffix %q in %v", w, got)
		}
	}
	// Should never include the bare leaf alone.
	for _, g := range got {
		if g == "scanner" {
			t.Errorf("bare leaf segment %q should not be indexed: %v", g, got)
		}
	}
}

func TestLinkNotesToPackages_Smoke(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "design.md")
	noteSrc := "Stuff lives in `internal/scanner/`. The orchestrator calls into `internal/render/forcegraph.go`. " +
		"Unrelated text mentions internal/scanner without backticks (should be ignored)."
	if err := os.WriteFile(notePath, []byte(noteSrc), 0o600); err != nil {
		t.Fatal(err)
	}

	graph := model.NewGraph(dir)
	graph.AddNode(&model.Node{
		ID:   "note:design/design",
		Name: "design",
		Type: model.NodeNote,
		Path: notePath,
	})
	graph.AddNode(&model.Node{
		ID:   "pkg:scanner/scanner",
		Name: "scanner",
		Type: model.NodePackage,
		Path: filepath.Join(dir, "internal", "scanner"),
	})
	graph.AddNode(&model.Node{
		ID:   "pkg:render/render",
		Name: "render",
		Type: model.NodePackage,
		Path: filepath.Join(dir, "internal", "render"),
	})

	added := LinkNotesToPackages(graph)
	if added != 2 {
		t.Errorf("expected 2 cross-substrate edges, got %d", added)
	}

	// Verify edges exist with the expected metadata.
	var found int
	for _, e := range graph.Edges() {
		if e.Source == "note:design/design" && e.Label == "documents" {
			if e.Confidence != 0.6 {
				t.Errorf("expected confidence 0.6, got %v", e.Confidence)
			}
			found++
		}
	}
	if found != 2 {
		t.Errorf("expected 2 edges with label 'documents', got %d", found)
	}
}

func TestLinkNotesToPackages_NoNotesNoOp(t *testing.T) {
	graph := model.NewGraph("/tmp")
	graph.AddNode(&model.Node{ID: "pkg:a/a", Type: model.NodePackage, Path: "/tmp/a"})
	if added := LinkNotesToPackages(graph); added != 0 {
		t.Errorf("expected 0 edges, got %d", added)
	}
}

func TestLinkNotesToPackages_DedupesWithinFile(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "n.md")
	src := "Mention `internal/scanner` once. Mention `internal/scanner` again. And `internal/scanner/`."
	if err := os.WriteFile(notePath, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	graph := model.NewGraph(dir)
	graph.AddNode(&model.Node{
		ID:   "note:foo/n",
		Type: model.NodeNote,
		Path: notePath,
	})
	graph.AddNode(&model.Node{
		ID:   "pkg:scanner/scanner",
		Type: model.NodePackage,
		Path: filepath.Join(dir, "internal", "scanner"),
	})

	added := LinkNotesToPackages(graph)
	if added != 1 {
		t.Errorf("expected 1 deduped edge, got %d", added)
	}
}
