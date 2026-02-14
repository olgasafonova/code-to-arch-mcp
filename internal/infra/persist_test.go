package infra

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateDir(t *testing.T) {
	dir, err := StateDir("test-server")
	if err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".mcp-context", "test-server")
	if dir != want {
		t.Errorf("StateDir = %q, want %q", dir, want)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal("directory not created:", err)
	}
	if !info.IsDir() {
		t.Fatal("not a directory")
	}

	// Cleanup
	_ = os.Remove(dir)
}

func TestLoadSaveJSON(t *testing.T) {
	type testData struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	// Save
	v := &testData{Name: "hello", Count: 42}
	if err := SaveJSON(path, v); err != nil {
		t.Fatal("SaveJSON:", err)
	}

	// Load
	got, err := LoadJSON[testData](path)
	if err != nil {
		t.Fatal("LoadJSON:", err)
	}
	if got.Name != v.Name || got.Count != v.Count {
		t.Errorf("LoadJSON = %+v, want %+v", got, v)
	}

	// Load missing file
	_, err = LoadJSON[testData](filepath.Join(dir, "missing.json"))
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist for missing file, got %v", err)
	}
}

func TestSaveJSON_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "test.json")

	v := struct{ X int }{X: 1}
	if err := SaveJSON(path, &v); err != nil {
		t.Fatal("SaveJSON with nested dirs:", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatal("file not created:", err)
	}
}
