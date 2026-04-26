package drift

import (
	"fmt"
	"sort"
	"strings"

	"github.com/olgasafonova/ridge/internal/model"
)

// Narrate produces a natural-language summary of a DiffReport. The output is
// 1 sentence for an empty diff and 2-5 sentences otherwise. No LLM calls;
// the function is deterministic templating built from the structured diff.
func Narrate(report *model.DiffReport) string {
	baseRef := refOrDefault(refOf(report, true), "baseline")
	headRef := refOrDefault(refOf(report, false), "current")

	if report == nil || !report.HasChanges() {
		return fmt.Sprintf("No structural change between %s and %s.", baseRef, headRef)
	}

	added := report.ChangesByType(model.ChangeAdded)
	removed := report.ChangesByType(model.ChangeRemoved)
	modified := report.ChangesByType(model.ChangeModified)

	addedNodes := filterCategory(added, "node")
	removedNodes := filterCategory(removed, "node")
	addedEdges := filterCategory(added, "edge")
	removedEdges := filterCategory(removed, "edge")
	validations := filterCategory(report.Changes, "validation")

	var sentences []string

	headline := dominantMove(addedNodes, removedNodes, addedEdges, removedEdges, modified)
	sentences = append(sentences, fmt.Sprintf("Between %s and %s, %s.", baseRef, headRef, headline))

	if len(addedNodes) > 0 {
		sentences = append(sentences, capitalize(formatNodeGroup("added", addedNodes))+".")
	}
	if len(removedNodes) > 0 {
		sentences = append(sentences, capitalize(formatNodeGroup("removed", removedNodes))+".")
	}
	if len(addedEdges) > 0 || len(removedEdges) > 0 {
		sentences = append(sentences, capitalize(formatEdgeChange(len(addedEdges), len(removedEdges)))+".")
	}

	if len(validations) > 0 {
		details := make([]string, 0, len(validations))
		for _, v := range validations {
			details = append(details, v.Detail)
		}
		sentences = append(sentences, "Validation: "+strings.Join(details, "; ")+".")
	} else if report.MaxSeverity == model.SeverityHigh {
		sentences = append(sentences, "Overall severity: high.")
	}

	return strings.Join(sentences, " ")
}

func refOf(report *model.DiffReport, base bool) string {
	if report == nil {
		return ""
	}
	if base {
		return report.BaseRef
	}
	return report.CompareRef
}

func refOrDefault(ref, def string) string {
	if ref == "" {
		return def
	}
	return ref
}

// dominantMove picks the highest-impact phrase to lead the paragraph with.
// Validation issues outrank node changes, which outrank edge changes.
func dominantMove(addedNodes, removedNodes, addedEdges, removedEdges, modified []model.DiffEntry) string {
	totalNodes := len(addedNodes) + len(removedNodes)
	totalEdges := len(addedEdges) + len(removedEdges)

	switch {
	case totalNodes > 0:
		parts := []string{}
		if len(addedNodes) > 0 {
			parts = append(parts, fmt.Sprintf("%d %s added", len(addedNodes), pluralize("component", len(addedNodes))))
		}
		if len(removedNodes) > 0 {
			parts = append(parts, fmt.Sprintf("%d removed", len(removedNodes)))
		}
		if len(modified) > 0 {
			parts = append(parts, fmt.Sprintf("%d modified", len(modified)))
		}
		if totalEdges > 0 {
			parts = append(parts, fmt.Sprintf("%d dependency %s changed", totalEdges, pluralize("edge", totalEdges)))
		}
		return strings.Join(parts, ", ")
	case totalEdges > 0:
		return fmt.Sprintf("%d dependency %s changed", totalEdges, pluralize("edge", totalEdges))
	case len(modified) > 0:
		return fmt.Sprintf("%d %s modified in place", len(modified), pluralize("component", len(modified)))
	}
	return "structural changes detected"
}

// formatNodeGroup builds a phrase like "added 2 packages (foo, bar) and 1 database (postgres)".
func formatNodeGroup(verb string, entries []model.DiffEntry) string {
	if len(entries) == 0 {
		return ""
	}
	byType := map[string][]string{}
	var typeOrder []string
	for _, e := range entries {
		typ, name := nodeTypeAndName(e.Subject)
		if _, ok := byType[typ]; !ok {
			typeOrder = append(typeOrder, typ)
		}
		byType[typ] = append(byType[typ], name)
	}
	sort.Strings(typeOrder)

	phrases := make([]string, 0, len(typeOrder))
	for _, typ := range typeOrder {
		names := byType[typ]
		sort.Strings(names)
		label := pluralizeType(typ, len(names))
		phrases = append(phrases, fmt.Sprintf("%d %s (%s)", len(names), label, joinTrimmed(names, 3)))
	}
	return verb + " " + joinPhrases(phrases)
}

func formatEdgeChange(added, removed int) string {
	switch {
	case added > 0 && removed > 0:
		return fmt.Sprintf("dependency edges shifted: %d added and %d removed", added, removed)
	case added > 0:
		return fmt.Sprintf("added %d new dependency %s", added, pluralize("edge", added))
	case removed > 0:
		return fmt.Sprintf("removed %d dependency %s", removed, pluralize("edge", removed))
	}
	return ""
}

// nodeTypeAndName splits a node ID like "pkg:internal/foo" into ("pkg", "internal/foo").
// IDs without a colon are treated as opaque names with type "node".
func nodeTypeAndName(subject string) (typ, name string) {
	if i := strings.Index(subject, ":"); i > 0 {
		return subject[:i], subject[i+1:]
	}
	return "node", subject
}

func filterCategory(entries []model.DiffEntry, category string) []model.DiffEntry {
	out := make([]model.DiffEntry, 0, len(entries))
	for _, e := range entries {
		if e.Category == category {
			out = append(out, e)
		}
	}
	return out
}

func joinTrimmed(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(", and %d more", len(items)-max)
}

func joinPhrases(phrases []string) string {
	switch len(phrases) {
	case 0:
		return ""
	case 1:
		return phrases[0]
	case 2:
		return phrases[0] + " and " + phrases[1]
	}
	return strings.Join(phrases[:len(phrases)-1], ", ") + ", and " + phrases[len(phrases)-1]
}

func pluralize(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// pluralizeType maps ID prefixes to friendly nouns for narrative output.
func pluralizeType(prefix string, n int) string {
	singular, plural := "node", "nodes"
	switch prefix {
	case "pkg":
		singular, plural = "package", "packages"
	case "svc", "service":
		singular, plural = "service", "services"
	case "module":
		singular, plural = "module", "modules"
	case "db", "database":
		singular, plural = "database", "databases"
	case "queue":
		singular, plural = "queue", "queues"
	case "cache":
		singular, plural = "cache", "caches"
	case "infra":
		singular, plural = "infrastructure component", "infrastructure components"
	case "api", "external_api":
		singular, plural = "external API", "external APIs"
	case "endpoint":
		singular, plural = "endpoint", "endpoints"
	case "note":
		singular, plural = "note", "notes"
	}
	if n == 1 {
		return singular
	}
	return plural
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
