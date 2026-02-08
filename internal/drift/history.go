package drift

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitLogEntry holds parsed git log data.
type GitLogEntry struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// HistoryEntry represents architecture state at a specific git commit.
type HistoryEntry struct {
	Ref                 string `json:"ref"`
	Date                string `json:"date"`
	Message             string `json:"message"`
	NodeCount           int    `json:"node_count"`
	EdgeCount           int    `json:"edge_count"`
	Topology            string `json:"topology"`
	ChangesFromPrevious int    `json:"changes_from_previous"`
}

// GetSignificantCommits returns the N most recent non-merge commits.
func GetSignificantCommits(ctx context.Context, repoPath string, limit int) ([]GitLogEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}

	cmd := exec.CommandContext(ctx, "git", "log",
		fmt.Sprintf("--format=%%H|%%aI|%%s"),
		fmt.Sprintf("-n%d", limit),
		"--no-merges",
	)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var entries []GitLogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		entries = append(entries, GitLogEntry{
			Hash:    parts[0],
			Date:    parts[1],
			Message: parts[2],
		})
	}

	return entries, nil
}
