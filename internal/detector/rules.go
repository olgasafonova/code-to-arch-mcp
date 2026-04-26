package detector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/olgasafonova/ridge/internal/model"
)

// RulesConfig holds custom architecture validation rules loaded from .arch-rules.yaml.
type RulesConfig struct {
	Rules []Rule `yaml:"rules"`
}

// Rule defines a single architecture constraint.
type Rule struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Type        string  `yaml:"type"`     // "no_dependency" or "require_dependency"
	Severity    string  `yaml:"severity"` // "critical", "high", "medium", "low"
	From        Matcher `yaml:"from"`
	To          Matcher `yaml:"to"`
}

// Matcher filters nodes by type and/or path glob.
type Matcher struct {
	Type string `yaml:"type,omitempty"` // node type (endpoint, database, service, module, etc.)
	Path string `yaml:"path,omitempty"` // glob pattern on relative node path
}

// LoadRules reads and parses an .arch-rules.yaml file.
func LoadRules(path string) (*RulesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading rules file: %w", err)
	}
	var cfg RulesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing rules file: %w", err)
	}
	for i, r := range cfg.Rules {
		if r.Name == "" {
			return nil, fmt.Errorf("rule %d: name is required", i)
		}
		if r.Type != "no_dependency" && r.Type != "require_dependency" {
			return nil, fmt.Errorf("rule %q: type must be no_dependency or require_dependency", r.Name)
		}
	}
	return &cfg, nil
}

// CheckCustomRules evaluates custom rules against the graph.
func CheckCustomRules(graph *model.ArchGraph, rules *RulesConfig, rootPath string) []Violation {
	var violations []Violation
	for _, rule := range rules.Rules {
		switch rule.Type {
		case "no_dependency":
			violations = append(violations, checkNoDependency(graph, rule, rootPath)...)
		case "require_dependency":
			violations = append(violations, checkRequireDependency(graph, rule, rootPath)...)
		}
	}
	return violations
}

func checkNoDependency(graph *model.ArchGraph, rule Rule, rootPath string) []Violation {
	var violations []Violation
	for _, e := range graph.Edges() {
		sourceNode := graph.GetNode(e.Source)
		targetNode := graph.GetNode(e.Target)
		if sourceNode == nil || targetNode == nil {
			continue
		}
		if matchNode(sourceNode, rule.From, rootPath) && matchNode(targetNode, rule.To, rootPath) {
			violations = append(violations, Violation{
				Rule:     rule.Name,
				Severity: parseSeverity(rule.Severity),
				Subject:  fmt.Sprintf("%s -> %s", sourceNode.Name, targetNode.Name),
				Detail:   ruleDetail(rule, sourceNode, targetNode),
			})
		}
	}
	return violations
}

func checkRequireDependency(graph *model.ArchGraph, rule Rule, rootPath string) []Violation {
	// Find all source nodes matching From
	var sources []*model.Node
	for _, n := range graph.Nodes() {
		if matchNode(n, rule.From, rootPath) {
			sources = append(sources, n)
		}
	}
	if len(sources) == 0 {
		return nil
	}

	// Check each source has at least one edge to a matching target
	var violations []Violation
	for _, src := range sources {
		found := false
		for _, e := range graph.EdgesFrom(src.ID) {
			targetNode := graph.GetNode(e.Target)
			if targetNode != nil && matchNode(targetNode, rule.To, rootPath) {
				found = true
				break
			}
		}
		if !found {
			violations = append(violations, Violation{
				Rule:     rule.Name,
				Severity: parseSeverity(rule.Severity),
				Subject:  src.Name,
				Detail:   fmt.Sprintf("%s: %s %q has no dependency matching target criteria", rule.Name, src.Type, src.Name),
			})
		}
	}
	return violations
}

func matchNode(node *model.Node, matcher Matcher, rootPath string) bool {
	if matcher.Type != "" {
		if string(node.Type) != matcher.Type {
			return false
		}
	}
	if matcher.Path != "" {
		relPath := node.Path
		if rootPath != "" && strings.HasPrefix(relPath, rootPath) {
			relPath = strings.TrimPrefix(relPath, rootPath)
			relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
		}
		matched, _ := filepath.Match(matcher.Path, relPath)
		if !matched {
			// Try matching with /** suffix for directory glob
			dirMatched, _ := filepath.Match(matcher.Path, filepath.Dir(relPath))
			if !dirMatched {
				return false
			}
		}
	}
	return matcher.Type != "" || matcher.Path != ""
}

func parseSeverity(s string) model.DiffSeverity {
	switch strings.ToLower(s) {
	case "critical":
		return model.SeverityCritical
	case "high":
		return model.SeverityHigh
	case "medium":
		return model.SeverityMedium
	case "low":
		return model.SeverityLow
	default:
		return model.SeverityMedium
	}
}

func ruleDetail(rule Rule, source, target *model.Node) string {
	if rule.Description != "" {
		return fmt.Sprintf("%s: %s (%s) -> %s (%s)", rule.Description, source.Name, source.Type, target.Name, target.Type)
	}
	return fmt.Sprintf("%s: %s (%s) -> %s (%s)", rule.Name, source.Name, source.Type, target.Name, target.Type)
}
