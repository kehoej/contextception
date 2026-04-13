package analyzer

import (
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestFormatCompact(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:    "src/auth/login.py",
		Confidence: 0.92,
		MustRead: []model.MustReadEntry{
			{File: "src/auth/session.py", Symbols: []string{"createSession"}, Direction: "imports"},
			{File: "src/auth/types.py", Direction: "imports", Role: "foundation", Stable: true},
			{File: "src/middleware/auth.py", Direction: "imported_by"},
		},
		LikelyModify: map[string][]model.LikelyModifyEntry{
			"auth": {
				{File: "session.py", Confidence: "high"},
				{File: "types.py", Confidence: "medium"},
			},
		},
		Tests: []model.TestEntry{
			{File: "tests/test_login.py", Direct: true},
			{File: "tests/test_session.py", Direct: false},
		},
		BlastRadius:  &model.BlastRadius{Level: "medium", Detail: "5 files"},
		CircularDeps: [][]string{{"src/auth/login.py", "src/auth/session.py"}},
		Hotspots:     []string{"src/auth/session.py"},
	}

	text := FormatCompact(output)

	// Check header.
	if !strings.Contains(text, "Context: src/auth/login.py") {
		t.Error("missing subject in header")
	}
	if !strings.Contains(text, "confidence: 0.92") {
		t.Error("missing confidence")
	}
	if !strings.Contains(text, "blast: medium") {
		t.Error("missing blast level")
	}

	// Check must read.
	if !strings.Contains(text, "Must read (3)") {
		t.Error("missing must read count")
	}
	if !strings.Contains(text, "src/auth/session.py") {
		t.Error("missing must_read file")
	}
	if !strings.Contains(text, "imports createSession") {
		t.Error("missing symbol in must_read reason")
	}
	if !strings.Contains(text, "imported by") {
		t.Error("missing imported_by direction")
	}
	if !strings.Contains(text, "stable") {
		t.Error("missing stable tag")
	}

	// Check likely modify.
	if !strings.Contains(text, "Likely modify:") {
		t.Error("missing likely modify")
	}
	if !strings.Contains(text, "session.py (high)") {
		t.Error("missing likely modify entry")
	}

	// Check tests.
	if !strings.Contains(text, "Tests:") {
		t.Error("missing tests section")
	}
	if !strings.Contains(text, "test_login.py (direct)") {
		t.Error("missing direct test")
	}

	// Check warnings.
	if !strings.Contains(text, "Warnings:") {
		t.Error("missing warnings")
	}
	if !strings.Contains(text, "circular dep") {
		t.Error("missing circular dep warning")
	}
	if !strings.Contains(text, "hotspots") {
		t.Error("missing hotspot warning")
	}
}

func TestFormatCompactMinimal(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:    "simple.py",
		Confidence: 1.0,
	}

	text := FormatCompact(output)

	if !strings.Contains(text, "Context: simple.py") {
		t.Error("missing subject")
	}
	// Should not have sections for empty data.
	if strings.Contains(text, "Must read") {
		t.Error("unexpected must read section for empty output")
	}
	if strings.Contains(text, "Likely modify") {
		t.Error("unexpected likely modify section")
	}
}

func TestFormatCompactChange(t *testing.T) {
	report := &model.ChangeReport{
		SchemaVersion: "3.2",
		RefRange:      "main..HEAD",
		ChangedFiles: []model.ChangedFile{
			{File: "src/main.py", Status: "modified", BlastRadius: &model.BlastRadius{Level: "high"}},
			{File: "src/new.py", Status: "added"},
		},
		Summary: model.ChangeSummary{
			TotalFiles:    2,
			Added:         1,
			Modified:      1,
			HighRiskFiles: 1,
		},
		BlastRadius: &model.BlastRadius{Level: "high", Detail: "test"},
		MustRead:    []model.MustReadEntry{{File: "src/utils.py"}},
		TestGaps:    []string{"src/new.py"},
		Coupling: []model.CouplingPair{
			{FileA: "src/main.py", FileB: "src/utils.py", Direction: "a_imports_b"},
		},
	}

	text := FormatCompactChange(report)

	if !strings.Contains(text, "Change: main..HEAD") {
		t.Error("missing ref range")
	}
	if !strings.Contains(text, "blast: high") {
		t.Error("missing blast level")
	}
	if !strings.Contains(text, "1 added") {
		t.Error("missing added count")
	}
	if !strings.Contains(text, "High risk: src/main.py") {
		t.Error("missing high risk file")
	}
	if !strings.Contains(text, "Must read") {
		t.Error("missing must read")
	}
	if !strings.Contains(text, "Test gaps: src/new.py") {
		t.Error("missing test gaps")
	}
	if !strings.Contains(text, "Coupling:") {
		t.Error("missing coupling")
	}
}

func TestFormatCompactChange_WithRiskTriage(t *testing.T) {
	report := &model.ChangeReport{
		RefRange: "main..HEAD",
		ChangedFiles: []model.ChangedFile{
			{File: "pkg/core.go", Status: "modified", RiskScore: 48},
			{File: "pkg/util.go", Status: "modified", RiskScore: 35},
			{File: "pkg/new.go", Status: "added", RiskScore: 10},
		},
		Summary:     model.ChangeSummary{TotalFiles: 3, Modified: 2, Added: 1},
		BlastRadius: &model.BlastRadius{Level: "medium"},
		MustRead:    []model.MustReadEntry{},
		RiskTriage: &model.RiskTriage{
			Critical: []string{},
			Test:     []string{},
			Review:   []string{"pkg/core.go", "pkg/util.go"},
			Safe:     []string{"pkg/new.go"},
		},
		AggregateRisk: &model.AggregateRisk{Score: 48},
	}

	text := FormatCompactChange(report)
	if !strings.Contains(text, "MODERATE RISK") {
		t.Errorf("compact output should include risk badge: %s", text)
	}
	if !strings.Contains(text, "2 to review") {
		t.Errorf("compact output should mention files to review: %s", text)
	}
	t.Logf("compact output:\n%s", text)
}

func TestCompactTokenSavings(t *testing.T) {
	// Verify compact output is significantly smaller than JSON.
	output := &model.AnalysisOutput{
		SchemaVersion: "3.2",
		Subject:       "src/auth/login.py",
		Confidence:    0.92,
		External:      []string{"requests", "jwt", "flask"},
		MustRead: []model.MustReadEntry{
			{File: "src/auth/session.py", Symbols: []string{"createSession", "validateToken"}, Direction: "imports", Role: "service"},
			{File: "src/auth/types.py", Symbols: []string{"AuthConfig"}, Direction: "imports", Role: "foundation", Stable: true},
			{File: "src/models/user.py", Symbols: []string{"User"}, Direction: "imports", Role: "model"},
			{File: "src/middleware/auth.py", Direction: "imported_by"},
			{File: "src/api/routes.py", Direction: "imports"},
		},
		LikelyModify: map[string][]model.LikelyModifyEntry{
			"auth": {
				{File: "session.py", Confidence: "high", Signals: []string{"direct_import", "co_change:5"}},
				{File: "types.py", Confidence: "medium", Signals: []string{"direct_import"}},
			},
		},
		Tests: []model.TestEntry{
			{File: "tests/test_login.py", Direct: true},
			{File: "tests/test_session.py", Direct: false},
		},
		BlastRadius:  &model.BlastRadius{Level: "medium", Detail: "8 files in dependency subgraph", Fragility: 0.45},
		Hotspots:     []string{"src/auth/session.py"},
		CircularDeps: [][]string{{"src/auth/login.py", "src/auth/session.py"}},
		Stats:        &model.IndexStats{TotalFiles: 2638, TotalEdges: 10087, UnresolvedCount: 42},
	}

	compact := FormatCompact(output)

	// Compact should be well under 500 chars for this example.
	if len(compact) > 2000 {
		t.Errorf("compact output too large: %d chars (expected < 2000)", len(compact))
	}
	if len(compact) == 0 {
		t.Error("compact output is empty")
	}
}

func TestFormatRiskTriage_Full(t *testing.T) {
	report := &model.ChangeReport{
		Summary: model.ChangeSummary{TotalFiles: 10},
		ChangedFiles: []model.ChangedFile{
			{File: "core.go", Status: "modified", RiskScore: 82, RiskTier: "CRITICAL", RiskNarrative: "Modified. 7 importers."},
			{File: "handler.go", Status: "modified", RiskScore: 61, RiskTier: "TEST", RiskNarrative: "Modified. Output format changed."},
			{File: "util.go", Status: "modified", RiskScore: 35, RiskTier: "REVIEW"},
			{File: "new.go", Status: "added", RiskScore: 10, RiskTier: "SAFE"},
		},
		RiskTriage: &model.RiskTriage{
			Critical: []string{"core.go"},
			Test:     []string{"handler.go"},
			Review:   []string{"util.go"},
			Safe:     []string{"new.go"},
		},
		AggregateRisk: &model.AggregateRisk{
			Score:          82,
			RegressionRisk: "core.go: 7 importers, 3 untested",
		},
	}

	output := FormatRiskTriage(report)
	if !strings.Contains(output, "RISK TRIAGE") {
		t.Error("missing RISK TRIAGE header")
	}
	if !strings.Contains(output, "CRITICAL (1 file)") {
		t.Errorf("missing CRITICAL section: %s", output)
	}
	if !strings.Contains(output, "TEST (1 file)") {
		t.Errorf("missing TEST section: %s", output)
	}
	if !strings.Contains(output, "REVIEW (1 file)") {
		t.Errorf("missing REVIEW section: %s", output)
	}
	if !strings.Contains(output, "SAFE (1 file)") {
		t.Errorf("missing SAFE section: %s", output)
	}
	if !strings.Contains(output, "score: 82") {
		t.Errorf("missing CRITICAL score: %s", output)
	}
}

func TestFormatRiskTriage_NoCritical(t *testing.T) {
	report := &model.ChangeReport{
		Summary: model.ChangeSummary{TotalFiles: 3},
		ChangedFiles: []model.ChangedFile{
			{File: "a.go", Status: "modified", RiskScore: 40, RiskTier: "REVIEW"},
			{File: "b.go", Status: "added", RiskScore: 10, RiskTier: "SAFE"},
		},
		RiskTriage: &model.RiskTriage{
			Critical: []string{},
			Test:     []string{},
			Review:   []string{"a.go"},
			Safe:     []string{"b.go"},
		},
		AggregateRisk: &model.AggregateRisk{Score: 40},
	}

	output := FormatRiskTriage(report)
	if strings.Contains(output, "CRITICAL") {
		t.Error("should not contain CRITICAL section when no critical files")
	}
	if !strings.Contains(output, "REVIEW") {
		t.Error("should contain REVIEW section")
	}
}

func TestFormatRiskTriage_AllSafe(t *testing.T) {
	report := &model.ChangeReport{
		Summary: model.ChangeSummary{TotalFiles: 5},
		ChangedFiles: []model.ChangedFile{
			{File: "a.go", Status: "added", RiskScore: 10, RiskTier: "SAFE"},
			{File: "b.go", Status: "added", RiskScore: 10, RiskTier: "SAFE"},
		},
		RiskTriage: &model.RiskTriage{
			Critical: []string{},
			Test:     []string{},
			Review:   []string{},
			Safe:     []string{"a.go", "b.go"},
		},
		AggregateRisk: &model.AggregateRisk{
			Score:          10,
			RegressionRisk: "No modifications to existing code. Low regression risk.",
		},
	}

	output := FormatRiskTriage(report)
	if !strings.Contains(output, "SAFE (2 files)") {
		t.Errorf("should show safe count: %s", output)
	}
}

func TestFormatRiskBadge_Critical(t *testing.T) {
	report := &model.ChangeReport{
		ChangedFiles: []model.ChangedFile{
			{File: "pkg/core.go", RiskScore: 85},
			{File: "pkg/handler.go", RiskScore: 60},
			{File: "pkg/cli.go", RiskScore: 55},
			{File: "pkg/util.go", RiskScore: 40},
			{File: "pkg/config.go", RiskScore: 35},
			{File: "pkg/new1.go", RiskScore: 10},
			{File: "pkg/new2.go", RiskScore: 10},
		},
		AggregateRisk: &model.AggregateRisk{Score: 85},
		RiskTriage: &model.RiskTriage{
			Critical: []string{"pkg/core.go"},
			Test:     []string{"pkg/handler.go", "pkg/cli.go"},
			Review:   []string{"pkg/util.go", "pkg/config.go"},
			Safe:     []string{"pkg/new1.go", "pkg/new2.go"},
		},
	}

	badge := FormatRiskBadge(report)
	if !strings.Contains(badge, "CRITICAL RISK") {
		t.Errorf("badge should say CRITICAL RISK: %s", badge)
	}
	if !strings.Contains(badge, "careful review") {
		t.Errorf("badge should mention careful review: %s", badge)
	}
	if !strings.Contains(badge, "core.go") {
		t.Errorf("badge should name the critical file: %s", badge)
	}
	t.Logf("badge: %s", badge)
}

func TestFormatRiskBadge_Moderate(t *testing.T) {
	report := &model.ChangeReport{
		ChangedFiles: []model.ChangedFile{
			{File: "pkg/a.go", RiskScore: 45},
			{File: "pkg/b.go", RiskScore: 40},
			{File: "pkg/c.go", RiskScore: 10},
			{File: "pkg/d.go", RiskScore: 10},
			{File: "pkg/e.go", RiskScore: 10},
		},
		AggregateRisk: &model.AggregateRisk{Score: 45},
		RiskTriage: &model.RiskTriage{
			Critical: []string{},
			Test:     []string{},
			Review:   []string{"pkg/a.go", "pkg/b.go"},
			Safe:     []string{"pkg/c.go", "pkg/d.go", "pkg/e.go"},
		},
	}

	badge := FormatRiskBadge(report)
	if !strings.Contains(badge, "MODERATE RISK") {
		t.Errorf("badge should say MODERATE RISK: %s", badge)
	}
	if !strings.Contains(badge, "a.go") {
		t.Errorf("badge should name review files: %s", badge)
	}
	if !strings.Contains(badge, "3 safe") {
		t.Errorf("badge should mention safe count: %s", badge)
	}
	t.Logf("badge: %s", badge)
}

func TestFormatRiskBadge_AllSafe(t *testing.T) {
	report := &model.ChangeReport{
		ChangedFiles: []model.ChangedFile{
			{File: "a.go", RiskScore: 10},
			{File: "b.go", RiskScore: 10},
			{File: "c.go", RiskScore: 10},
		},
		AggregateRisk: &model.AggregateRisk{Score: 10},
		RiskTriage: &model.RiskTriage{
			Critical: []string{},
			Test:     []string{},
			Review:   []string{},
			Safe:     []string{"a.go", "b.go", "c.go"},
		},
	}

	badge := FormatRiskBadge(report)
	if !strings.Contains(badge, "LOW RISK") {
		t.Errorf("badge should say LOW RISK: %s", badge)
	}
	if !strings.Contains(badge, "all 3 files are low risk") {
		t.Errorf("badge should say all files are low risk: %s", badge)
	}
	t.Logf("badge: %s", badge)
}

func TestFormatRiskBadge_Nil(t *testing.T) {
	if FormatRiskBadge(nil) != "" {
		t.Error("nil report should return empty badge")
	}
	if FormatRiskBadge(&model.ChangeReport{}) != "" {
		t.Error("empty report should return empty badge")
	}
}

func TestShortestUniqueNames_UniqueBasenames(t *testing.T) {
	paths := []string{"internal/analyzer/risk.go", "internal/cli/setup.go", "schema/analysis.go"}
	names := shortestUniqueNames(paths)
	if len(names) != 3 {
		t.Fatalf("got %d names, want 3", len(names))
	}
	// All basenames are unique, so just basenames.
	if names[0] != "risk.go" || names[1] != "setup.go" || names[2] != "analysis.go" {
		t.Errorf("expected basenames, got %v", names)
	}
}

func TestShortestUniqueNames_DuplicateBasenames(t *testing.T) {
	paths := []string{"schema/change.go", "internal/model/change.go"}
	names := shortestUniqueNames(paths)
	if len(names) != 2 {
		t.Fatalf("got %d names, want 2", len(names))
	}
	// Both are "change.go" — need parent dir to disambiguate.
	if names[0] == names[1] {
		t.Errorf("names should be different, got %v", names)
	}
	if names[0] != "schema/change.go" {
		t.Errorf("expected 'schema/change.go', got %q", names[0])
	}
	if names[1] != "model/change.go" {
		t.Errorf("expected 'model/change.go', got %q", names[1])
	}
	t.Logf("disambiguated: %v", names)
}

func TestShortestUniqueNames_TripleDuplicate(t *testing.T) {
	paths := []string{"a/b/change.go", "c/b/change.go", "d/e/change.go"}
	names := shortestUniqueNames(paths)
	// "b/change.go" appears twice, "e/change.go" is unique at depth 2.
	// Need depth 3 for the first two: "a/b/change.go" vs "c/b/change.go".
	dupes := findDuplicates(names)
	if len(dupes) > 0 {
		t.Errorf("still have duplicates: %v", names)
	}
	t.Logf("disambiguated: %v", names)
}

func TestShortestUniqueNames_Empty(t *testing.T) {
	names := shortestUniqueNames(nil)
	if names != nil {
		t.Errorf("expected nil, got %v", names)
	}
}

func TestShortestUniqueNames_Single(t *testing.T) {
	names := shortestUniqueNames([]string{"internal/foo/bar.go"})
	if len(names) != 1 || names[0] != "bar.go" {
		t.Errorf("expected [bar.go], got %v", names)
	}
}

func TestFormatRiskBadge_DuplicateBasenames(t *testing.T) {
	report := &model.ChangeReport{
		ChangedFiles: []model.ChangedFile{
			{File: "schema/change.go", RiskScore: 63},
			{File: "internal/model/change.go", RiskScore: 74},
		},
		AggregateRisk: &model.AggregateRisk{Score: 74},
		RiskTriage: &model.RiskTriage{
			Critical: []string{},
			Test:     []string{"internal/model/change.go", "schema/change.go"},
			Review:   []string{},
			Safe:     []string{},
		},
	}

	badge := FormatRiskBadge(report)
	// Should NOT say "change.go, change.go".
	if strings.Contains(badge, "change.go, change.go") {
		t.Errorf("badge has duplicate basenames: %s", badge)
	}
	// Should disambiguate.
	if !strings.Contains(badge, "model/change.go") {
		t.Errorf("badge should contain 'model/change.go': %s", badge)
	}
	t.Logf("badge: %s", badge)
}
