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

// ValidateOutputPath checks that a file path resolves to within baseDir.
// Unlike ValidateScanPath, the file does not need to exist yet (it may be
// created). The check uses filepath.Rel containment instead of HasPrefix
// (which is bypassable by sibling-prefix tricks like
// /tmp/baseEvil vs /tmp/base) and resolves symlinks on baseDir AND on
// the deepest existing ancestor of filePath, so a symlink under baseDir
// can't redirect the write outside it.
//
// Returns an error if the path escapes baseDir, targets a sensitive root,
// or fails resolution.
func ValidateOutputPath(filePath, baseDir string) error {
	if filePath == "" {
		return fmt.Errorf("file path is required")
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("resolving base directory: %w", err)
	}
	absBase = filepath.Clean(absBase)

	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}
	absFile = filepath.Clean(absFile)

	// Resolve symlinks on baseDir. baseDir is expected to exist; if it
	// doesn't, EvalSymlinks fails and we surface the error rather than
	// silently allowing.
	resolvedBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		return fmt.Errorf("resolving base directory symlinks: %w", err)
	}

	// Resolve symlinks on the deepest existing ancestor of filePath. The
	// file itself may not exist yet (this is a write check), but any
	// existing ancestor directory could be a symlink that redirects the
	// write outside baseDir.
	resolvedFile, err := evalDeepestAncestor(absFile)
	if err != nil {
		return fmt.Errorf("resolving file path symlinks: %w", err)
	}

	// Lexical Rel containment on the resolved values. Catches both `..`
	// escapes and sibling-prefix tricks that HasPrefix would have missed.
	if err := containedIn(resolvedFile, resolvedBase); err != nil {
		return err
	}

	// Defense in depth: also reject writes to sensitive roots even if
	// baseDir overlapped them.
	for _, root := range sensitiveRoots {
		if resolvedFile == root || strings.HasPrefix(resolvedFile, root+string(filepath.Separator)) {
			return fmt.Errorf("writing to %s is not allowed", root)
		}
	}

	return nil
}

// evalDeepestAncestor returns the path with the deepest existing ancestor
// directory's symlinks resolved. The basename (and any non-existent
// intermediate components) is appended unchanged. This lets ValidateOutputPath
// accept paths to files that do not yet exist while still defeating
// symlinked-ancestor tricks.
func evalDeepestAncestor(p string) (string, error) {
	cur := p
	suffix := ""
	for {
		if _, err := os.Lstat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			if suffix == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, suffix), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached the filesystem root and nothing existed; treat the
			// original path as already-canonical.
			return p, nil
		}
		base := filepath.Base(cur)
		if suffix == "" {
			suffix = base
		} else {
			suffix = filepath.Join(base, suffix)
		}
		cur = parent
	}
}

func containedIn(file, base string) error {
	rel, err := filepath.Rel(base, file)
	if err != nil {
		return fmt.Errorf("file path %s is outside allowed directory %s", file, base)
	}
	if rel == "." {
		// `file == base` is not a valid output path.
		return fmt.Errorf("file path equals base directory %s", base)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("file path %s is outside allowed directory %s", file, base)
	}
	return nil
}
