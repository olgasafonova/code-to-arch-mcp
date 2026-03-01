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

	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/common"
	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
	"github.com/olgasafonova/code-to-arch-mcp/internal/scanner"
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

// Clone returns an independent copy with a fresh tree-sitter parser.
// Tree-sitter parsers are not thread-safe; each goroutine needs its own.
func (a *Analyzer) Clone() scanner.Analyzer {
	return New()
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
	defer tree.Close()
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

	// Detect framework from imports before extracting routes
	framework := detectFramework(root, src)

	// Extract route handlers (Flask/FastAPI decorators)
	routeNodes, routeEdges := extractRoutes(root, src, modID, path, framework)
	nodes = append(nodes, routeNodes...)
	edges = append(edges, routeEdges...)

	// Extract Django URL patterns (urls.py files with path()/re_path() calls)
	if strings.HasSuffix(filepath.Base(path), "urls.py") {
		djangoNodes, djangoEdges := extractDjangoURLPatterns(root, src, modID, path)
		nodes = append(nodes, djangoNodes...)
		edges = append(edges, djangoEdges...)
	}

	// Detect outbound HTTP client calls
	callNodes, callEdges := extractHTTPCalls(root, src, modID)
	nodes = append(nodes, callNodes...)
	edges = append(edges, callEdges...)

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
			Source:     modID,
			Target:     "import:" + importPath,
			Type:       model.EdgeDependency,
			Label:      importPath,
			Confidence: 0.9,
		})

		infraNodes, infraEdges := common.ClassifyImport(importPath, modID, infraPatterns, ".")
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

// infraPatterns defines Python infrastructure package patterns.
var infraPatterns = []common.InfraPattern{
	{
		Packages: []string{"sqlalchemy", "django.db", "pymongo", "psycopg2", "mysql.connector", "sqlite3", "tortoise", "peewee", "databases", "asyncpg"},
		NodeType: model.NodeDatabase,
		EdgeType: model.EdgeReadWrite,
		NodeID:   "infra:database",
		NodeName: "Database",
	},
	{
		Packages: []string{"celery", "kombu", "pika", "kafka", "rq", "dramatiq"},
		NodeType: model.NodeQueue,
		EdgeType: model.EdgePublish,
		NodeID:   "infra:queue",
		NodeName: "Message Queue",
	},
	{
		Packages: []string{"redis", "pymemcache", "aiocache", "django.core.cache", "cachetools"},
		NodeType: model.NodeCache,
		EdgeType: model.EdgeReadWrite,
		NodeID:   "infra:cache",
		NodeName: "Cache",
	},
	{
		Packages: []string{"requests", "httpx", "aiohttp", "urllib3", "httplib2"},
		NodeType: model.NodeExternalAPI,
		EdgeType: model.EdgeAPICall,
		NodeID:   "infra:external_api",
		NodeName: "External API",
	},
}

// HTTP methods recognized as route decorators.
var httpMethods = map[string]bool{
	"route": true, "get": true, "post": true, "put": true, "delete": true, "patch": true,
	"options": true, "head": true,
}

// detectFramework checks imports to determine the web framework in use.
func detectFramework(root *sitter.Node, src []byte) string {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		var importPath string
		switch child.Type() {
		case "import_statement":
			importPath = extractDottedName(child, src)
		case "import_from_statement":
			importPath = extractFromModule(child, src)
		default:
			continue
		}
		if strings.HasPrefix(importPath, "fastapi") {
			return "fastapi"
		}
		if strings.HasPrefix(importPath, "flask") {
			return "flask"
		}
	}
	return "unknown"
}

// extractRoutes finds Flask/FastAPI route decorators.
func extractRoutes(root *sitter.Node, src []byte, modID, filePath, framework string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	common.WalkTree(root, func(node *sitter.Node) {
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

			fw := framework
			if fw == "unknown" {
				// Fallback heuristic: "route" is more common in Flask
				if method == "route" {
					fw = "flask"
				} else {
					fw = "fastapi"
				}
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
					"framework": fw,
				},
			})
			edges = append(edges, &model.Edge{
				Source:     modID,
				Target:     endpointID,
				Type:       model.EdgeAPICall,
				Label:      "serves",
				Confidence: 0.85,
			})
		}
	})

	return nodes, edges
}

// extractDecoratorRoute parses a decorator like @app.route('/path') or @app.get('/path').
func extractDecoratorRoute(decorator *sitter.Node, src []byte) (method, routePath string) {
	// Walk decorator children to find the call expression
	common.WalkTree(decorator, func(node *sitter.Node) {
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

// djangoURLFuncs are Django URL routing functions.
var djangoURLFuncs = map[string]bool{
	"path":    true,
	"re_path": true,
}

// extractDjangoURLPatterns finds Django path()/re_path() calls in urls.py files.
func extractDjangoURLPatterns(root *sitter.Node, src []byte, modID, filePath string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	common.WalkTree(root, func(node *sitter.Node) {
		if node.Type() != "call" {
			return
		}

		fn := node.ChildByFieldName("function")
		if fn == nil {
			return
		}

		// Match path() or re_path() calls
		var funcName string
		switch fn.Type() {
		case "identifier":
			funcName = fn.Content(src)
		case "attribute":
			attr := fn.ChildByFieldName("attribute")
			if attr != nil {
				funcName = attr.Content(src)
			}
		}
		if !djangoURLFuncs[funcName] {
			return
		}

		args := node.ChildByFieldName("arguments")
		if args == nil {
			return
		}

		routePath := extractFirstStringArg(args, src)
		if routePath == "" {
			return
		}

		line := int(node.StartPoint().Row) + 1
		endpointID := fmt.Sprintf("endpoint:%s:%d", filepath.Base(filePath), line)

		nodes = append(nodes, &model.Node{
			ID:       endpointID,
			Name:     fmt.Sprintf("URL %s", routePath),
			Type:     model.NodeEndpoint,
			Language: "python",
			Path:     filePath,
			Properties: map[string]string{
				"method":    funcName,
				"route":     routePath,
				"line":      fmt.Sprintf("%d", line),
				"framework": "django",
			},
		})
		edges = append(edges, &model.Edge{
			Source:     modID,
			Target:     endpointID,
			Type:       model.EdgeAPICall,
			Label:      "serves",
			Confidence: 0.85,
		})
	})

	return nodes, edges
}

// httpClientObjects are receiver names that indicate HTTP client calls in Python.
var httpClientObjects = map[string]bool{
	"requests": true, "httpx": true, "session": true, "client": true,
}

// extractHTTPCalls detects outbound HTTP calls (requests.get, httpx.post, etc.)
// and creates service nodes for target hosts.
func extractHTTPCalls(root *sitter.Node, src []byte, modID string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	common.WalkTree(root, func(node *sitter.Node) {
		if node.Type() != "call" {
			return
		}

		fn := node.ChildByFieldName("function")
		if fn == nil || fn.Type() != "attribute" {
			return
		}

		obj := fn.ChildByFieldName("object")
		attr := fn.ChildByFieldName("attribute")
		if obj == nil || attr == nil {
			return
		}

		objName := obj.Content(src)
		methodName := strings.ToLower(attr.Content(src))
		if !httpClientObjects[objName] || !httpMethods[methodName] {
			return
		}

		args := node.ChildByFieldName("arguments")
		if args == nil {
			return
		}

		rawURL := extractFirstStringArg(args, src)
		if rawURL == "" {
			return
		}

		host, ok := common.ParseServiceFromURL(rawURL)
		if !ok {
			return
		}

		serviceID := "service:" + host
		line := int(node.StartPoint().Row) + 1

		nodes = append(nodes, &model.Node{
			ID:   serviceID,
			Name: host,
			Type: model.NodeExternalAPI,
			Properties: map[string]string{
				"detected_via": "http_call",
			},
		})
		edges = append(edges, &model.Edge{
			Source:     modID,
			Target:     serviceID,
			Type:       model.EdgeAPICall,
			Label:      strings.ToUpper(methodName) + " " + rawURL,
			Confidence: 0.7,
			Properties: map[string]string{
				"url":  rawURL,
				"line": fmt.Sprintf("%d", line),
			},
		})
	})

	return nodes, edges
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
