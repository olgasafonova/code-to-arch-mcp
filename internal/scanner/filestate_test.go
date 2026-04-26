package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/olgasafonova/ridge/internal/model"
)

func TestDetectChanges_NewFiles(t *testing.T) {
	state := NewScanState("/tmp/test")

	dir := t.TempDir()
	f1 := writeTestFile(t, dir, "a.go", "package a")
	f2 := writeTestFile(t, dir, "b.go", "package b")

	cs, err := state.DetectChanges([]string{f1, f2})
	if err != nil {
		t.Fatal(err)
	}

	if len(cs.Added) != 2 {
		t.Errorf("expected 2 added, got %d", len(cs.Added))
	}
	if len(cs.Unchanged) != 0 || len(cs.Modified) != 0 || len(cs.Deleted) != 0 {
		t.Errorf("unexpected non-empty sets: unchanged=%d modified=%d deleted=%d",
			len(cs.Unchanged), len(cs.Modified), len(cs.Deleted))
	}
}

func TestDetectChanges_UnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTestFile(t, dir, "a.go", "package a")

	state := NewScanState(dir)
	if err := state.UpdateFile(f1, nil, nil); err != nil {
		t.Fatal(err)
	}

	cs, err := state.DetectChanges([]string{f1})
	if err != nil {
		t.Fatal(err)
	}

	if len(cs.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged, got %d", len(cs.Unchanged))
	}
	if len(cs.Added) != 0 || len(cs.Modified) != 0 {
		t.Errorf("unexpected: added=%d modified=%d", len(cs.Added), len(cs.Modified))
	}
}

func TestDetectChanges_ModifiedContent(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTestFile(t, dir, "a.go", "package a")

	state := NewScanState(dir)
	if err := state.UpdateFile(f1, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Modify the file content
	time.Sleep(10 * time.Millisecond) // ensure mtime changes
	if err := os.WriteFile(f1, []byte("package a\n// modified"), 0644); err != nil {
		t.Fatal(err)
	}

	cs, err := state.DetectChanges([]string{f1})
	if err != nil {
		t.Fatal(err)
	}

	if len(cs.Modified) != 1 {
		t.Errorf("expected 1 modified, got %d", len(cs.Modified))
	}
}

func TestDetectChanges_TouchedButSameContent(t *testing.T) {
	dir := t.TempDir()
	content := "package a"
	f1 := writeTestFile(t, dir, "a.go", content)

	state := NewScanState(dir)
	if err := state.UpdateFile(f1, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Touch the file (change mtime, same content)
	time.Sleep(10 * time.Millisecond)
	now := time.Now()
	if err := os.Chtimes(f1, now, now); err != nil {
		t.Fatal(err)
	}

	cs, err := state.DetectChanges([]string{f1})
	if err != nil {
		t.Fatal(err)
	}

	if len(cs.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged (touch with same content), got unchanged=%d modified=%d",
			len(cs.Unchanged), len(cs.Modified))
	}
}

func TestDetectChanges_DeletedFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTestFile(t, dir, "a.go", "package a")

	state := NewScanState(dir)
	if err := state.UpdateFile(f1, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Detect with empty walked set = f1 was deleted
	cs, err := state.DetectChanges(nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(cs.Deleted) != 1 {
		t.Errorf("expected 1 deleted, got %d", len(cs.Deleted))
	}
}

func TestUpdateFile_StoresNodesAndEdges(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTestFile(t, dir, "a.go", "package a")

	state := NewScanState(dir)
	nodes := []*model.Node{{ID: "n1", Name: "TestNode", Type: model.NodePackage}}
	edges := []*model.Edge{{Source: "n1", Target: "n2", Type: model.EdgeDependency}}

	if err := state.UpdateFile(f1, nodes, edges); err != nil {
		t.Fatal(err)
	}

	gotNodes, gotEdges, ok := state.CachedResult(f1)
	if !ok {
		t.Fatal("CachedResult returned false")
	}
	if len(gotNodes) != 1 || gotNodes[0].ID != "n1" {
		t.Errorf("unexpected nodes: %+v", gotNodes)
	}
	if len(gotEdges) != 1 || gotEdges[0].Source != "n1" {
		t.Errorf("unexpected edges: %+v", gotEdges)
	}
}

func TestRemoveFile(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTestFile(t, dir, "a.go", "package a")

	state := NewScanState(dir)
	_ = state.UpdateFile(f1, nil, nil)

	state.RemoveFile(f1)
	_, _, ok := state.CachedResult(f1)
	if ok {
		t.Error("expected CachedResult to return false after RemoveFile")
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTestFile(t, dir, "a.go", "hello")
	f2 := writeTestFile(t, dir, "b.go", "hello")
	f3 := writeTestFile(t, dir, "c.go", "world")

	h1, _ := hashFile(f1)
	h2, _ := hashFile(f2)
	h3, _ := hashFile(f3)

	if h1 != h2 {
		t.Error("same content should have same hash")
	}
	if h1 == h3 {
		t.Error("different content should have different hash")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(h1))
	}
}

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
