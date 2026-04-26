package drift

import (
	"strings"
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

func TestNarrate_NoChanges(t *testing.T) {
	report := &model.DiffReport{
		BaseRef:    "v1.0",
		CompareRef: "v1.1",
		Changes:    []model.DiffEntry{},
	}
	got := Narrate(report)
	want := "No structural change between v1.0 and v1.1."
	if got != want {
		t.Errorf("Narrate empty diff:\n got:  %q\n want: %q", got, want)
	}
}

func TestNarrate_NilReport(t *testing.T) {
	got := Narrate(nil)
	want := "No structural change between baseline and current."
	if got != want {
		t.Errorf("Narrate nil:\n got:  %q\n want: %q", got, want)
	}
}

func TestNarrate_NoRefsFallsBackToDefaults(t *testing.T) {
	report := &model.DiffReport{Changes: []model.DiffEntry{}}
	got := Narrate(report)
	if !strings.Contains(got, "baseline") || !strings.Contains(got, "current") {
		t.Errorf("expected default refs in narrative, got: %q", got)
	}
}

func TestNarrate_AddedNodes(t *testing.T) {
	report := &model.DiffReport{
		BaseRef:     "main",
		CompareRef:  "feat/billing",
		MaxSeverity: model.SeverityHigh,
		Changes: []model.DiffEntry{
			{ChangeType: model.ChangeAdded, Severity: model.SeverityHigh, Category: "node", Subject: "pkg:internal/billing", Detail: "Added package: billing"},
			{ChangeType: model.ChangeAdded, Severity: model.SeverityHigh, Category: "node", Subject: "infra:postgresql", Detail: "Added infra: postgresql"},
		},
	}
	got := Narrate(report)
	if !strings.Contains(got, "Between main and feat/billing") {
		t.Errorf("missing opening: %q", got)
	}
	if !strings.Contains(got, "billing") || !strings.Contains(got, "postgresql") {
		t.Errorf("missing node names: %q", got)
	}
	if !strings.Contains(got, "Added") {
		t.Errorf("missing 'Added' phrase: %q", got)
	}
	if !strings.Contains(got, "high") {
		t.Errorf("expected high severity callout: %q", got)
	}
}

func TestNarrate_RemovedAndModified(t *testing.T) {
	report := &model.DiffReport{
		BaseRef:    "v1.0",
		CompareRef: "v2.0",
		Changes: []model.DiffEntry{
			{ChangeType: model.ChangeRemoved, Severity: model.SeverityHigh, Category: "node", Subject: "pkg:rate-limiter", Detail: "Removed package: rate-limiter"},
			{ChangeType: model.ChangeModified, Severity: model.SeverityLow, Category: "node", Subject: "svc:auth", Detail: "Modified service: auth"},
		},
	}
	got := Narrate(report)
	if !strings.Contains(got, "rate-limiter") {
		t.Errorf("missing removed name: %q", got)
	}
	if !strings.Contains(got, "modified") {
		t.Errorf("missing modified phrase: %q", got)
	}
}

func TestNarrate_OnlyEdges(t *testing.T) {
	report := &model.DiffReport{
		BaseRef:    "main",
		CompareRef: "feat/redis",
		Changes: []model.DiffEntry{
			{ChangeType: model.ChangeAdded, Severity: model.SeverityMedium, Category: "edge", Subject: "svc:auth-[read_write]->infra:redis", Detail: "Added edge"},
		},
	}
	got := Narrate(report)
	if !strings.Contains(got, "1 dependency edge changed") &&
		!strings.Contains(got, "added 1 new dependency edge") {
		t.Errorf("expected edge-focused narrative, got: %q", got)
	}
}

func TestNarrate_CircularDependencyValidation(t *testing.T) {
	report := &model.DiffReport{
		BaseRef:     "main",
		CompareRef:  "feat/loop",
		MaxSeverity: model.SeverityCritical,
		Changes: []model.DiffEntry{
			{ChangeType: model.ChangeAdded, Severity: model.SeverityMedium, Category: "edge", Subject: "a-[dependency]->b", Detail: "Added edge"},
			{ChangeType: model.ChangeAdded, Severity: model.SeverityCritical, Category: "validation", Subject: "circular_dependency", Detail: "Circular dependency detected in current architecture"},
		},
	}
	got := Narrate(report)
	if !strings.Contains(got, "Validation:") {
		t.Errorf("expected validation callout: %q", got)
	}
	if !strings.Contains(got, "Circular dependency") {
		t.Errorf("expected circular-dep detail: %q", got)
	}
}

func TestNarrate_TruncatesLongLists(t *testing.T) {
	changes := []model.DiffEntry{}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		changes = append(changes, model.DiffEntry{
			ChangeType: model.ChangeAdded,
			Severity:   model.SeverityMedium,
			Category:   "node",
			Subject:    "pkg:" + name,
			Detail:     "Added package " + name,
		})
	}
	report := &model.DiffReport{BaseRef: "main", CompareRef: "feat", Changes: changes}
	got := Narrate(report)
	if !strings.Contains(got, "and 2 more") {
		t.Errorf("expected truncation marker for >3 items, got: %q", got)
	}
}

func TestNarrate_GroupsByNodeType(t *testing.T) {
	report := &model.DiffReport{
		BaseRef:    "main",
		CompareRef: "feat",
		Changes: []model.DiffEntry{
			{ChangeType: model.ChangeAdded, Category: "node", Subject: "pkg:foo", Detail: "Added package foo"},
			{ChangeType: model.ChangeAdded, Category: "node", Subject: "pkg:bar", Detail: "Added package bar"},
			{ChangeType: model.ChangeAdded, Category: "node", Subject: "infra:redis", Detail: "Added infra redis"},
		},
	}
	got := Narrate(report)
	if !strings.Contains(got, "2 packages") {
		t.Errorf("expected '2 packages' grouping: %q", got)
	}
	if !strings.Contains(got, "infrastructure component") {
		t.Errorf("expected infra grouping: %q", got)
	}
}

func TestNarrate_SentenceCount(t *testing.T) {
	// Bead acceptance: paragraph for non-empty diff. Allow 2-5 sentences.
	report := &model.DiffReport{
		BaseRef:     "main",
		CompareRef:  "feat",
		MaxSeverity: model.SeverityHigh,
		Changes: []model.DiffEntry{
			{ChangeType: model.ChangeAdded, Severity: model.SeverityHigh, Category: "node", Subject: "svc:new", Detail: "Added service: new"},
			{ChangeType: model.ChangeRemoved, Severity: model.SeverityHigh, Category: "node", Subject: "svc:old", Detail: "Removed service: old"},
			{ChangeType: model.ChangeAdded, Severity: model.SeverityMedium, Category: "edge", Subject: "a-[dependency]->b", Detail: "Added edge"},
		},
	}
	got := Narrate(report)
	count := strings.Count(got, ".")
	if count < 2 || count > 5 {
		t.Errorf("expected 2-5 sentences, got %d in: %q", count, got)
	}
}
