// Package typescript provides TypeScript/TSX analysis using tree-sitter.
// Detects imports, HTTP endpoints, database connections, and messaging patterns.
package typescript

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
)

// Analyzer implements the scanner.Analyzer interface for TypeScript/TSX files.
type Analyzer struct {
	tsParser  *sitter.Parser
	tsxParser *sitter.Parser
}

// New creates a TypeScript analyzer with tree-sitter parsers.
func New() *Analyzer {
	tsParser := sitter.NewParser()
	tsParser.SetLanguage(typescript.GetLanguage())

	tsxParser := sitter.NewParser()
	tsxParser.SetLanguage(tsx.GetLanguage())

	return &Analyzer{
		tsParser:  tsParser,
		tsxParser: tsxParser,
	}
}

// Extensions returns TypeScript/TSX file extensions.
func (a *Analyzer) Extensions() []string {
	return []string{".ts", ".tsx"}
}

// Language returns "typescript".
func (a *Analyzer) Language() string {
	return "typescript"
}

// Clone returns an independent copy with fresh tree-sitter parsers.
// Tree-sitter parsers are not thread-safe; each goroutine needs its own.
func (a *Analyzer) Clone() scanner.Analyzer {
	return New()
}

// Analyze parses a TypeScript file and extracts architectural elements.
func (a *Analyzer) Analyze(path string) ([]*model.Node, []*model.Edge, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	parser := a.tsParser
	if strings.HasSuffix(path, ".tsx") {
		parser = a.tsxParser
	}

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	root := tree.RootNode()

	dir := filepath.Base(filepath.Dir(path))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	modID := fmt.Sprintf("mod:%s/%s", dir, base)

	var nodes []*model.Node
	var edges []*model.Edge

	nodes = append(nodes, &model.Node{
		ID:       modID,
		Name:     base,
		Type:     model.NodeModule,
		Language: "typescript",
		Path:     filepath.Dir(path),
	})

	// Extract imports
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "import_statement" {
			continue
		}
		importPath := extractImportSource(child, src)
		if importPath == "" {
			continue
		}

		edges = append(edges, &model.Edge{
			Source: modID,
			Target: "import:" + importPath,
			Type:   model.EdgeDependency,
			Label:  importPath,
		})

		infraNodes, infraEdges := classifyImport(importPath, modID)
		nodes = append(nodes, infraNodes...)
		edges = append(edges, infraEdges...)
	}

	// Extract route handlers
	routeNodes, routeEdges := extractRoutes(root, src, modID, path)
	nodes = append(nodes, routeNodes...)
	edges = append(edges, routeEdges...)

	return nodes, edges, nil
}

// extractImportSource gets the import path string from an import_statement node.
func extractImportSource(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "string" || child.Type() == "string_fragment" {
			return stripQuotes(child.Content(src))
		}
	}
	// Try named child "source"
	source := node.ChildByFieldName("source")
	if source != nil {
		return stripQuotes(source.Content(src))
	}
	return ""
}

func stripQuotes(s string) string {
	s = strings.TrimPrefix(s, "'")
	s = strings.TrimSuffix(s, "'")
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")
	return s
}

// Infrastructure package patterns.
var infraPatterns = []struct {
	packages []string
	nodeType model.NodeType
	edgeType model.EdgeType
	nodeID   string
	nodeName string
}{
	{
		packages: []string{"pg", "knex", "prisma", "@prisma/client", "typeorm", "sequelize", "mongoose", "mongodb", "mysql2", "better-sqlite3", "drizzle-orm", "mikro-orm"},
		nodeType: model.NodeDatabase,
		edgeType: model.EdgeReadWrite,
		nodeID:   "infra:database",
		nodeName: "Database",
	},
	{
		packages: []string{"amqplib", "kafkajs", "bullmq", "bull", "@google-cloud/pubsub", "nats", "@azure/service-bus"},
		nodeType: model.NodeQueue,
		edgeType: model.EdgePublish,
		nodeID:   "infra:queue",
		nodeName: "Message Queue",
	},
	{
		packages: []string{"redis", "ioredis", "@redis/client", "memcached", "keyv"},
		nodeType: model.NodeCache,
		edgeType: model.EdgeReadWrite,
		nodeID:   "infra:cache",
		nodeName: "Cache",
	},
	{
		packages: []string{"axios", "node-fetch", "got", "undici", "superagent", "@apollo/client", "graphql-request"},
		nodeType: model.NodeExternalAPI,
		edgeType: model.EdgeAPICall,
		nodeID:   "infra:external_api",
		nodeName: "External API",
	},
}

func classifyImport(importPath, modID string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	for _, pattern := range infraPatterns {
		if matchesAny(importPath, pattern.packages) {
			nodes = append(nodes, &model.Node{
				ID:   pattern.nodeID,
				Name: pattern.nodeName,
				Type: pattern.nodeType,
				Properties: map[string]string{
					"detected_via": importPath,
				},
			})
			edges = append(edges, &model.Edge{
				Source: modID,
				Target: pattern.nodeID,
				Type:   pattern.edgeType,
				Label:  pattern.nodeName,
			})
			return nodes, edges
		}
	}
	return nil, nil
}

// matchesAny checks if importPath matches any of the patterns.
// Matches exact name or prefix with "/".
func matchesAny(importPath string, patterns []string) bool {
	for _, p := range patterns {
		if importPath == p || strings.HasPrefix(importPath, p+"/") {
			return true
		}
	}
	return false
}

// HTTP method names that indicate route registration.
var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "delete": true, "patch": true,
	"options": true, "head": true, "all": true, "use": true, "route": true,
}

func extractRoutes(root *sitter.Node, src []byte, modID, filePath string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	walkTree(root, func(node *sitter.Node) {
		if node.Type() != "call_expression" {
			return
		}

		fn := node.ChildByFieldName("function")
		if fn == nil || fn.Type() != "member_expression" {
			return
		}

		prop := fn.ChildByFieldName("property")
		if prop == nil {
			return
		}
		methodName := strings.ToLower(prop.Content(src))
		if !httpMethods[methodName] {
			return
		}

		args := node.ChildByFieldName("arguments")
		if args == nil || args.ChildCount() < 2 {
			return
		}

		// First argument should be a string (the route path)
		firstArg := args.Child(1) // Child(0) is "("
		if firstArg == nil {
			return
		}

		routePath := extractStringLiteral(firstArg, src)
		if routePath == "" {
			return
		}

		line := int(node.StartPoint().Row) + 1
		endpointID := fmt.Sprintf("endpoint:%s:%d", filepath.Base(filePath), line)

		nodes = append(nodes, &model.Node{
			ID:       endpointID,
			Name:     fmt.Sprintf("%s %s", strings.ToUpper(methodName), routePath),
			Type:     model.NodeEndpoint,
			Language: "typescript",
			Path:     filePath,
			Properties: map[string]string{
				"method":    methodName,
				"route":     routePath,
				"line":      fmt.Sprintf("%d", line),
				"framework": "express",
			},
		})
		edges = append(edges, &model.Edge{
			Source: modID,
			Target: endpointID,
			Type:   model.EdgeAPICall,
			Label:  "serves",
		})
	})

	return nodes, edges
}

// extractStringLiteral extracts a string value from a tree-sitter string node.
func extractStringLiteral(node *sitter.Node, src []byte) string {
	if node.Type() == "string" {
		// String node contains quote children + string_fragment
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "string_fragment" {
				return child.Content(src)
			}
		}
		// Fallback: strip quotes from the full content
		return stripQuotes(node.Content(src))
	}
	return ""
}

// walkTree performs a depth-first traversal calling fn for each node.
func walkTree(node *sitter.Node, fn func(*sitter.Node)) {
	fn(node)
	for i := 0; i < int(node.ChildCount()); i++ {
		walkTree(node.Child(i), fn)
	}
}
