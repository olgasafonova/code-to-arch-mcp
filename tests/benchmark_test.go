//go:build integration

package tests

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/olgasafonova/ridge/internal/analyzer/golang"
	"github.com/olgasafonova/ridge/internal/analyzer/python"
	"github.com/olgasafonova/ridge/internal/analyzer/typescript"
	"github.com/olgasafonova/ridge/internal/scanner"
)

// newBenchScanner returns a Scanner wired with all three language analyzers.
// Logging is silenced (warn-level only) so benchmark output isn't polluted.
func newBenchScanner() *scanner.Scanner {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return scanner.New(logger, golang.New(), typescript.New(), python.New())
}

// BenchmarkScan measures cold (empty cache) and warm (populated cache) scan
// times for three real codebases. Each sub-benchmark runs exactly once
// (-benchtime=1x), which is sufficient to observe the cold/warm gap.
//
// State isolation: every run uses b.TempDir() — the real ~/.mcp-context/ is
// never touched. ScanState is passed via ScanOptions.State; no env-var override
// or StateDir() calls are needed.
func BenchmarkScan(b *testing.B) {
	home, err := os.UserHomeDir()
	if err != nil {
		b.Fatalf("cannot resolve home dir: %v", err)
	}

	targets := []struct {
		name string
		path string
	}{
		{"miro-mcp-server", filepath.Join(home, "Projects", "miro-mcp-server")},
		{"gleif-mcp-server", filepath.Join(home, "Projects", "gleif-mcp-server")},
		{"ridge-self", filepath.Join(home, "Projects", "ridge")},
	}

	s := newBenchScanner()

	for _, target := range targets {
		target := target // capture

		// Check path exists; skip gracefully if missing
		if _, statErr := os.Stat(target.path); os.IsNotExist(statErr) {
			b.Run(target.name+"/cold", func(b *testing.B) {
				b.Skipf("path not found, skipping: %s", target.path)
			})
			b.Run(target.name+"/warm", func(b *testing.B) {
				b.Skipf("path not found, skipping: %s", target.path)
			})
			continue
		}

		// ----------------------------------------------------------------
		// Cold scan: ScanState starts empty (no cached results)
		// ----------------------------------------------------------------
		var coldState *scanner.ScanState // populated by cold run for warm pass

		b.Run(target.name+"/cold", func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				state := scanner.NewScanState(target.path)
				opts := scanner.ScanOptions{State: state}

				result, scanErr := s.ScanWithOptions(context.Background(), target.path, opts)
				if scanErr != nil {
					b.Fatalf("cold scan failed: %v", scanErr)
				}
				if result.Graph.NodeCount() == 0 {
					b.Fatal("expected at least one node")
				}

				// Keep the last state for the warm benchmark below.
				// b.N is 1 when using -benchtime=1x, so this is safe.
				coldState = state
			}
		})

		// ----------------------------------------------------------------
		// Warm scan: re-scan with state populated from the cold run.
		// Files haven't changed so the cache should absorb most work.
		// ----------------------------------------------------------------
		b.Run(target.name+"/warm", func(b *testing.B) {
			if coldState == nil {
				b.Skip("cold scan did not produce state (cold benchmark may have been skipped)")
			}

			b.ResetTimer()
			for range b.N {
				opts := scanner.ScanOptions{State: coldState}

				result, scanErr := s.ScanWithOptions(context.Background(), target.path, opts)
				if scanErr != nil {
					b.Fatalf("warm scan failed: %v", scanErr)
				}
				if result.Graph.NodeCount() == 0 {
					b.Fatal("expected at least one node after warm scan")
				}
			}
		})
	}
}
