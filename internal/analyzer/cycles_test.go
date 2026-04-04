package analyzer

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/kehoej/contextception/internal/db"
)

// setupTestIndex creates a temporary SQLite index for testing.
func setupTestIndex(t *testing.T) *db.Index {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "cycles_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpPath) })

	idx, err := db.OpenIndex(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.MigrateToLatest(); err != nil {
		idx.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { idx.Close() })

	return idx
}

// insertFile adds a file to the index within a transaction.
func insertFile(t *testing.T, dbConn *sql.DB, path string) {
	t.Helper()
	_, err := dbConn.Exec(
		"INSERT OR IGNORE INTO files (path, content_hash, last_modified, language, size_bytes) VALUES (?, 'hash', 0, 'go', 100)",
		path,
	)
	if err != nil {
		t.Fatal(err)
	}
}

// insertEdge adds an edge to the index.
func insertEdge(t *testing.T, dbConn *sql.DB, src, dst string) {
	t.Helper()
	_, err := dbConn.Exec(
		"INSERT OR IGNORE INTO edges (src_file, dst_file, edge_type, specifier, resolution_method, line_number) VALUES (?, ?, 'import', ?, 'direct', 1)",
		src, dst, dst,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDetectCycles_NoCycle(t *testing.T) {
	idx := setupTestIndex(t)
	// A → B → C (no cycle)
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		insertFile(t, idx.DB, f)
	}
	insertEdge(t, idx.DB, "a.go", "b.go")
	insertEdge(t, idx.DB, "b.go", "c.go")

	cycles := detectCycles(idx, "a.go", []string{"a.go", "b.go", "c.go"})
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d: %v", len(cycles), cycles)
	}
}

func TestDetectCycles_SimpleCycle(t *testing.T) {
	idx := setupTestIndex(t)
	// A → B → A (simple cycle)
	for _, f := range []string{"a.go", "b.go"} {
		insertFile(t, idx.DB, f)
	}
	insertEdge(t, idx.DB, "a.go", "b.go")
	insertEdge(t, idx.DB, "b.go", "a.go")

	cycles := detectCycles(idx, "a.go", []string{"a.go", "b.go"})
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
	// Cycle should be [a.go, b.go, a.go]
	cycle := cycles[0]
	if len(cycle) != 3 || cycle[0] != "a.go" || cycle[1] != "b.go" || cycle[2] != "a.go" {
		t.Errorf("unexpected cycle: %v", cycle)
	}
}

func TestDetectCycles_ThreeNodeCycle(t *testing.T) {
	idx := setupTestIndex(t)
	// A → B → C → A
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		insertFile(t, idx.DB, f)
	}
	insertEdge(t, idx.DB, "a.go", "b.go")
	insertEdge(t, idx.DB, "b.go", "c.go")
	insertEdge(t, idx.DB, "c.go", "a.go")

	cycles := detectCycles(idx, "a.go", []string{"a.go", "b.go", "c.go"})
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
	cycle := cycles[0]
	if len(cycle) != 4 || cycle[0] != "a.go" || cycle[3] != "a.go" {
		t.Errorf("unexpected cycle: %v", cycle)
	}
}

func TestDetectCycles_MaxCyclesCapped(t *testing.T) {
	idx := setupTestIndex(t)
	// A → B1..B6, each Bi → A (6 separate 2-node cycles, capped at 5)
	insertFile(t, idx.DB, "a.go")
	for i := 1; i <= 6; i++ {
		f := fmt.Sprintf("b%d.go", i)
		insertFile(t, idx.DB, f)
		insertEdge(t, idx.DB, "a.go", f)
		insertEdge(t, idx.DB, f, "a.go")
	}

	allFiles := []string{"a.go", "b1.go", "b2.go", "b3.go", "b4.go", "b5.go", "b6.go"}
	cycles := detectCycles(idx, "a.go", allFiles)
	if len(cycles) > maxCycles {
		t.Errorf("expected at most %d cycles, got %d", maxCycles, len(cycles))
	}
}

func TestDetectCycles_NoSubjectImports(t *testing.T) {
	idx := setupTestIndex(t)
	// A has no imports (leaf file)
	insertFile(t, idx.DB, "a.go")

	cycles := detectCycles(idx, "a.go", []string{"a.go"})
	if len(cycles) != 0 {
		t.Errorf("expected no cycles for leaf file, got %d", len(cycles))
	}
}

func TestCycleParticipants(t *testing.T) {
	cycles := [][]string{
		{"a.go", "b.go", "a.go"},
		{"a.go", "c.go", "d.go", "a.go"},
	}
	participants := cycleParticipants(cycles)

	expected := map[string]bool{"a.go": true, "b.go": true, "c.go": true, "d.go": true}
	for k, v := range expected {
		if participants[k] != v {
			t.Errorf("expected participants[%q] = %v, got %v", k, v, participants[k])
		}
	}
	if participants["e.go"] {
		t.Error("unexpected participant e.go")
	}
}
