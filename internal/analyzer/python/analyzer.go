// Package python provides Python analysis using tree-sitter.
// Detects imports, HTTP endpoints (Flask, FastAPI, Django), database connections, and messaging patterns.
package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Analyzer implements the scanner.Analyzer interface for Python files.
type Analyzer struct {
	parser *sitter.Parser
}

// New creates a Python analyzer with a tree-sitter parser.
func New() *Analyzer {
	p := sitter.NewParser()
	p.SetLanguage(python.GetLanguage())
	return &Analyzer{parser: p}
}

// Extensions returns Python file extensions.
func (a *Analyzer) Extensions() []string {
	return []string{".py"}
}

// Language returns "python".
func (a *Analyzer) Language() string {
	return "python"
}

// Analyze parses a Python file and extracts architectural elements.
func (a *Analyzer) Analyze(path string) ([]*model.Node, []*model.Edge, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}

	tree, err := a.parser.ParseCtx(context.Background(), nil, src)
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
		Language: "python",
		Path:     filepath.Dir(path),
	})

	// Extract imports
	importNodes, importEdges := extractImports(root, src, modID)
	nodes = append(nodes, importNodes...)
	edges = append(edges, importEdges...)

	// Extract route handlers (Flask/FastAPI decorators)
	routeNodes, routeEdges := extractRoutes(root, src, modID, path)
	nodes = append(nodes, routeNodes...)
	edges = append(edges, routeEdges...)

	return nodes, edges, nil
}

// extractImports walks the root's children for import and from-import statements.
func extractImports(root *sitter.Node, src []byte, modID string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		var importPath string

		switch child.Type() {
		case "import_statement":
			// import os, import sqlalchemy
			importPath = extractDottedName(child, src)
		case "import_from_statement":
			// from flask import Flask
			importPath = extractFromModule(child, src)
		default:
			continue
		}

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

	return nodes, edges
}

// extractDottedName gets the module name from an import_statement.
func extractDottedName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "dotted_name" {
			return child.Content(src)
		}
	}
	return ""
}

// extractFromModule gets the module name from an import_from_statement.
func extractFromModule(node *sitter.Node, src []byte) string {
	moduleName := node.ChildByFieldName("module_name")
	if moduleName != nil {
		return moduleName.Content(src)
	}
	// Fallback: find the dotted_name or relative_import child
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "dotted_name" {
			return child.Content(src)
		}
	}
	return ""
}

// Infrastructure package patterns for Python.
var infraPatterns = []struct {
	packages []string
	nodeType model.NodeType
	edgeType model.EdgeType
	nodeID   string
	nodeName string
}{
	{
		packages: []string{"sqlalchemy", "django.db", "pymongo", "psycopg2", "mysql.connector", "sqlite3", "tortoise", "peewee", "databases", "asyncpg"},
		nodeType: model.NodeDatabase,
		edgeType: model.EdgeReadWrite,
		nodeID:   "infra:database",
		nodeName: "Database",
	},
	{
		packages: []string{"celery", "kombu", "pika", "kafka", "rq", "dramatiq"},
		nodeType: model.NodeQueue,
		edgeType: model.EdgePublish,
		nodeID:   "infra:queue",
		nodeName: "Message Queue",
	},
	{
		packages: []string{"redis", "pymemcache", "aiocache", "django.core.cache", "cachetools"},
		nodeType: model.NodeCache,
		edgeType: model.EdgeReadWrite,
		nodeID:   "infra:cache",
		nodeName: "Cache",
	},
	{
		packages: []string{"requests", "httpx", "aiohttp", "urllib3", "httplib2"},
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

// matchesAny checks if importPath matches any pattern exactly or as a prefix.
func matchesAny(importPath string, patterns []string) bool {
	for _, p := range patterns {
		if importPath == p || strings.HasPrefix(importPath, p+".") {
			return true
		}
	}
	return false
}

// HTTP methods recognized as route decorators.
var httpMethods = map[string]bool{
	"route": true, "get": true, "post": true, "put": true, "delete": true, "patch": true,
	"options": true, "head": true,
}

// extractRoutes finds Flask/FastAPI route decorators.
func extractRoutes(root *sitter.Node, src []byte, modID, filePath string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	walkTree(root, func(node *sitter.Node) {
		if node.Type() != "decorated_definition" {
			return
		}

		// Look for decorator nodes within the decorated_definition
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "decorator" {
				continue
			}

			method, routePath := extractDecoratorRoute(child, src)
			if routePath == "" {
				continue
			}

			line := int(node.StartPoint().Row) + 1
			endpointID := fmt.Sprintf("endpoint:%s:%d", filepath.Base(filePath), line)

			framework := "flask"
			if method != "route" {
				framework = "fastapi"
			}

			nodes = append(nodes, &model.Node{
				ID:       endpointID,
				Name:     fmt.Sprintf("%s %s", strings.ToUpper(method), routePath),
				Type:     model.NodeEndpoint,
				Language: "python",
				Path:     filePath,
				Properties: map[string]string{
					"method":    method,
					"route":     routePath,
					"line":      fmt.Sprintf("%d", line),
					"framework": framework,
				},
			})
			edges = append(edges, &model.Edge{
				Source: modID,
				Target: endpointID,
				Type:   model.EdgeAPICall,
				Label:  "serves",
			})
		}
	})

	return nodes, edges
}

// extractDecoratorRoute parses a decorator like @app.route('/path') or @app.get('/path').
func extractDecoratorRoute(decorator *sitter.Node, src []byte) (method, routePath string) {
	// Walk decorator children to find the call expression
	walkTree(decorator, func(node *sitter.Node) {
		if routePath != "" {
			return // already found
		}
		if node.Type() != "call" {
			return
		}

		fn := node.ChildByFieldName("function")
		if fn == nil {
			return
		}

		// Check for attribute pattern: app.route, app.get, etc.
		if fn.Type() == "attribute" {
			attr := fn.ChildByFieldName("attribute")
			if attr == nil {
				return
			}
			methodName := strings.ToLower(attr.Content(src))
			if !httpMethods[methodName] {
				return
			}

			// Extract first string argument
			args := node.ChildByFieldName("arguments")
			if args == nil {
				return
			}

			path := extractFirstStringArg(args, src)
			if path != "" {
				method = methodName
				routePath = path
			}
		}
	})

	return method, routePath
}

// extractFirstStringArg finds the first string literal in an argument_list node.
func extractFirstStringArg(args *sitter.Node, src []byte) string {
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Type() == "string" {
			return stripPythonString(child.Content(src))
		}
	}
	return ""
}

// stripPythonString removes quotes from Python string literals.
func stripPythonString(s string) string {
	// Handle triple-quoted strings
	for _, q := range []string{`"""`, `'''`} {
		if strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			return s[3 : len(s)-3]
		}
	}
	// Handle single/double quoted strings
	for _, q := range []string{`"`, `'`} {
		if strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// walkTree performs a depth-first traversal calling fn for each node.
func walkTree(node *sitter.Node, fn func(*sitter.Node)) {
	fn(node)
	for i := 0; i < int(node.ChildCount()); i++ {
		walkTree(node.Child(i), fn)
	}
}
