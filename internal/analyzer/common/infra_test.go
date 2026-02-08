package common

import (
	"testing"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

var testPatterns = []InfraPattern{
	{
		Packages: []string{"pg", "prisma", "typeorm"},
		NodeType: model.NodeDatabase,
		EdgeType: model.EdgeReadWrite,
		NodeID:   "infra:database",
		NodeName: "Database",
	},
	{
		Packages: []string{"redis", "ioredis"},
		NodeType: model.NodeCache,
		EdgeType: model.EdgeReadWrite,
		NodeID:   "infra:cache",
		NodeName: "Cache",
	},
}

func TestClassifyImport_Match(t *testing.T) {
	nodes, edges := ClassifyImport("pg", "mod:test", testPatterns, "/")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Type != model.NodeDatabase {
		t.Fatalf("expected database node, got %s", nodes[0].Type)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
}

func TestClassifyImport_NoMatch(t *testing.T) {
	nodes, edges := ClassifyImport("express", "mod:test", testPatterns, "/")
	if nodes != nil || edges != nil {
		t.Fatal("expected nil for unmatched import")
	}
}

func TestMatchesAny_TSStyle(t *testing.T) {
	patterns := []string{"pg", "prisma"}
	if !MatchesAny("pg", patterns, "/") {
		t.Fatal("expected exact match")
	}
	if !MatchesAny("pg/pool", patterns, "/") {
		t.Fatal("expected prefix match with /")
	}
	if MatchesAny("pgx", patterns, "/") {
		t.Fatal("pgx should not match pg")
	}
}

func TestMatchesAny_PythonStyle(t *testing.T) {
	patterns := []string{"sqlalchemy", "django.db"}
	if !MatchesAny("sqlalchemy", patterns, ".") {
		t.Fatal("expected exact match")
	}
	if !MatchesAny("sqlalchemy.orm", patterns, ".") {
		t.Fatal("expected prefix match with .")
	}
	if MatchesAny("sqlalchemyx", patterns, ".") {
		t.Fatal("sqlalchemyx should not match sqlalchemy")
	}
}
