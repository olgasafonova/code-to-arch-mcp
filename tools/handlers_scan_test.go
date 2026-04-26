package tools

import (
	"context"
	"testing"
)

// TestArchScan_MutualExclusion verifies the validation rule:
// passing both path and paths returns a descriptive error.
func TestArchScan_MutualExclusion(t *testing.T) {
	h := NewHandlerRegistry(testLogger())

	_, err := h.archScan(context.Background(), ArchScanArgs{
		Path:  "/some/path",
		Paths: []string{"/other/path"},
	})
	if err == nil {
		t.Fatal("expected error when both path and paths are set, got nil")
	}
	want := "use either path or paths, not both"
	if err.Error() != want {
		t.Errorf("error = %q; want %q", err.Error(), want)
	}
}

// TestArchScan_MutualExclusion_RepoAndPaths verifies repo + paths is also rejected.
func TestArchScan_MutualExclusion_RepoAndPaths(t *testing.T) {
	h := NewHandlerRegistry(testLogger())

	_, err := h.archScan(context.Background(), ArchScanArgs{
		Repo:  "myrepo",
		Paths: []string{"/some/path"},
	})
	if err == nil {
		t.Fatal("expected error when both repo and paths are set, got nil")
	}
	want := "use either path or paths, not both"
	if err.Error() != want {
		t.Errorf("error = %q; want %q", err.Error(), want)
	}
}
