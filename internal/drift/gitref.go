package drift

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// ValidateRef checks that a git ref is safe to pass to git commands.
// Rejects empty refs, refs starting with "-", and refs with shell metacharacters.
func ValidateRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("empty ref")
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("ref cannot start with dash: %q", ref)
	}
	for _, c := range ref {
		if !isValidRefChar(c) {
			return fmt.Errorf("invalid character in ref: %c", c)
		}
	}
	return nil
}

// isValidRefChar returns true for characters safe in git refs.
func isValidRefChar(c rune) bool {
	if unicode.IsLetter(c) || unicode.IsDigit(c) {
		return true
	}
	switch c {
	case '.', '/', '-', '_', '~', '^':
		return true
	}
	return false
}

// CheckoutRef creates a temporary git worktree at the given ref.
// Returns the worktree path and a cleanup function that removes the worktree.
// The caller MUST call cleanup when done.
func CheckoutRef(ctx context.Context, repoPath, ref string) (worktreePath string, cleanup func(), err error) {
	if err := ValidateRef(ref); err != nil {
		return "", nil, fmt.Errorf("invalid ref: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "arch-drift-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}
	worktreePath = filepath.Join(tmpDir, "worktree")

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", worktreePath, ref)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("git worktree add %s: %s: %w", ref, string(out), err)
	}

	cleanup = func() {
		rmCmd := exec.Command("git", "worktree", "remove", "--force", worktreePath)
		rmCmd.Dir = repoPath
		rmCmd.Run()
		os.RemoveAll(tmpDir)
	}

	return worktreePath, cleanup, nil
}
