package safepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateScanPath_Valid(t *testing.T) {
	dir := t.TempDir()
	if err := ValidateScanPath(dir); err != nil {
		t.Fatalf("expected valid path, got: %v", err)
	}
}

func TestValidateScanPath_Empty(t *testing.T) {
	err := ValidateScanPath("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestValidateScanPath_NotExist(t *testing.T) {
	err := ValidateScanPath("/nonexistent/path/xyz123")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestValidateScanPath_NotDirectory(t *testing.T) {
	f, err := os.CreateTemp("", "safepath-test-*")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	if err := ValidateScanPath(f.Name()); err == nil {
		t.Fatal("expected error for file path")
	}
}

func TestValidateScanPath_SensitiveSystem(t *testing.T) {
	for _, dir := range []string{"/etc", "/proc", "/sys", "/dev"} {
		err := ValidateScanPath(dir)
		if err == nil {
			t.Fatalf("expected error for sensitive path %s", dir)
		}
	}
}

func TestValidateScanPath_SensitiveDotDirs(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	for _, dotDir := range sensitiveDotDirs {
		sensitive := filepath.Join(home, dotDir)
		// Only test if the directory exists
		if _, err := os.Stat(sensitive); err != nil {
			continue
		}
		if err := ValidateScanPath(sensitive); err == nil {
			t.Fatalf("expected error for sensitive path %s", sensitive)
		}
	}
}
