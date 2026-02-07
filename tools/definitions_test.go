package tools

import (
	"strings"
	"testing"
)

func TestAllToolsHaveRequiredFields(t *testing.T) {
	for _, spec := range AllTools {
		if spec.Name == "" {
			t.Errorf("tool missing Name")
		}
		if spec.Method == "" {
			t.Errorf("tool %q missing Method", spec.Name)
		}
		if spec.Title == "" {
			t.Errorf("tool %q missing Title", spec.Name)
		}
		if spec.Description == "" {
			t.Errorf("tool %q missing Description", spec.Name)
		}
		if spec.Category == "" {
			t.Errorf("tool %q missing Category", spec.Name)
		}
	}
}

func TestAllToolsHaveExpectedCount(t *testing.T) {
	if len(AllTools) != 12 {
		t.Fatalf("expected 12 tools, got %d", len(AllTools))
	}
}

func TestAllToolDescriptionsHaveUSEWHEN(t *testing.T) {
	for _, spec := range AllTools {
		if !strings.Contains(spec.Description, "USE WHEN") {
			t.Errorf("tool %q description missing 'USE WHEN' pattern", spec.Name)
		}
	}
}

func TestToolNamesAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, spec := range AllTools {
		if seen[spec.Name] {
			t.Errorf("duplicate tool name: %q", spec.Name)
		}
		seen[spec.Name] = true
	}
}

func TestToolMethodsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, spec := range AllTools {
		if seen[spec.Method] {
			t.Errorf("duplicate tool method: %q", spec.Method)
		}
		seen[spec.Method] = true
	}
}
