// Package golang provides Go-specific static analysis using go/ast.
// Detects imports, HTTP endpoints, database connections, and messaging patterns.
package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// Analyzer implements the scanner.Analyzer interface for Go source files.
type Analyzer struct{}

// New creates a Go analyzer.
func New() *Analyzer {
	return &Analyzer{}
}

// Extensions returns Go file extensions.
func (a *Analyzer) Extensions() []string {
	return []string{".go"}
}

// Language returns "go".
func (a *Analyzer) Language() string {
	return "go"
}

// Analyze parses a Go file and extracts architectural elements.
func (a *Analyzer) Analyze(path string) ([]*model.Node, []*model.Edge, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	pkgName := file.Name.Name
	relPath := filepath.Base(filepath.Dir(path))
	pkgID := fmt.Sprintf("pkg:%s/%s", relPath, pkgName)

	var nodes []*model.Node
	var edges []*model.Edge

	// Always create a package node for the file's package
	nodes = append(nodes, &model.Node{
		ID:       pkgID,
		Name:     pkgName,
		Type:     model.NodePackage,
		Language: "go",
		Path:     filepath.Dir(path),
	})

	// Extract imports as dependency edges
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		edges = append(edges, &model.Edge{
			Source: pkgID,
			Target: "import:" + importPath,
			Type:   model.EdgeDependency,
			Label:  importPath,
		})

		// Detect known infrastructure imports
		nodes, edges = a.classifyImport(importPath, pkgID, nodes, edges)
	}

	// Walk AST for endpoint and infrastructure patterns
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		newNodes, newEdges := a.analyzeCall(call, pkgID, path, fset)
		nodes = append(nodes, newNodes...)
		edges = append(edges, newEdges...)

		return true
	})

	return nodes, edges, nil
}

// classifyImport detects infrastructure dependencies from import paths.
func (a *Analyzer) classifyImport(importPath, pkgID string, nodes []*model.Node, edges []*model.Edge) ([]*model.Node, []*model.Edge) {
	switch {
	// Database drivers and ORMs
	case strings.Contains(importPath, "database/sql"),
		strings.Contains(importPath, "gorm.io"),
		strings.Contains(importPath, "pgx"),
		strings.Contains(importPath, "go-sql-driver"),
		strings.Contains(importPath, "sqlx"):
		dbNode := &model.Node{
			ID:   "infra:database",
			Name: "Database",
			Type: model.NodeDatabase,
			Properties: map[string]string{
				"detected_via": importPath,
			},
		}
		nodes = append(nodes, dbNode)
		edges = append(edges, &model.Edge{
			Source: pkgID,
			Target: dbNode.ID,
			Type:   model.EdgeReadWrite,
			Label:  "database access",
		})

	// Message queues
	case strings.Contains(importPath, "amqp"),
		strings.Contains(importPath, "kafka"),
		strings.Contains(importPath, "nats"),
		strings.Contains(importPath, "rabbitmq"):
		queueNode := &model.Node{
			ID:   "infra:queue",
			Name: "Message Queue",
			Type: model.NodeQueue,
			Properties: map[string]string{
				"detected_via": importPath,
			},
		}
		nodes = append(nodes, queueNode)
		edges = append(edges, &model.Edge{
			Source: pkgID,
			Target: queueNode.ID,
			Type:   model.EdgePublish,
			Label:  "message queue",
		})

	// Cache
	case strings.Contains(importPath, "redis"),
		strings.Contains(importPath, "memcache"):
		cacheNode := &model.Node{
			ID:   "infra:cache",
			Name: "Cache",
			Type: model.NodeCache,
			Properties: map[string]string{
				"detected_via": importPath,
			},
		}
		nodes = append(nodes, cacheNode)
		edges = append(edges, &model.Edge{
			Source: pkgID,
			Target: cacheNode.ID,
			Type:   model.EdgeReadWrite,
			Label:  "cache access",
		})

	// HTTP clients (external API calls)
	case importPath == "net/http" && strings.Contains(pkgID, "client"):
		apiNode := &model.Node{
			ID:   "infra:external_api",
			Name: "External API",
			Type: model.NodeExternalAPI,
		}
		nodes = append(nodes, apiNode)
		edges = append(edges, &model.Edge{
			Source: pkgID,
			Target: apiNode.ID,
			Type:   model.EdgeAPICall,
			Label:  "HTTP client",
		})
	}

	return nodes, edges
}

// analyzeCall inspects function call expressions for endpoint patterns.
func (a *Analyzer) analyzeCall(call *ast.CallExpr, pkgID, filePath string, fset *token.FileSet) ([]*model.Node, []*model.Edge) {
	var nodes []*model.Node
	var edges []*model.Edge

	funcName := callFuncName(call)
	if funcName == "" {
		return nodes, edges
	}

	// Detect HTTP handler registrations
	httpPatterns := map[string]bool{
		"HandleFunc": true, "Handle": true,
		"Get": true, "Post": true, "Put": true, "Delete": true, "Patch": true,
		"GET": true, "POST": true, "PUT": true, "DELETE": true, "PATCH": true,
		"Group": true, "Route": true,
	}

	if httpPatterns[funcName] && len(call.Args) >= 1 {
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			route := strings.Trim(lit.Value, `"`)
			pos := fset.Position(call.Pos())
			endpointID := fmt.Sprintf("endpoint:%s:%d", filepath.Base(filePath), pos.Line)

			nodes = append(nodes, &model.Node{
				ID:       endpointID,
				Name:     fmt.Sprintf("%s %s", strings.ToUpper(funcName), route),
				Type:     model.NodeEndpoint,
				Language: "go",
				Path:     filePath,
				Properties: map[string]string{
					"method": funcName,
					"route":  route,
					"line":   fmt.Sprintf("%d", pos.Line),
				},
			})
			edges = append(edges, &model.Edge{
				Source: pkgID,
				Target: endpointID,
				Type:   model.EdgeAPICall,
				Label:  "serves",
			})
		}
	}

	return nodes, edges
}

// callFuncName extracts the function name from a call expression.
func callFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}
