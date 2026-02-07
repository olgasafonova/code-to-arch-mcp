// Package detector provides architecture detection: boundaries, topology, dataflow, and validation.
package detector

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// BoundaryResult holds detected service/module boundaries.
type BoundaryResult struct {
	Topology   model.TopologyType
	Boundaries []Boundary
}

// Boundary represents a detected service or module boundary.
type Boundary struct {
	Name    string
	Path    string
	Type    string // "service", "module", "package"
	Markers []string
}

// DetectBoundaries walks a directory tree and identifies service/module boundaries.
func DetectBoundaries(rootPath string) (*BoundaryResult, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	result := &BoundaryResult{
		Topology: model.TopologyUnknown,
	}

	var goMods, packageJSONs, dockerfiles []string
	var cmdDirs []string
	hasGoWork := false
	hasNxJSON := false
	hasPnpmWorkspace := false
	hasDockerCompose := false
	hasK8sManifests := false

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			if name == "cmd" {
				// Check for subdirectories under cmd/
				entries, _ := os.ReadDir(path)
				for _, e := range entries {
					if e.IsDir() {
						cmdDirs = append(cmdDirs, filepath.Join(path, e.Name()))
					}
				}
			}
			return nil
		}

		name := d.Name()
		rel, _ := filepath.Rel(absRoot, path)

		switch name {
		case "go.mod":
			goMods = append(goMods, rel)
		case "go.work":
			hasGoWork = true
		case "package.json":
			packageJSONs = append(packageJSONs, rel)
		case "nx.json":
			hasNxJSON = true
		case "pnpm-workspace.yaml":
			hasPnpmWorkspace = true
		case "Dockerfile", "dockerfile":
			dockerfiles = append(dockerfiles, rel)
		case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
			hasDockerCompose = true
		}

		// Check for Kubernetes manifests
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			dir := filepath.Dir(rel)
			if strings.Contains(dir, "k8s") || strings.Contains(dir, "kubernetes") || strings.Contains(dir, "deploy") {
				hasK8sManifests = true
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Detect topology
	result.Topology = inferTopology(
		len(goMods), len(packageJSONs), len(dockerfiles), len(cmdDirs),
		hasGoWork, hasNxJSON, hasPnpmWorkspace, hasDockerCompose, hasK8sManifests,
	)

	// Build boundary list
	for _, mod := range goMods {
		dir := filepath.Dir(mod)
		name := filepath.Base(dir)
		if dir == "." {
			name = filepath.Base(absRoot)
		}
		result.Boundaries = append(result.Boundaries, Boundary{
			Name:    name,
			Path:    dir,
			Type:    "module",
			Markers: []string{"go.mod"},
		})
	}

	for _, cmd := range cmdDirs {
		rel, _ := filepath.Rel(absRoot, cmd)
		name := filepath.Base(cmd)
		result.Boundaries = append(result.Boundaries, Boundary{
			Name:    name,
			Path:    rel,
			Type:    "service",
			Markers: []string{"cmd/ directory"},
		})
	}

	for _, df := range dockerfiles {
		dir := filepath.Dir(df)
		name := filepath.Base(dir)
		if dir == "." {
			name = filepath.Base(absRoot)
		}
		// Avoid duplicates: only add if not already represented
		if !boundaryExistsAtPath(result.Boundaries, dir) {
			result.Boundaries = append(result.Boundaries, Boundary{
				Name:    name,
				Path:    dir,
				Type:    "service",
				Markers: []string{"Dockerfile"},
			})
		} else {
			// Add Dockerfile as marker to existing boundary
			for i := range result.Boundaries {
				if result.Boundaries[i].Path == dir {
					result.Boundaries[i].Markers = append(result.Boundaries[i].Markers, "Dockerfile")
				}
			}
		}
	}

	return result, nil
}

func inferTopology(
	goModCount, pkgJSONCount, dockerfileCount, cmdCount int,
	hasGoWork, hasNx, hasPnpmWorkspace, hasDockerCompose, hasK8s bool,
) model.TopologyType {
	// Monorepo signals
	if hasGoWork || hasNx || hasPnpmWorkspace {
		return model.TopologyMonorepo
	}

	// Multiple go.mod files = monorepo
	if goModCount > 1 {
		return model.TopologyMonorepo
	}

	// Multiple Dockerfiles or k8s manifests + docker-compose = microservices
	if dockerfileCount > 1 && (hasDockerCompose || hasK8s) {
		return model.TopologyMicroservice
	}

	// Multiple cmd/ directories = multiple services in one repo
	if cmdCount > 1 {
		return model.TopologyMonorepo
	}

	// Single entry point, single go.mod/package.json = monolith
	if (goModCount == 1 || pkgJSONCount == 1) && dockerfileCount <= 1 {
		return model.TopologyMonolith
	}

	return model.TopologyUnknown
}

func boundaryExistsAtPath(boundaries []Boundary, path string) bool {
	for _, b := range boundaries {
		if b.Path == path {
			return true
		}
	}
	return false
}
