package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
)

// createTestProject creates a temp Python project for integration tests.
// Returns the temp directory root path.
func createTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"setup.py": "# setup\n",
		"myapp/__init__.py": `from .models import User
`,
		"myapp/main.py": `from .models import User
from .utils import helper
import os
`,
		"myapp/models.py": `from .base import BaseModel
`,
		"myapp/utils.py": `import json
from . import models
`,
		"myapp/base.py":         "# no imports\n",
		"myapp/sub/__init__.py": "",
		"myapp/sub/handler.py": `from ..models import User
from . import helpers
`,
		"myapp/sub/helpers.py": `import logging
`,
		"tests/test_main.py": `from myapp.main import something
`,
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func openTestIndex(t *testing.T) (*db.Index, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := db.OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}
	return idx, path
}

func TestFullIndex(t *testing.T) {
	root := createTestProject(t)
	idx, _ := openTestIndex(t)
	defer idx.Close()

	ix, err := NewIndexer(Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  true,
		Output:   os.Stderr,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Force full index (no git history in temp dir).
	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	// Verify file count.
	fileCount, err := idx.FileCount()
	if err != nil {
		t.Fatal(err)
	}
	// 10 .py files: setup.py, myapp/__init__.py, main.py, models.py, utils.py, base.py,
	//               myapp/sub/__init__.py, handler.py, helpers.py, tests/test_main.py
	if fileCount != 10 {
		t.Errorf("FileCount = %d, want 10", fileCount)
	}

	// Verify edges exist.
	edgeCount, err := idx.EdgeCount()
	if err != nil {
		t.Fatal(err)
	}
	if edgeCount == 0 {
		t.Error("EdgeCount = 0, want > 0")
	}

	// Verify specific edges exist.
	assertEdge(t, idx, "myapp/main.py", "myapp/models.py")
	assertEdge(t, idx, "myapp/sub/handler.py", "myapp/models.py")
	assertEdge(t, idx, "tests/test_main.py", "myapp/main.py")

	// Verify unresolved imports (stdlib: os, json, logging).
	unresolvedCount, err := idx.UnresolvedCount()
	if err != nil {
		t.Fatal(err)
	}
	if unresolvedCount == 0 {
		t.Error("UnresolvedCount = 0, want > 0 (stdlib imports should be unresolved/external)")
	}

	// Verify signals computed.
	var signalCount int
	if err := idx.DB.QueryRow("SELECT COUNT(*) FROM signals").Scan(&signalCount); err != nil {
		t.Fatal(err)
	}
	if signalCount == 0 {
		t.Error("no signals computed")
	}

	// Verify entrypoints detected (files with indegree=0, not __init__.py).
	entrypoints, err := idx.EntrypointCount()
	if err != nil {
		t.Fatal(err)
	}
	if entrypoints == 0 {
		t.Error("no entrypoints detected")
	}

	// Verify package roots stored.
	pkgRoots, err := idx.PackageRootCount()
	if err != nil {
		t.Fatal(err)
	}
	if pkgRoots == 0 {
		t.Error("no package roots stored")
	}

	// Verify metadata set.
	lastIndexed, err := idx.GetMeta("last_indexed_at")
	if err != nil {
		t.Fatal(err)
	}
	if lastIndexed == "" {
		t.Error("last_indexed_at not set")
	}
}

func TestFullIndexEmptyRepo(t *testing.T) {
	root := t.TempDir()
	idx, _ := openTestIndex(t)
	defer idx.Close()

	ix, err := NewIndexer(Config{
		RepoRoot: root,
		Index:    idx,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	fileCount, err := idx.FileCount()
	if err != nil {
		t.Fatal(err)
	}
	if fileCount != 0 {
		t.Errorf("FileCount = %d, want 0 for empty repo", fileCount)
	}
}

func TestFullIndexIdempotent(t *testing.T) {
	root := createTestProject(t)
	idx, _ := openTestIndex(t)
	defer idx.Close()

	ix, err := NewIndexer(Config{
		RepoRoot: root,
		Index:    idx,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Index twice.
	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	count1, _ := idx.FileCount()
	edges1, _ := idx.EdgeCount()

	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	count2, _ := idx.FileCount()
	edges2, _ := idx.EdgeCount()

	if count1 != count2 {
		t.Errorf("FileCount changed: %d → %d", count1, count2)
	}
	if edges1 != edges2 {
		t.Errorf("EdgeCount changed: %d → %d", edges1, edges2)
	}
}

func TestDeletedFile(t *testing.T) {
	root := createTestProject(t)
	idx, _ := openTestIndex(t)
	defer idx.Close()

	ix, err := NewIndexer(Config{
		RepoRoot: root,
		Index:    idx,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	count1, _ := idx.FileCount()

	// Delete a file and re-index.
	os.Remove(filepath.Join(root, "myapp/utils.py"))

	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	count2, _ := idx.FileCount()
	if count2 != count1-1 {
		t.Errorf("FileCount after delete = %d, want %d", count2, count1-1)
	}

	// Verify the deleted file's edges are gone.
	var edgesFromUtils int
	err = idx.DB.QueryRow("SELECT COUNT(*) FROM edges WHERE src_file = 'myapp/utils.py'").Scan(&edgesFromUtils)
	if err != nil {
		t.Fatal(err)
	}
	if edgesFromUtils != 0 {
		t.Errorf("edges from deleted file = %d, want 0", edgesFromUtils)
	}
}

func TestScannerIgnoresDirs(t *testing.T) {
	root := t.TempDir()

	// Create files in ignored and non-ignored directories.
	writeFile(t, root, "main.py", "import os\n")
	writeFile(t, root, "__pycache__/cached.py", "import os\n")
	writeFile(t, root, ".venv/lib/pkg.py", "import os\n")
	writeFile(t, root, "node_modules/pkg.py", "import os\n")
	writeFile(t, root, "src/app.py", "import os\n")

	scanner := NewScanner(root, nil)
	files, err := scanner.ScanAll()
	if err != nil {
		t.Fatal(err)
	}

	paths := make(map[string]bool)
	for _, f := range files {
		paths[f.Path] = true
	}

	if !paths["main.py"] {
		t.Error("expected main.py to be scanned")
	}
	if !paths[filepath.Join("src", "app.py")] {
		t.Error("expected src/app.py to be scanned")
	}
	if paths[filepath.Join("__pycache__", "cached.py")] {
		t.Error("expected __pycache__/cached.py to be ignored")
	}
	if paths[filepath.Join(".venv", "lib", "pkg.py")] {
		t.Error("expected .venv/lib/pkg.py to be ignored")
	}
	if paths[filepath.Join("node_modules", "pkg.py")] {
		t.Error("expected node_modules/pkg.py to be ignored")
	}
}

func assertEdge(t *testing.T, idx *db.Index, src, dst string) {
	t.Helper()
	var count int
	err := idx.DB.QueryRow("SELECT COUNT(*) FROM edges WHERE src_file = ? AND dst_file = ?", src, dst).Scan(&count)
	if err != nil {
		t.Fatalf("querying edge %s → %s: %v", src, dst, err)
	}
	if count == 0 {
		t.Errorf("expected edge %s → %s, not found", src, dst)
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestParallelIndexDeterminism verifies that multiple full index runs produce
// identical results, regardless of goroutine scheduling.
func TestParallelIndexDeterminism(t *testing.T) {
	root := createTestProject(t)

	type snapshot struct {
		fileCount       int
		edgeCount       int
		unresolvedCount int
	}

	var snapshots []snapshot
	for run := 0; run < 5; run++ {
		idx, _ := openTestIndex(t)

		ix, err := NewIndexer(Config{
			RepoRoot: root,
			Index:    idx,
		})
		if err != nil {
			t.Fatal(err)
		}

		if err := ix.fullIndex(); err != nil {
			t.Fatal(err)
		}

		fc, _ := idx.FileCount()
		ec, _ := idx.EdgeCount()
		uc, _ := idx.UnresolvedCount()
		snapshots = append(snapshots, snapshot{fc, ec, uc})

		idx.Close()
	}

	for i := 1; i < len(snapshots); i++ {
		if snapshots[i] != snapshots[0] {
			t.Errorf("run %d differs from run 0: %+v vs %+v", i, snapshots[i], snapshots[0])
		}
	}
}

// BenchmarkFullIndex measures the performance of a full index operation.
func BenchmarkFullIndex(b *testing.B) {
	root := createBenchProject(b)

	for i := 0; i < b.N; i++ {
		dir := b.TempDir()
		dbPath := filepath.Join(dir, "bench.sqlite")
		idx, err := db.OpenIndex(dbPath)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := idx.MigrateToLatest(); err != nil {
			b.Fatal(err)
		}

		ix, err := NewIndexer(Config{
			RepoRoot: root,
			Index:    idx,
		})
		if err != nil {
			b.Fatal(err)
		}

		if err := ix.fullIndex(); err != nil {
			b.Fatal(err)
		}
		idx.Close()
	}
}

// createBenchProject creates a temp project with enough files to exercise parallelism.
func createBenchProject(b *testing.B) string {
	b.Helper()
	root := b.TempDir()

	// Create 100 Python files with cross-imports.
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("pkg/mod_%d.py", i)
		var content string
		if i > 0 {
			content = fmt.Sprintf("from .mod_%d import something\nimport os\n", i-1)
		} else {
			content = "import os\n"
		}
		abs := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	// Create __init__.py.
	initPath := filepath.Join(root, "pkg/__init__.py")
	if err := os.WriteFile(initPath, []byte(""), 0o644); err != nil {
		b.Fatal(err)
	}
	// Create setup.py for package root detection.
	setupPath := filepath.Join(root, "setup.py")
	if err := os.WriteFile(setupPath, []byte("# setup\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	return root
}

// TestIndexerConfigIgnore verifies that config-ignored directories
// are not indexed.
func TestIndexerConfigIgnore(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/main.py", "from . import models\n")
	writeFile(t, root, "pkg/models.py", "# models\n")
	writeFile(t, root, "vendor/dep.py", "from pkg import models\n")
	writeFile(t, root, "vendor/util.py", "# util\n")

	cfg := &config.Config{
		Version: 1,
		Ignore:  []string{"vendor/"},
	}

	idx, _ := openTestIndex(t)
	defer idx.Close()

	ix, err := NewIndexer(Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
		Config:   cfg,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	// Verify vendor files are NOT indexed.
	exists, err := idx.FileExists("vendor/dep.py")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("vendor/dep.py should not be indexed when vendor/ is config-ignored")
	}

	exists, err = idx.FileExists("vendor/util.py")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("vendor/util.py should not be indexed when vendor/ is config-ignored")
	}

	// Verify non-ignored files ARE indexed.
	exists, err = idx.FileExists("pkg/main.py")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("pkg/main.py should be indexed")
	}
}

// TestSchemaAwareReindex verifies that Run() forces a full reindex when
// indexed_at_schema doesn't match the current DB schema version.
func TestSchemaAwareReindex(t *testing.T) {
	root := createTestProject(t)
	idx, _ := openTestIndex(t)
	defer idx.Close()

	ix, err := NewIndexer(Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  true,
		Output:   os.Stderr,
	})
	if err != nil {
		t.Fatal(err)
	}

	// First full index — sets indexed_at_schema.
	if err := ix.fullIndex(); err != nil {
		t.Fatal(err)
	}

	schemaVer, err := idx.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	got, err := idx.GetMeta("indexed_at_schema")
	if err != nil {
		t.Fatal(err)
	}
	if got != strconv.Itoa(schemaVer) {
		t.Fatalf("indexed_at_schema = %q, want %q", got, strconv.Itoa(schemaVer))
	}

	// Verify edges have imported_names populated (at least one non-NULL).
	var withNames int
	err = idx.DB.QueryRow("SELECT COUNT(*) FROM edges WHERE imported_names IS NOT NULL AND imported_names != ''").Scan(&withNames)
	if err != nil {
		t.Fatal(err)
	}
	if withNames == 0 {
		t.Fatal("expected at least one edge with imported_names after full index")
	}

	// Simulate a schema migration without reindex: clear imported_names
	// and remove indexed_at_schema.
	if _, err := idx.DB.Exec("UPDATE edges SET imported_names = NULL"); err != nil {
		t.Fatal(err)
	}
	if err := idx.SetMeta("indexed_at_schema", ""); err != nil {
		t.Fatal(err)
	}

	// Run() should detect the schema mismatch and force full reindex.
	if err := ix.Run(); err != nil {
		t.Fatal(err)
	}

	// Verify imported_names are repopulated.
	var withNamesAfter int
	err = idx.DB.QueryRow("SELECT COUNT(*) FROM edges WHERE imported_names IS NOT NULL AND imported_names != ''").Scan(&withNamesAfter)
	if err != nil {
		t.Fatal(err)
	}
	if withNamesAfter == 0 {
		t.Error("expected imported_names to be repopulated after schema-aware reindex")
	}

	// Verify indexed_at_schema is set again.
	got, err = idx.GetMeta("indexed_at_schema")
	if err != nil {
		t.Fatal(err)
	}
	if got != strconv.Itoa(schemaVer) {
		t.Errorf("indexed_at_schema after reindex = %q, want %q", got, strconv.Itoa(schemaVer))
	}
}

func TestMergeEdgeSymbols(t *testing.T) {
	t.Run("merges duplicate keys", func(t *testing.T) {
		edges := []pendingEdge{
			{src: "a.ts", dst: "b.ts", specifier: "./b", method: "tsconfig_paths", line: 1, importedNames: []string{"MatchData"}},
			{src: "a.ts", dst: "b.ts", specifier: "./b", method: "tsconfig_paths", line: 5, importedNames: []string{"createRouter"}},
		}
		merged := mergeEdgeSymbols(edges)
		if len(merged) != 1 {
			t.Fatalf("len(merged) = %d, want 1", len(merged))
		}
		if len(merged[0].importedNames) != 2 {
			t.Fatalf("len(importedNames) = %d, want 2", len(merged[0].importedNames))
		}
		names := make(map[string]bool)
		for _, n := range merged[0].importedNames {
			names[n] = true
		}
		if !names["MatchData"] || !names["createRouter"] {
			t.Errorf("importedNames = %v, want [MatchData, createRouter]", merged[0].importedNames)
		}
		// Preserves first edge's line number.
		if merged[0].line != 1 {
			t.Errorf("line = %d, want 1", merged[0].line)
		}
	})

	t.Run("different keys not merged", func(t *testing.T) {
		edges := []pendingEdge{
			{src: "a.ts", dst: "b.ts", specifier: "./b", importedNames: []string{"Foo"}},
			{src: "a.ts", dst: "c.ts", specifier: "./c", importedNames: []string{"Bar"}},
		}
		merged := mergeEdgeSymbols(edges)
		if len(merged) != 2 {
			t.Fatalf("len(merged) = %d, want 2", len(merged))
		}
	})

	t.Run("nil importedNames handled", func(t *testing.T) {
		edges := []pendingEdge{
			{src: "a.ts", dst: "b.ts", specifier: "./b", importedNames: nil},
			{src: "a.ts", dst: "b.ts", specifier: "./b", importedNames: []string{"Foo"}},
		}
		merged := mergeEdgeSymbols(edges)
		if len(merged) != 1 {
			t.Fatalf("len(merged) = %d, want 1", len(merged))
		}
		if len(merged[0].importedNames) != 1 || merged[0].importedNames[0] != "Foo" {
			t.Errorf("importedNames = %v, want [Foo]", merged[0].importedNames)
		}
	})

	t.Run("deduplicates overlapping names", func(t *testing.T) {
		edges := []pendingEdge{
			{src: "a.ts", dst: "b.ts", specifier: "./b", importedNames: []string{"Foo", "Bar"}},
			{src: "a.ts", dst: "b.ts", specifier: "./b", importedNames: []string{"Bar", "Baz"}},
		}
		merged := mergeEdgeSymbols(edges)
		if len(merged) != 1 {
			t.Fatalf("len(merged) = %d, want 1", len(merged))
		}
		if len(merged[0].importedNames) != 3 {
			t.Errorf("len(importedNames) = %d, want 3 (Foo, Bar, Baz)", len(merged[0].importedNames))
		}
	})

	t.Run("single edge passthrough", func(t *testing.T) {
		edges := []pendingEdge{
			{src: "a.ts", dst: "b.ts", specifier: "./b", importedNames: []string{"Foo"}},
		}
		merged := mergeEdgeSymbols(edges)
		if len(merged) != 1 {
			t.Fatalf("len(merged) = %d, want 1", len(merged))
		}
	})

	t.Run("empty slice passthrough", func(t *testing.T) {
		merged := mergeEdgeSymbols(nil)
		if len(merged) != 0 {
			t.Fatalf("len(merged) = %d, want 0", len(merged))
		}
	})
}
