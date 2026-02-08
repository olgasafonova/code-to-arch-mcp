// Package safepath validates user-supplied filesystem paths for scan operations.
package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sensitiveRoots are directories that should never be scanned.
var sensitiveRoots = []string{
	"/etc",
	"/proc",
	"/sys",
	"/dev",
}

// sensitiveDotDirs are home-directory dotfiles that should never be scanned.
var sensitiveDotDirs = []string{
	".ssh",
	".gnupg",
	".aws",
	".config/gcloud",
}

// ValidateScanPath checks that a path is safe for scanning.
// Returns an error if the path is empty, doesn't exist, is not a directory,
// or points to a sensitive location.
func ValidateScanPath(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If EvalSymlinks fails, the path likely doesn't exist — check below
		resolved = absPath
	}

	// Check both original and resolved paths against sensitive locations
	pathsToCheck := []string{absPath, resolved}

	for _, p := range pathsToCheck {
		for _, root := range sensitiveRoots {
			if p == root || strings.HasPrefix(p, root+"/") {
				return fmt.Errorf("scanning %s is not allowed", root)
			}
		}
	}

	// Check sensitive home dotfiles
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, p := range pathsToCheck {
			for _, dotDir := range sensitiveDotDirs {
				sensitive := filepath.Join(home, dotDir)
				if p == sensitive || strings.HasPrefix(p, sensitive+"/") {
					return fmt.Errorf("scanning %s is not allowed", sensitive)
				}
			}
		}
	}

	// Path must exist and be a directory
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	return nil
}
