package db

import (
	"path/filepath"
	"testing"
)

func TestSearchByPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "src/auth/login.py", "h1", 1, "python", 200); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "src/auth/models.py", "h2", 1, "python", 300); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "src/api/routes.py", "h3", 1, "python", 150); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Substring match.
	results, err := idx.SearchByPath("auth", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'auth', got %d", len(results))
	}

	// Case-insensitive.
	results2, err := idx.SearchByPath("AUTH", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results2) != 2 {
		t.Fatalf("expected 2 results for 'AUTH' (case-insensitive), got %d", len(results2))
	}

	// Limit.
	results3, err := idx.SearchByPath("src", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results3) != 1 {
		t.Fatalf("expected 1 result with limit=1, got %d", len(results3))
	}

	// No match.
	results4, err := idx.SearchByPath("nonexistent", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results4) != 0 {
		t.Fatalf("expected 0 results for 'nonexistent', got %d", len(results4))
	}
}

func TestSearchBySymbol(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/models.py", "h1", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/views.py", "h2", 1, "python", 200); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/api.py", "h3", 1, "python", 150); err != nil {
		t.Fatal(err)
	}
	// views.py imports models.py with symbols ["Auth", "AuthService"]
	if err := InsertEdgeTx(tx, "pkg/views.py", "pkg/models.py", "import", "models", "relative", 1, []string{"Auth", "AuthService"}); err != nil {
		t.Fatal(err)
	}
	// api.py imports models.py with symbols ["User"]
	if err := InsertEdgeTx(tx, "pkg/api.py", "pkg/models.py", "import", "models", "relative", 1, []string{"User"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Exact match: "Auth" should find models.py (via views.py import).
	results, err := idx.SearchBySymbol("Auth", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'Auth', got %d", len(results))
	}
	if results[0].Path != "pkg/models.py" {
		t.Errorf("expected pkg/models.py, got %q", results[0].Path)
	}

	// "Auth" should NOT match "AuthService" (exact match, not substring).
	resultsAS, err := idx.SearchBySymbol("AuthService", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(resultsAS) != 1 {
		t.Fatalf("expected 1 result for 'AuthService', got %d", len(resultsAS))
	}

	// "User" should find models.py (via api.py import).
	resultsU, err := idx.SearchBySymbol("User", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(resultsU) != 1 {
		t.Fatalf("expected 1 result for 'User', got %d", len(resultsU))
	}

	// No match.
	resultsNone, err := idx.SearchBySymbol("NonExistent", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(resultsNone) != 0 {
		t.Fatalf("expected 0 results for 'NonExistent', got %d", len(resultsNone))
	}
}

func TestGetEntrypoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "cmd/main.go", "h1", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/server.go", "h2", 1, "go", 200); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/util.go", "h3", 1, "go", 50); err != nil {
		t.Fatal(err)
	}
	// main.go is entrypoint.
	if _, err := tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"cmd/main.go", 0, 5, 1, 0); err != nil {
		t.Fatal(err)
	}
	// server.go is not entrypoint.
	if _, err := tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"pkg/server.go", 10, 3, 0, 0); err != nil {
		t.Fatal(err)
	}
	// util.go is utility.
	if _, err := tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"pkg/util.go", 2, 8, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	results, err := idx.GetEntrypoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 entrypoint, got %d", len(results))
	}
	if results[0].Path != "cmd/main.go" {
		t.Errorf("expected cmd/main.go, got %q", results[0].Path)
	}
	if results[0].Outdegree != 5 {
		t.Errorf("expected outdegree=5, got %d", results[0].Outdegree)
	}
}

func TestGetFoundations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "cmd/main.go", "h1", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/models.go", "h2", 1, "go", 200); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/server.go", "h3", 1, "go", 300); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "pkg/util.go", "h4", 1, "go", 50); err != nil {
		t.Fatal(err)
	}
	// Entrypoint — should be excluded.
	if _, err := tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"cmd/main.go", 0, 5, 1, 0); err != nil {
		t.Fatal(err)
	}
	// High indegree, not entrypoint or utility — should be foundation.
	if _, err := tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"pkg/models.go", 15, 2, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"pkg/server.go", 8, 3, 0, 0); err != nil {
		t.Fatal(err)
	}
	// Utility — should be excluded.
	if _, err := tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"pkg/util.go", 5, 8, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	results, err := idx.GetFoundations(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 foundations, got %d", len(results))
	}
	// Ordered by indegree desc.
	if results[0].Path != "pkg/models.go" {
		t.Errorf("expected pkg/models.go first, got %q", results[0].Path)
	}
	if results[0].Indegree != 15 {
		t.Errorf("expected indegree=15, got %d", results[0].Indegree)
	}
	if results[1].Path != "pkg/server.go" {
		t.Errorf("expected pkg/server.go second, got %q", results[1].Path)
	}

	// Test limit.
	limited, err := idx.GetFoundations(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 foundation with limit=1, got %d", len(limited))
	}
}

func TestGetDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "src/auth/login.py", "h1", 1, "python", 200); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "src/auth/models.py", "h2", 1, "python", 300); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "src/api/routes.ts", "h3", 1, "typescript", 150); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "tests/test_auth.py", "h4", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err := InsertFileTx(tx, "setup.py", "h5", 1, "python", 50); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	results, err := idx.GetDirectoryStructure()
	if err != nil {
		t.Fatal(err)
	}

	// Build a map for easier assertions.
	type key struct{ dir, lang string }
	counts := make(map[key]int)
	for _, r := range results {
		counts[key{r.Dir, r.Language}] = r.FileCount
	}

	// src/ has 2 python + 1 typescript = 3 files.
	if counts[key{"src", "python"}] != 2 {
		t.Errorf("src/python = %d, want 2", counts[key{"src", "python"}])
	}
	if counts[key{"src", "typescript"}] != 1 {
		t.Errorf("src/typescript = %d, want 1", counts[key{"src", "typescript"}])
	}
	// tests/ has 1 python file.
	if counts[key{"tests", "python"}] != 1 {
		t.Errorf("tests/python = %d, want 1", counts[key{"tests", "python"}])
	}
	// Root has 1 python file.
	if counts[key{".", "python"}] != 1 {
		t.Errorf("./python = %d, want 1", counts[key{".", "python"}])
	}
}

func TestGetEntrypointsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	results, err := idx.GetEntrypoints()
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil for empty index, got %v", results)
	}
}

func TestGetFoundationsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	results, err := idx.GetFoundations(10)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil for empty index, got %v", results)
	}
}
