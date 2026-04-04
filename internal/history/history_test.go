package history

import (
	"os"
	"testing"
	"time"

	"github.com/kehoej/contextception/internal/model"
)

func TestRecordAndQuery(t *testing.T) {
	dir, err := os.MkdirTemp("", "history-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	report := &model.ChangeReport{
		SchemaVersion: "1.0",
		RefRange:      "main..feature",
		ChangedFiles: []model.ChangedFile{
			{File: "src/main.py", Status: "modified", BlastRadius: &model.BlastRadius{Level: "high"}},
			{File: "src/utils.py", Status: "modified", BlastRadius: &model.BlastRadius{Level: "low"}},
		},
		Summary: model.ChangeSummary{
			TotalFiles:    2,
			Modified:      2,
			HighRiskFiles: 1,
		},
		BlastRadius: &model.BlastRadius{Level: "high", Detail: "test", Fragility: 0.5},
		Hotspots:    []string{"src/main.py"},
		TestGaps:    []string{"src/utils.py"},
		Coupling:    []model.CouplingPair{{FileA: "src/main.py", FileB: "src/utils.py", Direction: "a_imports_b"}},
		MustRead:    []model.MustReadEntry{},
		LikelyModify: map[string][]model.LikelyModifyEntry{},
		Tests:       []model.TestEntry{},
	}

	id, err := store.RecordRun(report, "abc123", "feature")
	if err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive run ID, got %d", id)
	}

	// Test trend query.
	trends, err := store.GetBlastRadiusTrend(10, "")
	if err != nil {
		t.Fatalf("GetBlastRadiusTrend: %v", err)
	}
	if len(trends) != 1 {
		t.Fatalf("expected 1 trend, got %d", len(trends))
	}
	if trends[0].Level != "high" {
		t.Errorf("trend level = %q, want %q", trends[0].Level, "high")
	}

	// Test branch filter.
	trends, err = store.GetBlastRadiusTrend(10, "other-branch")
	if err != nil {
		t.Fatalf("GetBlastRadiusTrend (filtered): %v", err)
	}
	if len(trends) != 0 {
		t.Errorf("expected 0 trends for other-branch, got %d", len(trends))
	}

	// Test hotspot evolution.
	hotspots, err := store.GetHotspotEvolution(10, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("GetHotspotEvolution: %v", err)
	}
	if len(hotspots) != 1 {
		t.Fatalf("expected 1 hotspot, got %d", len(hotspots))
	}
	if hotspots[0].File != "src/main.py" {
		t.Errorf("hotspot file = %q, want %q", hotspots[0].File, "src/main.py")
	}

	// Test distribution.
	dist, err := store.GetBlastDistribution(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("GetBlastDistribution: %v", err)
	}
	if len(dist) != 1 {
		t.Fatalf("expected 1 distribution entry, got %d", len(dist))
	}
	if dist[0].Level != "high" || dist[0].Count != 1 {
		t.Errorf("distribution = %+v, want high/1", dist[0])
	}

	// Test file history.
	fileHist, err := store.GetFileRiskHistory("src/main.py", 10)
	if err != nil {
		t.Fatalf("GetFileRiskHistory: %v", err)
	}
	if len(fileHist) != 1 {
		t.Fatalf("expected 1 file history entry, got %d", len(fileHist))
	}
	if fileHist[0].BlastLevel != "high" {
		t.Errorf("file blast level = %q, want %q", fileHist[0].BlastLevel, "high")
	}

	// Test run count.
	count, err := store.RunCount()
	if err != nil {
		t.Fatalf("RunCount: %v", err)
	}
	if count != 1 {
		t.Errorf("run count = %d, want 1", count)
	}
}
