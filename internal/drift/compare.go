package drift

import (
	"fmt"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Compare detects differences between a baseline graph and a current graph.
func Compare(baseline, current *model.ArchGraph) *model.DiffReport {
	report := &model.DiffReport{
		MaxSeverity: model.SeverityNone,
		Changes:     []model.DiffEntry{},
	}

	baseNodes := nodeMap(baseline.Nodes())
	currNodes := nodeMap(current.Nodes())

	// Detect added and removed nodes
	for id, node := range currNodes {
		if _, exists := baseNodes[id]; !exists {
			severity := classifyNodeChangeSeverity(node)
			report.Changes = append(report.Changes, model.DiffEntry{
				ChangeType: model.ChangeAdded,
				Severity:   severity,
				Category:   "node",
				Subject:    id,
				Detail:     fmt.Sprintf("Added %s: %s (%s)", node.Type, node.Name, id),
			})
			report.MaxSeverity = maxSeverity(report.MaxSeverity, severity)
		}
	}

	for id, node := range baseNodes {
		if _, exists := currNodes[id]; !exists {
			severity := classifyNodeChangeSeverity(node)
			report.Changes = append(report.Changes, model.DiffEntry{
				ChangeType: model.ChangeRemoved,
				Severity:   severity,
				Category:   "node",
				Subject:    id,
				Detail:     fmt.Sprintf("Removed %s: %s (%s)", node.Type, node.Name, id),
			})
			report.MaxSeverity = maxSeverity(report.MaxSeverity, severity)
		}
	}

	// Detect modified nodes (same ID, different properties)
	for id, currNode := range currNodes {
		baseNode, exists := baseNodes[id]
		if !exists {
			continue
		}
		if currNode.Name != baseNode.Name || currNode.Type != baseNode.Type {
			report.Changes = append(report.Changes, model.DiffEntry{
				ChangeType: model.ChangeModified,
				Severity:   model.SeverityLow,
				Category:   "node",
				Subject:    id,
				Detail:     fmt.Sprintf("Modified %s: %s", currNode.Type, currNode.Name),
			})
			report.MaxSeverity = maxSeverity(report.MaxSeverity, model.SeverityLow)
		}
	}

	// Detect edge changes
	baseEdges := edgeSet(baseline.Edges())
	currEdges := edgeSet(current.Edges())

	for key := range currEdges {
		if _, exists := baseEdges[key]; !exists {
			report.Changes = append(report.Changes, model.DiffEntry{
				ChangeType: model.ChangeAdded,
				Severity:   model.SeverityMedium,
				Category:   "edge",
				Subject:    key,
				Detail:     fmt.Sprintf("Added edge: %s", key),
			})
			report.MaxSeverity = maxSeverity(report.MaxSeverity, model.SeverityMedium)
		}
	}

	for key := range baseEdges {
		if _, exists := currEdges[key]; !exists {
			report.Changes = append(report.Changes, model.DiffEntry{
				ChangeType: model.ChangeRemoved,
				Severity:   model.SeverityMedium,
				Category:   "edge",
				Subject:    key,
				Detail:     fmt.Sprintf("Removed edge: %s", key),
			})
			report.MaxSeverity = maxSeverity(report.MaxSeverity, model.SeverityMedium)
		}
	}

	// Check for circular dependencies in current graph
	if current.HasCycle() {
		report.Changes = append(report.Changes, model.DiffEntry{
			ChangeType: model.ChangeAdded,
			Severity:   model.SeverityCritical,
			Category:   "validation",
			Subject:    "circular_dependency",
			Detail:     "Circular dependency detected in current architecture",
		})
		report.MaxSeverity = model.SeverityCritical
	}

	// Generate summary
	added := len(report.ChangesByType(model.ChangeAdded))
	removed := len(report.ChangesByType(model.ChangeRemoved))
	modified := len(report.ChangesByType(model.ChangeModified))

	if len(report.Changes) == 0 {
		report.Summary = "No architectural changes detected."
	} else {
		report.Summary = fmt.Sprintf(
			"%d changes detected: %d added, %d removed, %d modified. Max severity: %s.",
			len(report.Changes), added, removed, modified, report.MaxSeverity,
		)
	}

	return report
}

func nodeMap(nodes []*model.Node) map[string]*model.Node {
	m := make(map[string]*model.Node, len(nodes))
	for _, n := range nodes {
		m[n.ID] = n
	}
	return m
}

func edgeSet(edges []*model.Edge) map[string]bool {
	s := make(map[string]bool, len(edges))
	for _, e := range edges {
		key := fmt.Sprintf("%s-[%s]->%s", e.Source, e.Type, e.Target)
		s[key] = true
	}
	return s
}

func classifyNodeChangeSeverity(node *model.Node) model.DiffSeverity {
	switch node.Type {
	case model.NodeService:
		return model.SeverityHigh
	case model.NodeDatabase, model.NodeQueue, model.NodeCache:
		return model.SeverityHigh
	case model.NodeExternalAPI:
		return model.SeverityMedium
	case model.NodeModule, model.NodePackage:
		return model.SeverityMedium
	case model.NodeEndpoint:
		return model.SeverityLow
	default:
		return model.SeverityLow
	}
}

func maxSeverity(a, b model.DiffSeverity) model.DiffSeverity {
	order := map[model.DiffSeverity]int{
		model.SeverityNone:     0,
		model.SeverityLow:      1,
		model.SeverityMedium:   2,
		model.SeverityHigh:     3,
		model.SeverityCritical: 4,
	}
	if order[b] > order[a] {
		return b
	}
	return a
}
