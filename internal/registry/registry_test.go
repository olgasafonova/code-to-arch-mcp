package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRegistry creates a registry pointing at a temp dir.
func setupTestRegistry(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	return &Registry{
		Version: "1",
		Repos:   make(map[string]Repo),
		dir:     dir,
	}
}

func TestAddAndGet(t *testing.T) {
	reg := setupTestRegistry(t)

	if err := reg.Add("myrepo", "/tmp/myrepo"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	repo, err := reg.Get("myrepo")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if repo.Path != "/tmp/myrepo" {
		t.Errorf("got path %q, want /tmp/myrepo", repo.Path)
	}
	if repo.AddedAt.IsZero() {
		t.Error("AddedAt should be set")
	}
}

func TestAddDuplicateAlias(t *testing.T) {
	reg := setupTestRegistry(t)

	if err := reg.Add("myrepo", "/tmp/myrepo"); err != nil {
		t.Fatalf("first Add failed: %v", err)
	}
	err := reg.Add("myrepo", "/tmp/other")
	if err == nil {
		t.Fatal("expected error for duplicate alias")
	}
}

// TestValidateAlias_RejectsPathTraversal is a regression test for the
// Carlini-scaffold finding: archRegistryAdd fed any string into Add as an
// alias, and StatePath later did filepath.Join(stateDir, alias+".json"). An
// alias of "../../tmp/victim" would let scan write/delete /tmp/victim.json.
func TestValidateAlias_RejectsPathTraversal(t *testing.T) {
	cases := []struct {
		name  string
		alias string
	}{
		{"empty", ""},
		{"dotdot", ".."},
		{"dotdot_traversal", "../../etc/foo"},
		{"forward_slash", "foo/bar"},
		{"backward_slash", `foo\bar`},
		{"leading_dot", ".hidden"},
		{"leading_hyphen", "-flag"},
		{"null_byte", "foo\x00bar"},
		{"newline", "foo\nbar"},
		{"space", "foo bar"},
		{"too_long", strings.Repeat("a", MaxAliasLen+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateAlias(tc.alias); err == nil {
				t.Errorf("expected ValidateAlias(%q) to error, got nil", tc.alias)
			}
		})
	}
}

func TestValidateAlias_AcceptsCommonShapes(t *testing.T) {
	for _, alias := range []string{"myrepo", "my-repo", "my_repo", "repo.v2", "MyRepo123", "a"} {
		if err := ValidateAlias(alias); err != nil {
			t.Errorf("ValidateAlias(%q) unexpectedly errored: %v", alias, err)
		}
	}
}

// TestAdd_RejectsBadAlias confirms validation runs at the Add boundary so
// callers that bypass archRegistryAdd's pre-check still cannot escape.
func TestAdd_RejectsBadAlias(t *testing.T) {
	reg := setupTestRegistry(t)
	if err := reg.Add("../../etc/passwd", "/tmp/x"); err == nil {
		t.Fatal("expected Add to reject path-traversal alias")
	}
	// Confirm StatePath isn't tricked: bad alias never makes it into the map.
	if _, ok := reg.Repos["../../etc/passwd"]; ok {
		t.Fatal("registry retained bad alias despite Add error")
	}
}

func TestGetNotFound(t *testing.T) {
	reg := setupTestRegistry(t)

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing alias")
	}
}

func TestRemove(t *testing.T) {
	reg := setupTestRegistry(t)

	if err := reg.Add("myrepo", "/tmp/myrepo"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Create a fake state file so we can verify cleanup.
	stateFile := reg.StatePath("myrepo")
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(stateFile, []byte("{}"), 0644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	if err := reg.Remove("myrepo"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	_, err := reg.Get("myrepo")
	if err == nil {
		t.Error("expected error after removal")
	}

	if _, statErr := os.Stat(stateFile); !os.IsNotExist(statErr) {
		t.Error("state file should have been deleted")
	}
}

func TestRemoveNotFound(t *testing.T) {
	reg := setupTestRegistry(t)

	err := reg.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing alias")
	}
}

func TestList(t *testing.T) {
	reg := setupTestRegistry(t)

	// Add one with a real path and one with a non-existent path.
	realDir := t.TempDir()
	if err := reg.Add("real", realDir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := reg.Add("gone", "/nonexistent/path/xyz"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	entries := reg.List()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	byAlias := make(map[string]RepoEntry)
	for _, e := range entries {
		byAlias[e.Alias] = e
	}

	if byAlias["real"].Stale {
		t.Error("real repo should not be stale")
	}
	if !byAlias["gone"].Stale {
		t.Error("gone repo should be stale")
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	reg := &Registry{
		Version: "1",
		Repos:   make(map[string]Repo),
		dir:     dir,
	}

	if err := reg.Add("test", "/tmp/test"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	reg.UpdateScanInfo("test", 10, 20, "monolith")

	if err := reg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload by reading the file directly.
	path := filepath.Join(dir, registryFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var loaded Registry
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	loaded.dir = dir

	repo, err := loaded.Get("test")
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if repo.NodeCount != 10 {
		t.Errorf("NodeCount = %d, want 10", repo.NodeCount)
	}
	if repo.EdgeCount != 20 {
		t.Errorf("EdgeCount = %d, want 20", repo.EdgeCount)
	}
	if repo.Topology != "monolith" {
		t.Errorf("Topology = %q, want monolith", repo.Topology)
	}
	if repo.LastScan.IsZero() {
		t.Error("LastScan should be set after UpdateScanInfo")
	}
}

func TestUpdateScanInfoMissing(t *testing.T) {
	reg := setupTestRegistry(t)
	// Should not panic on missing alias.
	reg.UpdateScanInfo("nonexistent", 1, 2, "mono")
}

func TestStatePath(t *testing.T) {
	reg := setupTestRegistry(t)
	sp := reg.StatePath("myrepo")
	if filepath.Base(sp) != "myrepo.json" {
		t.Errorf("StatePath base = %q, want myrepo.json", filepath.Base(sp))
	}
	if filepath.Base(filepath.Dir(sp)) != stateSubdir {
		t.Errorf("StatePath parent dir = %q, want %s", filepath.Base(filepath.Dir(sp)), stateSubdir)
	}
}
