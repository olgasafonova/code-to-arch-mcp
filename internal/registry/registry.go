// Package registry manages a persistent repo registry at ~/.mcp-context/ridge/.
// Repos registered by alias enable incremental scan state persistence and
// path resolution by name instead of absolute path.
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/olgasafonova/ridge/internal/infra"
)

const (
	serverName   = "ridge"
	registryFile = "registry.json"
	stateSubdir  = "state"
)

// Repo is a registered repository.
type Repo struct {
	Path      string    `json:"path"`
	AddedAt   time.Time `json:"added_at"`
	LastScan  time.Time `json:"last_scan,omitzero"`
	NodeCount int       `json:"node_count,omitempty"`
	EdgeCount int       `json:"edge_count,omitempty"`
	Topology  string    `json:"topology,omitempty"`
}

// Registry holds the persistent set of registered repos.
type Registry struct {
	Version string          `json:"version"`
	Repos   map[string]Repo `json:"repos"`

	dir string // resolved state directory (not serialized)
}

// Load reads the registry from disk. Returns an empty registry if the file doesn't exist.
func Load() (*Registry, error) {
	dir, err := infra.StateDir(serverName)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, registryFile)
	reg, err := infra.LoadJSON[Registry](path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{
				Version: "1",
				Repos:   make(map[string]Repo),
				dir:     dir,
			}, nil
		}
		return nil, fmt.Errorf("loading registry: %w", err)
	}
	reg.dir = dir
	if reg.Repos == nil {
		reg.Repos = make(map[string]Repo)
	}
	return reg, nil
}

// Save writes the registry to disk.
func (r *Registry) Save() error {
	path := filepath.Join(r.dir, registryFile)
	return infra.SaveJSON(path, r)
}

// Add registers a repo under the given alias. Returns an error if the alias is taken.
func (r *Registry) Add(alias, path string) error {
	if existing, ok := r.Repos[alias]; ok {
		return fmt.Errorf("alias %q already registered (path: %s)", alias, existing.Path)
	}
	r.Repos[alias] = Repo{
		Path:    path,
		AddedAt: time.Now().UTC(),
	}
	return nil
}

// Remove deletes a repo by alias and its state file. Returns an error if not found.
func (r *Registry) Remove(alias string) error {
	if _, ok := r.Repos[alias]; !ok {
		return fmt.Errorf("alias %q not found in registry", alias)
	}
	delete(r.Repos, alias)

	// Best-effort cleanup of state file.
	stateFile := r.StatePath(alias)
	_ = os.Remove(stateFile)
	return nil
}

// Get returns the repo for the given alias, or an error if not found.
func (r *Registry) Get(alias string) (Repo, error) {
	repo, ok := r.Repos[alias]
	if !ok {
		return Repo{}, fmt.Errorf("alias %q not found in registry", alias)
	}
	return repo, nil
}

// List returns all registered repos. Entries whose paths no longer exist
// are annotated in the returned RepoEntry slice.
func (r *Registry) List() []RepoEntry {
	entries := make([]RepoEntry, 0, len(r.Repos))
	for alias, repo := range r.Repos {
		entry := RepoEntry{
			Alias: alias,
			Repo:  repo,
		}
		if _, err := os.Stat(repo.Path); err != nil {
			entry.Stale = true
		}
		entries = append(entries, entry)
	}
	return entries
}

// UpdateScanInfo updates metadata for a repo after a successful scan.
func (r *Registry) UpdateScanInfo(alias string, nodeCount, edgeCount int, topology string) {
	repo, ok := r.Repos[alias]
	if !ok {
		return
	}
	repo.LastScan = time.Now().UTC()
	repo.NodeCount = nodeCount
	repo.EdgeCount = edgeCount
	repo.Topology = topology
	r.Repos[alias] = repo
}

// StatePath returns the path for a repo's incremental scan state file.
func (r *Registry) StatePath(alias string) string {
	return filepath.Join(r.dir, stateSubdir, alias+".json")
}

// RepoEntry is a repo with its alias and staleness flag, used for listing.
type RepoEntry struct {
	Alias string `json:"alias"`
	Repo
	Stale bool `json:"stale,omitempty"`
}
