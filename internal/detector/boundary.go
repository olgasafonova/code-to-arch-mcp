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

// boundaryMarkers holds the raw findings from one walk of the project tree.
type boundaryMarkers struct {
	goMods       []string
	packageJSONs []string
	dockerfiles  []string
	pyProjects   []string
	cargoTomls   []string
	pomXMLs      []string
	gradleBuilds []string
	cmdDirs      []string

	hasGoWork        bool
	hasNxJSON        bool
	hasTurboJSON     bool
	hasRushJSON      bool
	hasPnpmWorkspace bool
	hasDockerCompose bool
	hasK8sManifests  bool
}

var boundarySkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
}

// DetectBoundaries walks a directory tree and identifies service/module boundaries.
func DetectBoundaries(rootPath string) (*BoundaryResult, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	markers, err := collectBoundaryMarkers(absRoot)
	if err != nil {
		return nil, err
	}

	totalProjectMarkers := len(markers.goMods) + len(markers.packageJSONs) +
		len(markers.pyProjects) + len(markers.cargoTomls) +
		len(markers.pomXMLs) + len(markers.gradleBuilds)

	result := &BoundaryResult{
		Topology: inferTopology(
			len(markers.goMods), len(markers.packageJSONs), len(markers.dockerfiles), len(markers.cmdDirs),
			totalProjectMarkers,
			markers.hasGoWork, markers.hasNxJSON, markers.hasTurboJSON, markers.hasRushJSON,
			markers.hasPnpmWorkspace, markers.hasDockerCompose, markers.hasK8sManifests,
		),
	}

	result.Boundaries = buildBoundaries(result.Boundaries, absRoot, markers)
	return result, nil
}

// collectBoundaryMarkers walks the tree once and returns every project-level
// marker we recognize: per-language manifests, container files, workspace
// configs, k8s manifests, and cmd/* subdirectories.
func collectBoundaryMarkers(absRoot string) (*boundaryMarkers, error) {
	m := &boundaryMarkers{}
	err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if boundarySkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if d.Name() == "cmd" {
				m.cmdDirs = append(m.cmdDirs, listSubdirs(path)...)
			}
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		classifyMarkerFile(m, d.Name(), rel)
		return nil
	})
	return m, err
}

// classifyMarkerFile updates m based on a single file name + relative path.
func classifyMarkerFile(m *boundaryMarkers, name, rel string) {
	switch name {
	case "go.mod":
		m.goMods = append(m.goMods, rel)
	case "go.work":
		m.hasGoWork = true
	case "package.json":
		m.packageJSONs = append(m.packageJSONs, rel)
	case "nx.json":
		m.hasNxJSON = true
	case "turbo.json":
		m.hasTurboJSON = true
	case "rush.json":
		m.hasRushJSON = true
	case "pnpm-workspace.yaml":
		m.hasPnpmWorkspace = true
	case "Dockerfile", "dockerfile":
		m.dockerfiles = append(m.dockerfiles, rel)
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		m.hasDockerCompose = true
	case "pyproject.toml", "setup.py", "setup.cfg":
		m.pyProjects = append(m.pyProjects, rel)
	case "Cargo.toml":
		m.cargoTomls = append(m.cargoTomls, rel)
	case "pom.xml":
		m.pomXMLs = append(m.pomXMLs, rel)
	case "build.gradle", "build.gradle.kts":
		m.gradleBuilds = append(m.gradleBuilds, rel)
	}
	if isInDeployDir(name, rel) {
		m.hasK8sManifests = true
	}
}

// isInDeployDir returns true for *.yaml/*.yml files under k8s/, kubernetes/, or deploy/.
func isInDeployDir(name, rel string) bool {
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return false
	}
	dir := filepath.Dir(rel)
	return strings.Contains(dir, "k8s") || strings.Contains(dir, "kubernetes") || strings.Contains(dir, "deploy")
}

// listSubdirs returns absolute paths of every direct subdirectory under path.
func listSubdirs(path string) []string {
	var out []string
	entries, _ := os.ReadDir(path)
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	return out
}

// buildBoundaries turns the collected markers into a deduped Boundary slice.
// Order matches the original implementation: Go modules, cmd dirs, Dockerfiles
// (which augment existing entries), then Python/Rust/Maven/Gradle modules.
func buildBoundaries(boundaries []Boundary, absRoot string, m *boundaryMarkers) []Boundary {
	for _, mod := range m.goMods {
		dir := filepath.Dir(mod)
		boundaries = append(boundaries, Boundary{
			Name:    boundaryName(dir, absRoot),
			Path:    dir,
			Type:    "module",
			Markers: []string{"go.mod"},
		})
	}
	for _, cmd := range m.cmdDirs {
		rel, _ := filepath.Rel(absRoot, cmd)
		boundaries = append(boundaries, Boundary{
			Name:    filepath.Base(cmd),
			Path:    rel,
			Type:    "service",
			Markers: []string{"cmd/ directory"},
		})
	}
	for _, df := range m.dockerfiles {
		dir := filepath.Dir(df)
		boundaries = appendOrAugment(boundaries, dir, boundaryName(dir, absRoot), "service", "Dockerfile", true)
	}
	for _, py := range m.pyProjects {
		dir := filepath.Dir(py)
		boundaries = appendOrAugment(boundaries, dir, boundaryName(dir, absRoot), "module", filepath.Base(py), false)
	}
	for _, cargo := range m.cargoTomls {
		dir := filepath.Dir(cargo)
		boundaries = appendOrAugment(boundaries, dir, boundaryName(dir, absRoot), "module", "Cargo.toml", false)
	}
	for _, pom := range m.pomXMLs {
		dir := filepath.Dir(pom)
		boundaries = appendOrAugment(boundaries, dir, boundaryName(dir, absRoot), "module", "pom.xml", false)
	}
	for _, gradle := range m.gradleBuilds {
		dir := filepath.Dir(gradle)
		boundaries = appendOrAugment(boundaries, dir, boundaryName(dir, absRoot), "module", filepath.Base(gradle), false)
	}
	return boundaries
}

// appendOrAugment adds a new boundary at dir, or — if one already exists at
// that path — appends the marker to it when augment is true (otherwise skips
// silently).
func appendOrAugment(boundaries []Boundary, dir, name, kind, marker string, augment bool) []Boundary {
	for i := range boundaries {
		if boundaries[i].Path == dir {
			if augment {
				boundaries[i].Markers = append(boundaries[i].Markers, marker)
			}
			return boundaries
		}
	}
	return append(boundaries, Boundary{Name: name, Path: dir, Type: kind, Markers: []string{marker}})
}

// boundaryName returns the directory's basename, falling back to the project
// root's basename when dir is "." (the project root itself).
func boundaryName(dir, absRoot string) string {
	if dir == "." {
		return filepath.Base(absRoot)
	}
	return filepath.Base(dir)
}

func inferTopology(
	goModCount, pkgJSONCount, dockerfileCount, cmdCount, totalProjectMarkers int,
	hasGoWork, hasNx, hasTurbo, hasRush, hasPnpmWorkspace, hasDockerCompose, hasK8s bool,
) model.TopologyType {
	// Monorepo signals: workspace-level config files
	if hasGoWork || hasNx || hasTurbo || hasRush || hasPnpmWorkspace {
		return model.TopologyMonorepo
	}

	// Multiple go.mod files = monorepo
	if goModCount > 1 {
		return model.TopologyMonorepo
	}

	// Multiple Dockerfiles with orchestration = microservices
	if dockerfileCount > 1 && (hasDockerCompose || hasK8s) {
		return model.TopologyMicroservice
	}

	// 3+ Dockerfiles without orchestration still indicates microservices
	if dockerfileCount > 2 {
		return model.TopologyMicroservice
	}

	// Multiple cmd/ directories = multiple services in one repo
	if cmdCount > 1 {
		return model.TopologyMonorepo
	}

	// Multiple project markers across different languages = monorepo
	if totalProjectMarkers > 2 {
		return model.TopologyMonorepo
	}

	// Single entry point, single project marker = monolith
	if (goModCount == 1 || pkgJSONCount == 1) && dockerfileCount <= 1 {
		return model.TopologyMonolith
	}

	return model.TopologyUnknown
}
