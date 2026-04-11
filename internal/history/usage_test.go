package history

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kehoej/contextception/internal/model"
)

func TestRecordAndQueryUsage(t *testing.T) {
	dir, err := os.MkdirTemp("", "usage-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	// Record some usage entries.
	entry1 := &UsageEntry{
		Source:            "cli",
		Tool:              "analyze",
		FilesAnalyzed:     []string{"src/main.py"},
		MustReadCount:     3,
		LikelyModifyCount: 2,
		TestCount:         1,
		BlastLevel:        "medium",
		Confidence:        0.92,
		HasConfidence:     true,
		ResponseTokens:    200,
		DurationMs:        150,
		Mode:              "implement",
	}
	id1, err := store.RecordUsage(entry1)
	if err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}
	if id1 <= 0 {
		t.Fatalf("expected positive ID, got %d", id1)
	}

	entry2 := &UsageEntry{
		Source:            "mcp",
		Tool:              "get_context",
		FilesAnalyzed:     []string{"src/auth/login.py", "src/auth/session.py"},
		MustReadCount:     5,
		LikelyModifyCount: 3,
		TestCount:         2,
		BlastLevel:        "high",
		Confidence:        0.85,
		HasConfidence:     true,
		ResponseTokens:    450,
		DurationMs:        300,
		Mode:              "plan",
	}
	id2, err := store.RecordUsage(entry2)
	if err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}
	if id2 <= id1 {
		t.Fatalf("expected id2 > id1, got %d <= %d", id2, id1)
	}

	// Test usage count.
	count, err := store.UsageCount()
	if err != nil {
		t.Fatalf("UsageCount: %v", err)
	}
	if count != 2 {
		t.Errorf("usage count = %d, want 2", count)
	}

	// Test summary.
	summary, err := store.GetUsageSummary(time.Now().Add(-1 * time.Hour))
	if err != nil {
		t.Fatalf("GetUsageSummary: %v", err)
	}
	if summary.TotalAnalyses != 2 {
		t.Errorf("total analyses = %d, want 2", summary.TotalAnalyses)
	}
	if summary.FilesAnalyzed != 3 { // 1 + 2
		t.Errorf("total files = %d, want 3", summary.FilesAnalyzed)
	}
	if summary.AvgConfidence < 0.88 || summary.AvgConfidence > 0.89 {
		t.Errorf("avg confidence = %.2f, want ~0.885", summary.AvgConfidence)
	}

	// Test blast level counts.
	if summary.BlastLevelCounts["medium"] != 1 {
		t.Errorf("medium count = %d, want 1", summary.BlastLevelCounts["medium"])
	}
	if summary.BlastLevelCounts["high"] != 1 {
		t.Errorf("high count = %d, want 1", summary.BlastLevelCounts["high"])
	}

	// Test period query.
	daily, err := store.GetUsageByPeriod("day", time.Now().Add(-1*time.Hour), 10)
	if err != nil {
		t.Fatalf("GetUsageByPeriod: %v", err)
	}
	if len(daily) != 1 {
		t.Fatalf("expected 1 daily period, got %d", len(daily))
	}
	if daily[0].Analyses != 2 {
		t.Errorf("daily analyses = %d, want 2", daily[0].Analyses)
	}

	// Test top files.
	topFiles, err := store.GetTopFiles(time.Now().Add(-1*time.Hour), 10)
	if err != nil {
		t.Fatalf("GetTopFiles: %v", err)
	}
	if len(topFiles) != 3 {
		t.Fatalf("expected 3 top files, got %d", len(topFiles))
	}
	// All should have count 1.
	for _, f := range topFiles {
		if f.Count != 1 {
			t.Errorf("file %s count = %d, want 1", f.File, f.Count)
		}
	}
}

func TestUsageEntryFromAnalysis(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:    "test.py",
		Confidence: 0.95,
		MustRead: []model.MustReadEntry{
			{File: "a.py"},
			{File: "b.py"},
		},
		LikelyModify: map[string][]model.LikelyModifyEntry{
			"pkg": {{File: "c.py", Confidence: "high"}},
		},
		Tests: []model.TestEntry{
			{File: "test_a.py", Direct: true},
		},
		BlastRadius: &model.BlastRadius{Level: "medium", Detail: "test"},
	}

	entry := UsageEntryFromAnalysis("cli", "analyze", []string{"test.py"}, output, 100, "implement", 0)

	if entry.Source != "cli" {
		t.Errorf("source = %q, want %q", entry.Source, "cli")
	}
	if entry.MustReadCount != 2 {
		t.Errorf("must_read_count = %d, want 2", entry.MustReadCount)
	}
	if entry.LikelyModifyCount != 1 {
		t.Errorf("likely_modify_count = %d, want 1", entry.LikelyModifyCount)
	}
	if entry.TestCount != 1 {
		t.Errorf("test_count = %d, want 1", entry.TestCount)
	}
	if entry.BlastLevel != "medium" {
		t.Errorf("blast_level = %q, want %q", entry.BlastLevel, "medium")
	}
	if entry.Confidence != 0.95 {
		t.Errorf("confidence = %f, want 0.95", entry.Confidence)
	}
	if entry.ResponseTokens <= 0 {
		t.Error("expected positive response_tokens")
	}
}

func TestUsageEntryFromChangeReport(t *testing.T) {
	report := &model.ChangeReport{
		RefRange: "main..HEAD",
		ChangedFiles: []model.ChangedFile{
			{File: "a.py", Status: "modified"},
			{File: "b.py", Status: "added"},
		},
		MustRead: []model.MustReadEntry{{File: "c.py"}},
		LikelyModify: map[string][]model.LikelyModifyEntry{
			"pkg": {{File: "d.py"}},
		},
		Tests:       []model.TestEntry{{File: "test.py", Direct: true}},
		BlastRadius: &model.BlastRadius{Level: "high"},
	}

	entry := UsageEntryFromChangeReport("mcp", nil, report, 200, "", 0)

	if entry.Tool != "analyze_change" {
		t.Errorf("tool = %q, want %q", entry.Tool, "analyze_change")
	}
	if len(entry.FilesAnalyzed) != 2 {
		t.Errorf("files_analyzed len = %d, want 2", len(entry.FilesAnalyzed))
	}
	if entry.BlastLevel != "high" {
		t.Errorf("blast_level = %q, want %q", entry.BlastLevel, "high")
	}
}

func TestFormatGainSummary(t *testing.T) {
	summary := &UsageSummary{
		ContextAnalyses: 10,
		FilesAnalyzed: 15,
		AvgConfidence: 0.89,
		AvgDurationMs: 200,
		BlastLevelCounts: map[string]int{
			"low":    5,
			"medium": 3,
			"high":   2,
		},
	}

	topFiles := []TopFile{
		{File: "src/main.py", Count: 5, LastSeen: "2026-04-11"},
		{File: "src/auth.py", Count: 3, LastSeen: "2026-04-11"},
	}

	daily := []UsagePeriod{
		{Period: "2026-04-11", Analyses: 5, Files: 8, AvgConfidence: 0.9, AvgDurationMs: 150},
		{Period: "2026-04-10", Analyses: 5, Files: 7, AvgConfidence: 0.88, AvgDurationMs: 250},
	}

	text := FormatGainSummary(summary, topFiles, daily)

	// Verify key content is present.
	if text == "" {
		t.Fatal("expected non-empty output")
	}

	// Check that it contains key info.
	checks := []string{
		"Context analyses:",
		"10",
		"0.89",
		"src/main.py",
		"5 analyses",
	}
	for _, check := range checks {
		if !contains(text, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestGainExportJSON(t *testing.T) {
	export := &GainExport{
		Summary: &UsageSummary{
			ContextAnalyses: 5,
			FilesAnalyzed:   10,
			AvgConfidence:   0.9,
			AvgDurationMs:   100,
			BlastLevelCounts: map[string]int{"low": 5},
		},
		TopFiles: []TopFile{{File: "a.py", Count: 3, LastSeen: "2026-04-11"}},
		Daily:    []UsagePeriod{{Period: "2026-04-11", Analyses: 5}},
	}

	data, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var parsed GainExport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if parsed.Summary.ContextAnalyses != 5 {
		t.Errorf("context_analyses = %d, want 5", parsed.Summary.ContextAnalyses)
	}
}

func TestRecordAndQueryFeedback(t *testing.T) {
	dir, err := os.MkdirTemp("", "feedback-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	// First record a usage entry so feedback can link to it.
	usage := &UsageEntry{
		Source:            "mcp",
		Tool:              "get_context",
		FilesAnalyzed:     []string{"src/auth/login.py"},
		MustReadCount:     4,
		LikelyModifyCount: 2,
		BlastLevel:        "medium",
		Confidence:        0.9,
		DurationMs:        100,
	}
	_, err = store.RecordUsage(usage)
	if err != nil {
		t.Fatalf("RecordUsage: %v", err)
	}

	// Record feedback.
	fb := &FeedbackEntry{
		FilePath:         "src/auth/login.py",
		Usefulness:       4,
		UsefulFiles:      []string{"src/auth/session.py", "src/auth/types.py"},
		UnnecessaryFiles: []string{"src/utils/helpers.py"},
		MissingFiles:     []string{"src/auth/tokens.py"},
		ModifiedFiles:    []string{"src/auth/session.py"},
		Notes:            "Good recommendations overall",
	}
	id, err := store.RecordFeedback(fb)
	if err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive feedback ID, got %d", id)
	}

	// Record a second feedback with lower rating.
	fb2 := &FeedbackEntry{
		FilePath:         "src/config/settings.py",
		Usefulness:       2,
		UsefulFiles:      []string{"src/config/defaults.py"},
		UnnecessaryFiles: []string{"src/db/models.py", "src/api/routes.py"},
		MissingFiles:     []string{"src/config/env.py"},
		Notes:            "Missing key dependency on env.py",
	}
	_, err = store.RecordFeedback(fb2)
	if err != nil {
		t.Fatalf("RecordFeedback 2: %v", err)
	}

	// Test feedback count.
	count, err := store.FeedbackCount()
	if err != nil {
		t.Fatalf("FeedbackCount: %v", err)
	}
	if count != 2 {
		t.Errorf("feedback count = %d, want 2", count)
	}

	// Test accuracy metrics.
	metrics, err := store.GetAccuracyMetrics(time.Now().Add(-1 * time.Hour))
	if err != nil {
		t.Fatalf("GetAccuracyMetrics: %v", err)
	}
	if metrics.TotalRatings != 2 {
		t.Errorf("total ratings = %d, want 2", metrics.TotalRatings)
	}
	if metrics.AvgUsefulness != 3.0 { // (4 + 2) / 2
		t.Errorf("avg usefulness = %.1f, want 3.0", metrics.AvgUsefulness)
	}

	// Precision: useful / (useful + unnecessary)
	// useful=3 (2+1), unnecessary=3 (1+2)
	// precision = 3/6 = 0.5
	if metrics.MustReadPrecision < 0.49 || metrics.MustReadPrecision > 0.51 {
		t.Errorf("precision = %.2f, want ~0.50", metrics.MustReadPrecision)
	}

	// Recall: useful / (useful + missing)
	// useful=3, missing=2 (1+1)
	// recall = 3/5 = 0.6
	if metrics.MustReadRecall < 0.59 || metrics.MustReadRecall > 0.61 {
		t.Errorf("recall = %.2f, want ~0.60", metrics.MustReadRecall)
	}

	// Test lowest rated.
	lowest, err := store.GetLowestRated(time.Now().Add(-1*time.Hour), 5)
	if err != nil {
		t.Fatalf("GetLowestRated: %v", err)
	}
	if len(lowest) != 2 {
		t.Fatalf("expected 2 lowest rated, got %d", len(lowest))
	}
	if lowest[0].Usefulness != 2 {
		t.Errorf("lowest[0] usefulness = %d, want 2", lowest[0].Usefulness)
	}
	if lowest[0].FilePath != "src/config/settings.py" {
		t.Errorf("lowest[0] file = %q, want %q", lowest[0].FilePath, "src/config/settings.py")
	}
}

func TestFormatAccuracySummary(t *testing.T) {
	metrics := &AccuracyMetrics{
		TotalRatings:         10,
		AvgUsefulness:        3.8,
		MustReadPrecision:    0.78,
		MustReadRecall:       0.85,
		LikelyModifyAccuracy: 0.72,
	}

	lowest := []RatedAnalysis{
		{FilePath: "src/utils/helpers.py", Usefulness: 2, Notes: "Too many unrelated files"},
	}

	text := FormatAccuracySummary(metrics, lowest)

	checks := []string{
		"Recommendation Accuracy",
		"10 rated",
		"78%",
		"85%",
		"72%",
		"3.8",
		"helpers.py",
	}
	for _, check := range checks {
		if !contains(text, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestFormatAccuracySummaryEmpty(t *testing.T) {
	metrics := &AccuracyMetrics{TotalRatings: 0}
	text := FormatAccuracySummary(metrics, nil)

	if !contains(text, "No feedback recorded") {
		t.Error("expected empty state message")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
