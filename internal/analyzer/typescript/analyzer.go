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

	"github.com/olgasafonova/code-to-arch-mcp/internal/analyzer/common"
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
			Source:     modID,
			Target:     "import:" + importPath,
			Type:       model.EdgeDependency,
			Label:      importPath,
			Confidence: 0.9,
		})

		infraNodes, infraEdges := common.ClassifyImport(importPath, modID, infraPatterns, "/")
		nodes = append(nodes, infraNodes...)
		edges = append(edges, infraEdges...)
	}

	// Detect framework from imports before extracting routes
	framework := detectFramework(root, src)

	// Extract route handlers
	routeNodes, routeEdges := extractRoutes(root, src, modID, path, framework)
	nodes = append(nodes, routeNodes...)
	edges = append(edges, routeEdges...)

	// Extract NestJS decorator-based routes
	if framework == "nestjs" {
		nestNodes, nestEdges := extractNestJSRoutes(root, src, modID, path)
		nodes = append(nodes, nestNodes...)
		edges = append(edges, nestEdges...)
	}

	// Detect outbound HTTP client calls
	callNodes, callEdges := extractHTTPCalls(root, src, modID)
	nodes = append(nodes, callNodes...)
	edges = append(edges, callEdges...)

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

// infraPatterns defines TypeScript infrastructure package patterns.
var infraPatterns = []common.InfraPattern{
	{
		Packages: []string{"pg", "knex", "prisma", "@prisma/client", "typeorm", "sequelize", "mongoose", "mongodb", "mysql2", "better-sqlite3", "drizzle-orm", "mikro-orm"},
		NodeType: model.NodeDatabase,
		EdgeType: model.EdgeReadWrite,
		NodeID:   "infra:database",
		NodeName: "Database",
	},
	{
		Packages: []string{"amqplib", "kafkajs", "bullmq", "bull", "@google-cloud/pubsub", "nats", "@azure/service-bus"},
		NodeType: model.NodeQueue,
		EdgeType: model.EdgePublish,
		NodeID:   "infra:queue",
		NodeName: "Message Queue",
	},
	{
		Packages: []string{"redis", "ioredis", "@redis/client", "memcached", "keyv"},
		NodeType: model.NodeCache,
		EdgeType: model.EdgeReadWrite,
		NodeID:   "infra:cache",
		NodeName: "Cache",
	},
	{
		Packages: []string{"axios", "node-fetch", "got", "undici", "superagent", "@apollo/client", "graphql-request"},
		NodeType: model.NodeExternalAPI,
		EdgeType: model.EdgeAPICall,
		NodeID:   "infra:external_api",
		NodeName: "External API",
	},
}

// HTTP method names that indicate route registration.
var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "delete": true, "patch": true,
	"options": true, "head": true, "all": true, "use": true, "route": true,
}

// frameworkPackages maps npm package names to framework labels.
var frameworkPackages = map[string]string{
	"express":        "express",
	"fastify":        "fastify",
	"koa":            "koa",
	"hapi":           "hapi",
	"@hapi/hapi":     "hapi",
	"restify":        "restify",
	"@nestjs/common": "nestjs",
	"@nestjs/core":   "nestjs",
}

// nestjsRouteDecorators maps NestJS HTTP method decorator names to HTTP methods.
var nestjsRouteDecorators = map[string]string{
	"Get": "GET", "Post": "POST", "Put": "PUT",
	"Delete": "DELETE", "Patch": "PATCH", "All": "ALL",
	"Head": "HEAD", "Options": "OPTIONS",
}

// detectFramework checks imports to determine the web framework in use.
func detectFramework(root *sitter.Node, src []byte) string {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "import_statement" {
			continue
		}
		importPath := extractImportSource(child, src)
		if fw, ok := frameworkPackages[importPath]; ok {
			return fw
		}
	}
	return "unknown"
}

func extractRoutes(root *sitter.Node, src []byte, modID, filePath, framework string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	common.WalkTree(root, func(node *sitter.Node) {
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
				"framework": framework,
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

// extractNestJSRoutes detects NestJS @Controller/@Get/@Post decorator patterns
// and creates endpoint nodes. Also detects @Injectable() as service nodes.
func extractNestJSRoutes(root *sitter.Node, src []byte, modID, filePath string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	common.WalkTree(root, func(node *sitter.Node) {
		if node.Type() != "class_declaration" {
			return
		}

		// Look for @Controller decorator on or above this class
		controllerPath := findControllerPath(node, src)
		if controllerPath == "" {
			// Check @Injectable for service detection
			if hasDecorator(node, src, "Injectable") {
				className := extractClassName(node, src)
				if className != "" {
					line := int(node.StartPoint().Row) + 1
					serviceID := fmt.Sprintf("service:%s:%d", filepath.Base(filePath), line)
					nodes = append(nodes, &model.Node{
						ID:       serviceID,
						Name:     className,
						Type:     model.NodeModule,
						Language: "typescript",
						Path:     filePath,
						Properties: map[string]string{
							"injectable": "true",
							"framework":  "nestjs",
							"line":       fmt.Sprintf("%d", line),
						},
					})
					edges = append(edges, &model.Edge{
						Source:     modID,
						Target:     serviceID,
						Type:       model.EdgeDependency,
						Label:      "provides",
						Confidence: 0.85,
					})
				}
			}
			return
		}

		body := node.ChildByFieldName("body")
		if body == nil {
			return
		}

		// Walk class body: decorators appear as siblings before method_definition
		var pendingDecorators []*sitter.Node
		for i := 0; i < int(body.ChildCount()); i++ {
			child := body.Child(i)
			if child.Type() == "decorator" {
				pendingDecorators = append(pendingDecorators, child)
				continue
			}
			if child.Type() != "method_definition" {
				pendingDecorators = nil
				continue
			}

			// Check pending sibling decorators for route decorators
			httpMethod, routePath := findRouteDecorator(pendingDecorators, src)
			if httpMethod == "" {
				// Also check children of the method node (some grammars nest decorators)
				httpMethod, routePath = findRouteDecorator(collectChildDecorators(child), src)
			}
			pendingDecorators = nil

			if httpMethod == "" {
				continue
			}

			fullPath := joinPaths(controllerPath, routePath)
			line := int(child.StartPoint().Row) + 1
			endpointID := fmt.Sprintf("endpoint:%s:%d", filepath.Base(filePath), line)

			nodes = append(nodes, &model.Node{
				ID:       endpointID,
				Name:     fmt.Sprintf("%s %s", httpMethod, fullPath),
				Type:     model.NodeEndpoint,
				Language: "typescript",
				Path:     filePath,
				Properties: map[string]string{
					"method":    strings.ToLower(httpMethod),
					"route":     fullPath,
					"line":      fmt.Sprintf("%d", line),
					"framework": "nestjs",
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

// findControllerPath looks for @Controller('path') on a class_declaration.
// Checks both the node's own children and its parent's children (for export_statement wrapping).
func findControllerPath(classNode *sitter.Node, src []byte) string {
	// Check decorators that are children of the class node
	for i := 0; i < int(classNode.ChildCount()); i++ {
		child := classNode.Child(i)
		if child.Type() == "decorator" {
			name, arg := extractDecoratorInfo(child, src)
			if name == "Controller" {
				if arg == "" {
					return "/"
				}
				return arg
			}
		}
	}

	// Check preceding siblings in the parent (export_statement or program)
	parent := classNode.Parent()
	if parent == nil {
		return ""
	}
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == classNode {
			break // Stop once we reach the class itself
		}
		if child.Type() == "decorator" {
			name, arg := extractDecoratorInfo(child, src)
			if name == "Controller" {
				if arg == "" {
					return "/"
				}
				return arg
			}
		}
	}

	return ""
}

// hasDecorator checks if a class has a specific decorator (by name).
func hasDecorator(classNode *sitter.Node, src []byte, decoratorName string) bool {
	// Check own children
	for i := 0; i < int(classNode.ChildCount()); i++ {
		child := classNode.Child(i)
		if child.Type() == "decorator" {
			name, _ := extractDecoratorInfo(child, src)
			if name == decoratorName {
				return true
			}
		}
	}
	// Check preceding siblings in parent
	parent := classNode.Parent()
	if parent == nil {
		return false
	}
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == classNode {
			break
		}
		if child.Type() == "decorator" {
			name, _ := extractDecoratorInfo(child, src)
			if name == decoratorName {
				return true
			}
		}
	}
	return false
}

// extractDecoratorInfo extracts the decorator name and first string argument.
// For @Controller('/users') returns ("Controller", "/users").
// For @Get() returns ("Get", "").
func extractDecoratorInfo(decorator *sitter.Node, src []byte) (name, arg string) {
	for i := 0; i < int(decorator.ChildCount()); i++ {
		child := decorator.Child(i)
		if child.Type() == "call_expression" {
			fn := child.ChildByFieldName("function")
			if fn != nil && fn.Type() == "identifier" {
				name = fn.Content(src)
			}
			args := child.ChildByFieldName("arguments")
			if args != nil {
				// First real argument (skip parentheses)
				for j := 0; j < int(args.ChildCount()); j++ {
					argChild := args.Child(j)
					if argChild.Type() == "string" {
						arg = extractStringLiteral(argChild, src)
						break
					}
				}
			}
			return name, arg
		}
		// Bare decorator without call: @Get (no parentheses)
		if child.Type() == "identifier" {
			return child.Content(src), ""
		}
	}
	return "", ""
}

// extractClassName gets the class name from a class_declaration node.
func extractClassName(node *sitter.Node, src []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return nameNode.Content(src)
	}
	return ""
}

// findRouteDecorator checks a slice of decorator nodes for NestJS route decorators.
// Returns the HTTP method and route path, or empty strings if none found.
func findRouteDecorator(decorators []*sitter.Node, src []byte) (httpMethod, routePath string) {
	for _, d := range decorators {
		name, arg := extractDecoratorInfo(d, src)
		if method, ok := nestjsRouteDecorators[name]; ok {
			return method, arg
		}
	}
	return "", ""
}

// collectChildDecorators gathers decorator child nodes from a given parent.
func collectChildDecorators(node *sitter.Node) []*sitter.Node {
	var decorators []*sitter.Node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "decorator" {
			decorators = append(decorators, child)
		}
	}
	return decorators
}

// joinPaths combines a controller base path with a method route path.
func joinPaths(base, route string) string {
	base = strings.TrimSuffix(base, "/")
	if route == "" {
		if base == "" {
			return "/"
		}
		return base
	}
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	return base + route
}

// httpClientObjects are receiver/object names that indicate HTTP client calls.
var httpClientObjects = map[string]bool{
	"axios": true, "http": true, "client": true, "api": true,
}

// extractHTTPCalls detects outbound HTTP calls (fetch, axios.get, etc.)
// and creates service nodes for target hosts.
func extractHTTPCalls(root *sitter.Node, src []byte, modID string) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	common.WalkTree(root, func(node *sitter.Node) {
		if node.Type() != "call_expression" {
			return
		}

		fn := node.ChildByFieldName("function")
		if fn == nil {
			return
		}

		var method string

		switch fn.Type() {
		case "identifier":
			// fetch("http://...")
			if fn.Content(src) != "fetch" {
				return
			}
			method = "GET"
		case "member_expression":
			// axios.get("http://...") or client.post("http://...")
			obj := fn.ChildByFieldName("object")
			prop := fn.ChildByFieldName("property")
			if obj == nil || prop == nil {
				return
			}
			objName := obj.Content(src)
			propName := strings.ToLower(prop.Content(src))
			if !httpClientObjects[objName] || !httpMethods[propName] {
				return
			}
			method = strings.ToUpper(propName)
		default:
			return
		}

		args := node.ChildByFieldName("arguments")
		if args == nil || args.ChildCount() < 2 {
			return
		}
		firstArg := args.Child(1) // Child(0) is "("
		if firstArg == nil {
			return
		}

		rawURL := extractStringLiteral(firstArg, src)
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
			Label:      method + " " + rawURL,
			Confidence: 0.7,
			Properties: map[string]string{
				"url":  rawURL,
				"line": fmt.Sprintf("%d", line),
			},
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
