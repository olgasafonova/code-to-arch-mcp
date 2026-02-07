package model

// DiffSeverity classifies the severity of an architecture change.
type DiffSeverity string

const (
	SeverityNone     DiffSeverity = "none"
	SeverityLow      DiffSeverity = "low"      // Property changes
	SeverityMedium   DiffSeverity = "medium"   // New dependencies
	SeverityHigh     DiffSeverity = "high"     // New/removed services
	SeverityCritical DiffSeverity = "critical" // Circular deps, boundary violations
)

// DiffChangeType describes what kind of change was detected.
type DiffChangeType string

const (
	ChangeAdded    DiffChangeType = "added"
	ChangeRemoved  DiffChangeType = "removed"
	ChangeModified DiffChangeType = "modified"
)

// DiffEntry represents a single architectural change.
type DiffEntry struct {
	ChangeType DiffChangeType `json:"change_type"`
	Severity   DiffSeverity   `json:"severity"`
	Category   string         `json:"category"` // "node" or "edge"
	Subject    string         `json:"subject"`  // Node ID or edge description
	Detail     string         `json:"detail"`
}

// DiffReport summarizes all changes between two architecture graphs.
type DiffReport struct {
	BaseRef     string       `json:"base_ref,omitempty"`
	CompareRef  string       `json:"compare_ref,omitempty"`
	Changes     []DiffEntry  `json:"changes"`
	MaxSeverity DiffSeverity `json:"max_severity"`
	Summary     string       `json:"summary"`
}

// HasChanges returns true if any changes were detected.
func (r *DiffReport) HasChanges() bool {
	return len(r.Changes) > 0
}

// ChangesByType returns changes filtered by change type.
func (r *DiffReport) ChangesByType(ct DiffChangeType) []DiffEntry {
	var result []DiffEntry
	for _, c := range r.Changes {
		if c.ChangeType == ct {
			result = append(result, c)
		}
	}
	return result
}

// ChangesBySeverity returns changes at or above the given severity.
func (r *DiffReport) ChangesBySeverity(minSeverity DiffSeverity) []DiffEntry {
	severityOrder := map[DiffSeverity]int{
		SeverityNone:     0,
		SeverityLow:      1,
		SeverityMedium:   2,
		SeverityHigh:     3,
		SeverityCritical: 4,
	}
	minLevel := severityOrder[minSeverity]

	var result []DiffEntry
	for _, c := range r.Changes {
		if severityOrder[c.Severity] >= minLevel {
			result = append(result, c)
		}
	}
	return result
}
