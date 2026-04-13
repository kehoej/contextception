package history

import (
	"os"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	// Create .contextception directory.
	ccDir := filepath.Join(dir, ".contextception")
	if err := os.MkdirAll(ccDir, 0o755); err != nil {
		t.Fatalf("creating dir: %v", err)
	}
	store, err := Open(dir)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestStoreRiskScore_Insert(t *testing.T) {
	store := openTestStore(t)
	err := store.StoreRiskScore("pkg/core.go", 72, "main..HEAD")
	if err != nil {
		t.Fatalf("StoreRiskScore: %v", err)
	}

	// Verify it was inserted.
	var count int
	err = store.db.QueryRow("SELECT COUNT(*) FROM risk_scores").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("count=%d, want 1", count)
	}
}

func TestStoreRiskScores_Batch(t *testing.T) {
	store := openTestStore(t)
	entries := []RiskScoreEntry{
		{FilePath: "a.go", RiskScore: 50, RefRange: "main..HEAD"},
		{FilePath: "b.go", RiskScore: 30, RefRange: "main..HEAD"},
		{FilePath: "c.go", RiskScore: 80, RefRange: "main..HEAD"},
	}
	err := store.StoreRiskScores(entries)
	if err != nil {
		t.Fatalf("StoreRiskScores: %v", err)
	}

	var count int
	err = store.db.QueryRow("SELECT COUNT(*) FROM risk_scores").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 3 {
		t.Errorf("count=%d, want 3", count)
	}
}

func TestStoreRiskScores_Empty(t *testing.T) {
	store := openTestStore(t)
	err := store.StoreRiskScores(nil)
	if err != nil {
		t.Fatalf("StoreRiskScores with nil: %v", err)
	}
}

func TestComputePercentile_SufficientData(t *testing.T) {
	store := openTestStore(t)

	// Insert 15 records with various scores.
	for i := 0; i < 15; i++ {
		score := (i + 1) * 6 // 6, 12, 18, 24, ..., 90
		err := store.StoreRiskScore("file.go", score, "ref")
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	pct, err := store.ComputePercentile(72)
	if err != nil {
		t.Fatalf("ComputePercentile: %v", err)
	}
	// 72 is higher than scores 6,12,18,24,30,36,42,48,54,60,66 (11 of 15) = 73%
	if pct < 50 || pct > 90 {
		t.Errorf("percentile=%d, want 50-90 for score 72 out of 15 records", pct)
	}
	t.Logf("percentile=%d", pct)
}

func TestComputePercentile_InsufficientData(t *testing.T) {
	store := openTestStore(t)

	// Insert only 5 records.
	for i := 0; i < 5; i++ {
		err := store.StoreRiskScore("file.go", (i+1)*20, "ref")
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	pct, err := store.ComputePercentile(50)
	if err != nil {
		t.Fatalf("ComputePercentile: %v", err)
	}
	if pct != 0 {
		t.Errorf("percentile=%d, want 0 (insufficient data)", pct)
	}
}

func TestComputePercentile_EmptyTable(t *testing.T) {
	store := openTestStore(t)

	pct, err := store.ComputePercentile(50)
	if err != nil {
		t.Fatalf("ComputePercentile: %v", err)
	}
	if pct != 0 {
		t.Errorf("percentile=%d, want 0 (empty table)", pct)
	}
}

func TestComputePercentile_ScoreVersionFilter(t *testing.T) {
	store := openTestStore(t)

	// Insert 10 records with version 1 (current).
	for i := 0; i < 10; i++ {
		err := store.StoreRiskScore("file.go", (i+1)*10, "ref")
		if err != nil {
			t.Fatalf("insert v1: %v", err)
		}
	}

	// Insert 10 records with a different version (simulate old formula).
	for i := 0; i < 10; i++ {
		_, err := store.db.Exec(
			`INSERT INTO risk_scores (file_path, risk_score, score_version, ref_range) VALUES (?, ?, ?, ?)`,
			"file.go", (i+1)*10, 999, "ref",
		)
		if err != nil {
			t.Fatalf("insert v999: %v", err)
		}
	}

	// Percentile should only consider current version.
	pct, err := store.ComputePercentile(50)
	if err != nil {
		t.Fatalf("ComputePercentile: %v", err)
	}
	// Score 50 is above 10,20,30,40 (4 of 10) = 40%
	if pct != 40 {
		t.Errorf("percentile=%d, want 40", pct)
	}
}

func TestRiskScoresTable_Migration(t *testing.T) {
	store := openTestStore(t)

	// Verify the table exists by querying its schema.
	var tableName string
	err := store.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='risk_scores'`,
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("risk_scores table not found: %v", err)
	}
	if tableName != "risk_scores" {
		t.Errorf("table name: got %q, want 'risk_scores'", tableName)
	}

	// Verify all expected columns exist via insert+query.
	_, err = store.db.Exec(
		`INSERT INTO risk_scores (file_path, risk_score, score_version, ref_range)
		 VALUES ('test.go', 50, 1, 'main..HEAD')`,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var filePath string
	var riskScore, scoreVersion int
	var refRange string
	var createdAt string
	err = store.db.QueryRow(
		`SELECT file_path, risk_score, score_version, ref_range, created_at FROM risk_scores WHERE id=1`,
	).Scan(&filePath, &riskScore, &scoreVersion, &refRange, &createdAt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if filePath != "test.go" {
		t.Errorf("file_path: got %q, want 'test.go'", filePath)
	}
	if riskScore != 50 {
		t.Errorf("risk_score: got %d, want 50", riskScore)
	}
	if scoreVersion != 1 {
		t.Errorf("score_version: got %d, want 1", scoreVersion)
	}
	if createdAt == "" {
		t.Error("created_at should be auto-populated")
	}

	// Verify indexes exist.
	var indexCount int
	err = store.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_risk_scores%'`,
	).Scan(&indexCount)
	if err != nil {
		t.Fatalf("index query: %v", err)
	}
	if indexCount < 2 {
		t.Errorf("expected at least 2 risk_scores indexes, got %d", indexCount)
	}
}
