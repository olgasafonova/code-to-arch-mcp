// Package scanner provides the file walker and analyzer orchestration layer.
package scanner

import "github.com/olgasafonova/code-to-arch-mcp/internal/model"

// Analyzer is implemented by each language-specific analyzer.
// An analyzer receives a file path and returns nodes and edges discovered in that file.
type Analyzer interface {
	// Analyze processes a single source file and returns architectural elements found in it.
	Analyze(path string) ([]*model.Node, []*model.Edge, error)

	// Extensions returns the file extensions this analyzer handles (e.g., ".go", ".ts").
	Extensions() []string

	// Language returns the language name for metadata.
	Language() string

	// Clone returns an independent copy safe for use in a separate goroutine.
	// Tree-sitter parsers are not thread-safe; each worker needs its own instance.
	Clone() Analyzer
}
