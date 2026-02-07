package model

import "testing"

func TestDiffReport_HasChanges(t *testing.T) {
	empty := &DiffReport{}
	if empty.HasChanges() {
		t.Fatal("empty report should have no changes")
	}

	withChanges := &DiffReport{
		Changes: []DiffEntry{{ChangeType: ChangeAdded, Subject: "new-service"}},
	}
	if !withChanges.HasChanges() {
		t.Fatal("report with entries should have changes")
	}
}

func TestDiffReport_ChangesByType(t *testing.T) {
	report := &DiffReport{
		Changes: []DiffEntry{
			{ChangeType: ChangeAdded, Subject: "svc:new"},
			{ChangeType: ChangeRemoved, Subject: "svc:old"},
			{ChangeType: ChangeAdded, Subject: "db:cache"},
		},
	}

	added := report.ChangesByType(ChangeAdded)
	if len(added) != 2 {
		t.Fatalf("expected 2 added, got %d", len(added))
	}

	removed := report.ChangesByType(ChangeRemoved)
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(removed))
	}
}

func TestDiffReport_ChangesBySeverity(t *testing.T) {
	report := &DiffReport{
		Changes: []DiffEntry{
			{Severity: SeverityLow, Subject: "property-change"},
			{Severity: SeverityMedium, Subject: "new-dep"},
			{Severity: SeverityHigh, Subject: "new-service"},
			{Severity: SeverityCritical, Subject: "circular-dep"},
		},
	}

	highAndAbove := report.ChangesBySeverity(SeverityHigh)
	if len(highAndAbove) != 2 {
		t.Fatalf("expected 2 high+ changes, got %d", len(highAndAbove))
	}

	mediumAndAbove := report.ChangesBySeverity(SeverityMedium)
	if len(mediumAndAbove) != 3 {
		t.Fatalf("expected 3 medium+ changes, got %d", len(mediumAndAbove))
	}
}
