package detector

import (
	"fmt"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Explanation holds structured architecture analysis.
type Explanation struct {
	Summary        string   `json:"summary"`
	TopologyReason string   `json:"topology_reason"`
	Patterns       []string `json:"patterns"`
	KeyDecisions   []string `json:"key_decisions"`
	Risks          []string `json:"risks"`
}

// ExplainArchitecture provides rich structural analysis of a graph.
func ExplainArchitecture(graph *model.ArchGraph, boundaries *BoundaryResult) *Explanation {
	exp := &Explanation{
		Summary: graph.Summary(),
	}

	exp.TopologyReason = explainTopology(boundaries)
	exp.Patterns = detectPatterns(graph)
	exp.KeyDecisions = extractDecisions(graph)
	exp.Risks = identifyRisks(graph)

	return exp
}

func explainTopology(boundaries *BoundaryResult) string {
	if boundaries == nil {
		return "No boundary information available."
	}

	switch boundaries.Topology {
	case model.TopologyMonolith:
		return "Classified as monolith: single service boundary detected with one module root."
	case model.TopologyMonorepo:
		markers := collectMarkers(boundaries)
		return fmt.Sprintf("Classified as monorepo: multiple module roots detected. Markers: %s.", strings.Join(markers, ", "))
	case model.TopologyMicroservice:
		return fmt.Sprintf("Classified as microservices: %d service boundaries with container/orchestration markers.", len(boundaries.Boundaries))
	default:
		return "Topology could not be determined from project structure."
	}
}

func collectMarkers(boundaries *BoundaryResult) []string {
	seen := make(map[string]bool)
	var markers []string
	for _, b := range boundaries.Boundaries {
		for _, m := range b.Markers {
			if !seen[m] {
				seen[m] = true
				markers = append(markers, m)
			}
		}
	}
	return markers
}

func detectPatterns(graph *model.ArchGraph) []string {
	var patterns []string

	services := graph.NodesByType(model.NodeService)
	modules := graph.NodesByType(model.NodeModule)
	databases := graph.NodesByType(model.NodeDatabase)
	queues := graph.NodesByType(model.NodeQueue)
	caches := graph.NodesByType(model.NodeCache)
	endpoints := graph.NodesByType(model.NodeEndpoint)

	serviceCount := len(services) + len(modules)

	// Database patterns
	if len(databases) == 1 && serviceCount > 1 {
		patterns = append(patterns, "Shared database: multiple services reference a single data store")
	} else if len(databases) > 1 && serviceCount > 1 {
		patterns = append(patterns, "Database-per-service: multiple data stores detected alongside multiple services")
	}

	// Communication patterns
	if len(queues) > 0 {
		patterns = append(patterns, "Event-driven communication: message queue infrastructure detected")
	}

	apiCallCount := 0
	for _, e := range graph.Edges() {
		if e.Type == model.EdgeAPICall && e.Label != "serves" {
			apiCallCount++
		}
	}
	if apiCallCount > 0 {
		patterns = append(patterns, fmt.Sprintf("Synchronous inter-service communication: %d API call edges detected", apiCallCount))
	}

	// Caching
	if len(caches) > 0 {
		patterns = append(patterns, "Caching layer detected")
	}

	// Endpoint density
	if len(endpoints) > 0 {
		patterns = append(patterns, fmt.Sprintf("HTTP API surface: %d endpoints detected", len(endpoints)))
	}

	if len(patterns) == 0 {
		patterns = append(patterns, "No distinctive architectural patterns detected")
	}

	return patterns
}

func extractDecisions(graph *model.ArchGraph) []string {
	var decisions []string

	// Extract infrastructure choices from node properties
	for _, n := range graph.Nodes() {
		detectedVia, ok := n.Properties["detected_via"]
		if !ok {
			continue
		}

		switch n.Type {
		case model.NodeDatabase:
			decisions = append(decisions, fmt.Sprintf("Uses %s for data persistence (detected via %s import)", n.Name, detectedVia))
		case model.NodeQueue:
			decisions = append(decisions, fmt.Sprintf("Uses %s for messaging (detected via %s import)", n.Name, detectedVia))
		case model.NodeCache:
			decisions = append(decisions, fmt.Sprintf("Uses %s for caching (detected via %s import)", n.Name, detectedVia))
		case model.NodeExternalAPI:
			decisions = append(decisions, fmt.Sprintf("Calls external APIs (detected via %s import)", detectedVia))
		}
	}

	// Language mix
	languages := make(map[string]int)
	for _, n := range graph.Nodes() {
		if n.Language != "" {
			languages[n.Language]++
		}
	}
	if len(languages) > 1 {
		var langParts []string
		for lang, count := range languages {
			langParts = append(langParts, fmt.Sprintf("%s (%d components)", lang, count))
		}
		decisions = append(decisions, fmt.Sprintf("Multi-language codebase: %s", strings.Join(langParts, ", ")))
	} else if len(languages) == 1 {
		for lang := range languages {
			decisions = append(decisions, fmt.Sprintf("Single-language codebase: %s", lang))
		}
	}

	return decisions
}

func identifyRisks(graph *model.ArchGraph) []string {
	var risks []string

	services := graph.NodesByType(model.NodeService)
	modules := graph.NodesByType(model.NodeModule)
	databases := graph.NodesByType(model.NodeDatabase)
	caches := graph.NodesByType(model.NodeCache)
	endpoints := graph.NodesByType(model.NodeEndpoint)

	serviceCount := len(services) + len(modules)

	// Shared database risk
	if len(databases) == 1 && serviceCount > 2 {
		risks = append(risks, "Single database shared by multiple services may create coupling and scaling bottlenecks")
	}

	// Missing cache
	if len(endpoints) > 5 && len(caches) == 0 {
		risks = append(risks, "No caching layer detected despite multiple endpoints; may impact performance under load")
	}

	// Circular dependencies
	if graph.HasCycle() {
		risks = append(risks, "Circular dependencies detected; may cause build issues and unclear ownership")
	}

	// Orphan detection
	edgeNodes := make(map[string]bool)
	for _, e := range graph.Edges() {
		edgeNodes[e.Source] = true
		edgeNodes[e.Target] = true
	}
	orphanCount := 0
	for _, n := range graph.Nodes() {
		if n.Type != model.NodeEndpoint && !edgeNodes[n.ID] {
			orphanCount++
		}
	}
	if orphanCount > 0 {
		risks = append(risks, fmt.Sprintf("%d disconnected nodes detected; may indicate dead code or missing integrations", orphanCount))
	}

	return risks
}
