package drift

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CheckoutRef creates a temporary git worktree at the given ref.
// Returns the worktree path and a cleanup function that removes the worktree.
// The caller MUST call cleanup when done.
func CheckoutRef(repoPath, ref string) (worktreePath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "arch-drift-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp dir: %w", err)
	}
	worktreePath = filepath.Join(tmpDir, "worktree")

	cmd := exec.Command("git", "worktree", "add", "--detach", worktreePath, ref)
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
