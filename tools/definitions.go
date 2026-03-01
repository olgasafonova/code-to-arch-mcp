// Package tools provides MCP tool definitions and handlers for Code to Arch MCP.
package tools

// ToolSpec defines a tool's metadata for declarative registration.
type ToolSpec struct {
	Name        string
	Method      string
	Description string
	Title       string
	Category    string
	ReadOnly    bool
	Idempotent  bool
	OpenWorld   bool
}

// ptr creates a pointer to a value.
func ptr[T any](v T) *T {
	return &v
}

// AllTools defines the 14 MCP tools for code-to-arch analysis.
var AllTools = []ToolSpec{
	{
		Name:   "arch_scan",
		Method: "ArchScan",
		Title:  "Scan Codebase Architecture",
		Description: `Analyze a codebase directory and generate an architecture model.
USE WHEN the user wants to understand the overall architecture of a project,
discover services, dependencies, and infrastructure components.
Returns a summary by default; set detail="full" for the complete node/edge graph.
For a single service or subdirectory, use arch_focus instead.
WHY: Parses Go with go/ast, TypeScript and Python with tree-sitter. Detects dependencies from import statements only; dynamic loading, reflection, or runtime service discovery is invisible.
FAILS WHEN: directory path doesn't exist (check path and retry), directory contains no supported code files in Go/TypeScript/Python (other languages are not yet supported).`,
		Category:   "analysis",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_focus",
		Method: "ArchFocus",
		Title:  "Focus on Subsystem",
		Description: `Analyze a specific subdirectory or service within a codebase.
USE WHEN the user wants to zoom into one service or module, not the entire project.
Pass a subdirectory path; returns the same format as arch_scan scoped to that subtree.
FAILS WHEN: subdirectory path doesn't exist (check path), no supported code files in that subtree (Go/TypeScript/Python only).`,
		Category:   "analysis",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_generate",
		Method: "ArchGenerate",
		Title:  "Generate Architecture Diagram",
		Description: `Generate a diagram from a scanned architecture in the specified format.
USE WHEN the user wants a visual representation of the architecture.
Supports Mermaid, PlantUML, C4, Structurizr DSL, draw.io, Excalidraw, and JSON output.
View levels: system (high-level), container (services + infra), component (all packages).
Optional theme_bg and theme_fg hex colors (e.g. "#ffffff", "#1e293b") derive a full Mermaid color palette from two colors. Works with Mermaid format only.
Set prune_threshold (0.0-1.0) to remove ubiquitous nodes like logging or fmt that clutter diagrams. A value of 0.5 removes nodes targeted by more than 50% of source nodes.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first), invalid format name (valid: mermaid, plantuml, c4, structurizr, drawio, excalidraw, json).`,
		Category:   "diagram",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_dependencies",
		Method: "ArchDependencies",
		Title:  "Map Dependencies",
		Description: `Map all dependencies: internal packages, external libraries, and infrastructure.
USE WHEN the user asks about what depends on what, import graphs, or external service dependencies.
Returns categorized dependency lists with import paths and detected infrastructure.
WHY: Detects dependencies from static import analysis only. Runtime dependencies, reflection-based injection, or dynamically loaded plugins are not captured.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first).`,
		Category:   "analysis",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_dataflow",
		Method: "ArchDataflow",
		Title:  "Trace Data Flow",
		Description: `Trace how data flows through the system from input to storage.
USE WHEN the user asks about data paths, where data enters the system,
how it gets processed, and where it ends up.
Identifies HTTP endpoints, message producers/consumers, and data stores.
Returns structured process traces: entry-to-terminal chains with confidence scores and edge types, grouped by entry point. Each trace shows the full path from an endpoint through intermediate packages to a terminal node (database, queue, cache, or external API).
WHY: Detects endpoints from stdlib patterns (net/http, Express, Flask/FastAPI). Custom frameworks or code-generated routes may not be detected.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first). Returns empty data paths if the codebase has no HTTP handlers, message producers, or data stores.`,
		Category:   "analysis",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_boundaries",
		Method: "ArchBoundaries",
		Title:  "Detect Service Boundaries",
		Description: `Identify service and module boundaries within a codebase.
USE WHEN the user wants to understand how the codebase is divided,
whether it's a monolith, monorepo, or microservices.
Detects boundaries from go.mod/package.json, cmd/ directories, Dockerfiles, and k8s manifests.
WHY: Infers boundaries from conventional markers. Projects without go.mod, package.json, Dockerfiles, or k8s manifests produce weaker boundary detection and may report "unknown" topology.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first).`,
		Category:   "analysis",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_diff",
		Method: "ArchDiff",
		Title:  "Compare Against Baseline",
		Description: `Compare current code architecture against a stored baseline snapshot.
USE WHEN the user wants to check if the code has drifted from the documented architecture.
For comparing two git refs (branches/tags) instead, use arch_drift.
Returns a diff report with added/removed/modified components and severity classification.
WHY: Uses exact node ID matching, not fuzzy. Renamed packages or services appear as separate "removed" and "added" entries, not as "modified."
FAILS WHEN: no baseline snapshot exists (run arch_snapshot first to create one), snapshot was saved for a different project directory.`,
		Category:   "drift",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_drift",
		Method: "ArchDrift",
		Title:  "Detect Drift Between Refs",
		Description: `Detect architectural drift between two git references (branches, tags, commits).
USE WHEN the user wants to compare architecture between git refs,
like "how has the architecture changed since v1.0?"
For comparing against a saved baseline snapshot, use arch_diff instead.
Scans both refs and reports differences.
WHY: Checks out each ref independently, scans both, and diffs the resulting architecture graphs. Works with any valid git ref (branch name, tag, commit SHA).
FAILS WHEN: directory is not a git repository, specified git ref doesn't exist (check branch/tag names with git branch -a or git tag).`,
		Category:   "drift",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_validate",
		Method: "ArchValidate",
		Title:  "Validate Architecture Rules",
		Description: `Check architecture against rules: circular dependencies, layering violations, boundary crossings.
USE WHEN the user asks "are there any architecture problems?" or wants to enforce constraints.
Returns a list of violations with severity and suggested fixes.
For numeric health scores (coupling, instability), use arch_metrics instead.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first). Returns empty violations list if no problems found (that's a good result, not an error).`,
		Category:   "validation",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_history",
		Method: "ArchHistory",
		Title:  "Architecture Evolution",
		Description: `Show how architecture has evolved over git history.
USE WHEN the user asks "how has the architecture changed over time?" or wants to see growth patterns.
Samples key commits/tags and shows component counts, new services, removed services.
For comparing exactly two git refs, use arch_drift instead.
FAILS WHEN: directory is not a git repository. Produces minimal output if the repo has too few commits or tags to show meaningful evolution.`,
		Category:   "history",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_snapshot",
		Method: "ArchSnapshot",
		Title:  "Save Architecture Baseline",
		Description: `Save the current architecture as a baseline for future drift detection.
USE WHEN the user wants to establish a reference point, like "save this as our v2.0 architecture."
Writes a JSON snapshot file that arch_diff can compare against.
WARNING: Overwrites any existing snapshot for this project directory without confirmation.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first), target directory (~/.mcp-context/) is not writable.`,
		Category: "export",
		ReadOnly: false, // writes a snapshot file
	},
	{
		Name:   "arch_metrics",
		Method: "ArchMetrics",
		Title:  "Architecture Fitness Metrics",
		Description: `Compute structural metrics: coupling, instability, dependency depth.
USE WHEN the user asks about code quality, technical debt, architectural health,
or wants numeric scores to track over time.
For rule violations (circular deps, layering), use arch_validate instead.
Returns per-component coupling and instability scores plus project-wide averages.
WHY: Instability = Ce/(Ca+Ce). High instability (near 1.0) means a component depends on many others but few depend on it. Low instability (near 0.0) means many components depend on it, making it hard to change safely.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first).`,
		Category:   "validation",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_explain",
		Method: "ArchExplain",
		Title:  "Explain Architecture",
		Description: `Explain architecture decisions with code evidence.
USE WHEN the user asks "why is it structured this way?" or "explain the architecture."
Uses the scanned graph plus code patterns to provide architectural rationale.
For actionable improvement suggestions, use arch_recommend instead.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first).`,
		Category:   "analysis",
		ReadOnly:   true,
		Idempotent: true,
	},
	{
		Name:   "arch_recommend",
		Method: "ArchRecommend",
		Title:  "Recommend Architecture Improvements",
		Description: `Analyze architecture and recommend specific improvements with priorities.
USE WHEN the user asks "how should I improve this?" or "what should the architecture look like?"
Combines validation, metrics, and pattern analysis to produce actionable recommendations.
For just violations, use arch_validate. For just metrics, use arch_metrics.
FAILS WHEN: no architecture data loaded (run arch_scan or arch_focus first).`,
		Category:   "validation",
		ReadOnly:   true,
		Idempotent: true,
	},
}
