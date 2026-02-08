// Package common provides shared utilities for language analyzers.
package common

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// InfraPattern defines an infrastructure import pattern with the node/edge to create.
type InfraPattern struct {
	Packages []string
	NodeType model.NodeType
	EdgeType model.EdgeType
	NodeID   string
	NodeName string
}

// ClassifyImport checks if importPath matches any infrastructure pattern
// and returns the corresponding infrastructure node and edge.
// separator is "/" for TypeScript (e.g. "pg/pool") or "." for Python (e.g. "sqlalchemy.orm").
func ClassifyImport(importPath, modID string, patterns []InfraPattern, separator string) ([]*model.Node, []*model.Edge) {
	for _, pattern := range patterns {
		if MatchesAny(importPath, pattern.Packages, separator) {
			nodes := []*model.Node{{
				ID:   pattern.NodeID,
				Name: pattern.NodeName,
				Type: pattern.NodeType,
				Properties: map[string]string{
					"detected_via": importPath,
				},
			}}
			edges := []*model.Edge{{
				Source: modID,
				Target: pattern.NodeID,
				Type:   pattern.EdgeType,
				Label:  pattern.NodeName,
			}}
			return nodes, edges
		}
	}
	return nil, nil
}

// MatchesAny checks if importPath matches any of the patterns.
// Matches exact name or as a prefix with the given separator.
func MatchesAny(importPath string, patterns []string, separator string) bool {
	for _, p := range patterns {
		if importPath == p || strings.HasPrefix(importPath, p+separator) {
			return true
		}
	}
	return false
}

// WalkTree performs a depth-first traversal of a tree-sitter node,
// calling fn for each node encountered.
func WalkTree(node *sitter.Node, fn func(*sitter.Node)) {
	fn(node)
	for i := 0; i < int(node.ChildCount()); i++ {
		WalkTree(node.Child(i), fn)
	}
}
