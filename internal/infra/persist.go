package infra

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StateDir returns the persistent state directory for an MCP server.
// Convention: ~/.mcp-context/<serverName>/
// Creates the directory if it doesn't exist.
func StateDir(serverName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}

	dir := filepath.Join(home, ".mcp-context", serverName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating state dir: %w", err)
	}

	return dir, nil
}

// LoadJSON reads a JSON file into the given type. Returns os.ErrNotExist if missing.
func LoadJSON[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}

	return &v, nil
}

// SaveJSON writes a value as indented JSON to the given path. Creates parent dirs.
func SaveJSON[T any](path string, v *T) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating parent dir: %w", err)
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
