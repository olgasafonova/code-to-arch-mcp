package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/olgasafonova/ridge/internal/model"
)

func writeGoFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnalyze_BasicPackage(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "main.go", `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`)

	a := New()
	nodes, edges, err := a.Analyze(path)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should find at least the package node
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}

	foundPkg := false
	for _, n := range nodes {
		if n.Type == model.NodePackage && n.Name == "main" {
			foundPkg = true
		}
	}
	if !foundPkg {
		t.Fatal("expected to find package node 'main'")
	}

	// Should have dependency edge for fmt
	foundFmt := false
	for _, e := range edges {
		if e.Type == model.EdgeDependency && e.Label == "fmt" {
			foundFmt = true
		}
	}
	if !foundFmt {
		t.Fatal("expected dependency edge for fmt import")
	}
}

func TestAnalyze_DetectsDatabase(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "db.go", `package repo

import "database/sql"

var db *sql.DB
`)

	a := New()
	nodes, edges, err := a.Analyze(path)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	foundDB := false
	for _, n := range nodes {
		if n.Type == model.NodeDatabase {
			foundDB = true
		}
	}
	if !foundDB {
		t.Fatal("expected database node from database/sql import")
	}

	foundRW := false
	for _, e := range edges {
		if e.Type == model.EdgeReadWrite {
			foundRW = true
		}
	}
	if !foundRW {
		t.Fatal("expected read_write edge to database")
	}
}

func TestAnalyze_DetectsHTTPEndpoint(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "server.go", `package main

import "net/http"

func main() {
	http.HandleFunc("/api/health", healthHandler)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {}
`)

	a := New()
	nodes, _, err := a.Analyze(path)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	foundEndpoint := false
	for _, n := range nodes {
		if n.Type == model.NodeEndpoint {
			foundEndpoint = true
			if n.Properties["route"] != "/api/health" {
				t.Fatalf("expected route /api/health, got %s", n.Properties["route"])
			}
		}
	}
	if !foundEndpoint {
		t.Fatal("expected endpoint node from HandleFunc")
	}
}

func TestAnalyze_DetectsMessageQueue(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "publisher.go", `package events

import "github.com/streadway/amqp"

var _ amqp.Channel
`)

	a := New()
	nodes, _, err := a.Analyze(path)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	foundQueue := false
	for _, n := range nodes {
		if n.Type == model.NodeQueue {
			foundQueue = true
		}
	}
	if !foundQueue {
		t.Fatal("expected queue node from amqp import")
	}
}

func TestAnalyze_DetectsCache(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "cache.go", `package store

import "github.com/go-redis/redis/v8"

var _ redis.Client
`)

	a := New()
	nodes, _, err := a.Analyze(path)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	foundCache := false
	for _, n := range nodes {
		if n.Type == model.NodeCache {
			foundCache = true
		}
	}
	if !foundCache {
		t.Fatal("expected cache node from redis import")
	}
}

func TestAnalyze_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := writeGoFile(t, dir, "bad.go", `not valid go`)

	a := New()
	_, _, err := a.Analyze(path)
	if err == nil {
		t.Fatal("expected error for invalid Go file")
	}
}

func TestExtensions(t *testing.T) {
	a := New()
	exts := a.Extensions()
	if len(exts) != 1 || exts[0] != ".go" {
		t.Fatalf("expected [.go], got %v", exts)
	}
}

func TestLanguage(t *testing.T) {
	a := New()
	if a.Language() != "go" {
		t.Fatalf("expected 'go', got %s", a.Language())
	}
}
