package grader

import (
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestGradeFile_HighQualityOutput(t *testing.T) {
	out := &model.AnalysisOutput{
		SchemaVersion: "3.2",
		Subject:       "pkg/service.go",
		Confidence:    0.98,
		MustRead: []model.MustReadEntry{
			{File: "pkg/types.go", Symbols: []string{"Config"}, Direction: "imports", Role: "foundation"},
			{File: "pkg/db.go", Symbols: []string{"Store"}, Direction: "imports"},
			{File: "pkg/handler.go", Direction: "imported_by", Role: "orchestrator"},
		},
		LikelyModify: map[string][]model.LikelyModifyEntry{
			"pkg": {
				{File: "pkg/handler.go", Confidence: "high", Signals: []string{"reverse_import:1"}},
				{File: "pkg/api.go", Confidence: "medium", Signals: []string{"co_change:3"}},
			},
		},
		Tests: []model.TestEntry{
			{File: "pkg/service_test.go", Direct: true},
			{File: "pkg/handler_test.go", Direct: false},
		},
		Related: map[string][]model.RelatedEntry{
			"pkg": {
				{File: "pkg/util.go", Signals: []string{"distance:2", "structural_proximity"}},
			},
		},
		BlastRadius: &model.BlastRadius{Level: "medium", Detail: "5 direct importers", Fragility: 0.3},
		Stats:       &model.IndexStats{TotalFiles: 100, TotalEdges: 200, UnresolvedCount: 5},
	}

	fg := GradeFile(out, "Service/Controller")

	if fg.Overall < 3.0 {
		t.Errorf("expected high-quality output to get overall >= 3.0, got %.2f", fg.Overall)
	}
	if fg.MustRead < 3.0 {
		t.Errorf("expected must_read >= 3.0, got %.2f", fg.MustRead)
	}
	if fg.LikelyModify < 3.0 {
		t.Errorf("expected likely_modify >= 3.0, got %.2f", fg.LikelyModify)
	}
	if fg.Tests < 3.0 {
		t.Errorf("expected tests >= 3.0, got %.2f", fg.Tests)
	}
	if fg.BlastRadius < 3.0 {
		t.Errorf("expected blast_radius >= 3.0, got %.2f", fg.BlastRadius)
	}
}

func TestGradeFile_LowQualityOutput(t *testing.T) {
	out := &model.AnalysisOutput{
		SchemaVersion: "3.2",
		Subject:       "pkg/broken.go",
		Confidence:    0.3,
		MustRead:      []model.MustReadEntry{},
		LikelyModify:  map[string][]model.LikelyModifyEntry{},
		Tests:         []model.TestEntry{},
		Related:       map[string][]model.RelatedEntry{},
		BlastRadius:   nil,
		Stats:         &model.IndexStats{TotalFiles: 100, TotalEdges: 200, UnresolvedCount: 50},
	}

	fg := GradeFile(out, "Service/Controller")

	if fg.Overall > 3.0 {
		t.Errorf("expected low-quality output to get overall <= 3.0, got %.2f", fg.Overall)
	}
	if fg.BlastRadius > 1.0 {
		t.Errorf("expected nil blast_radius to get 1.0, got %.2f", fg.BlastRadius)
	}
}

func TestComputeOverall_Weights(t *testing.T) {
	fg := FileGrade{
		MustRead:     4.0,
		LikelyModify: 4.0,
		Tests:        4.0,
		Related:      4.0,
		BlastRadius:  4.0,
	}
	fg.ComputeOverall()

	if diff := fg.Overall - 4.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("all 4.0 should produce ~4.0 overall, got %.4f", fg.Overall)
	}

	fg2 := FileGrade{
		MustRead:     1.0,
		LikelyModify: 1.0,
		Tests:        1.0,
		Related:      1.0,
		BlastRadius:  1.0,
	}
	fg2.ComputeOverall()
	if diff := fg2.Overall - 1.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("all 1.0 should produce ~1.0 overall, got %.4f", fg2.Overall)
	}
}

func TestLetterGradeFromScore(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{4.0, "A"},
		{3.5, "A"},
		{3.49, "B"},
		{2.5, "B"},
		{2.49, "C"},
		{1.5, "C"},
		{1.49, "D"},
		{1.0, "D"},
	}
	for _, tt := range tests {
		got := LetterGradeFromScore(tt.score)
		if got != tt.want {
			t.Errorf("LetterGradeFromScore(%.2f) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestComputeSections(t *testing.T) {
	files := []FileGrade{
		{MustRead: 4.0, LikelyModify: 3.0, Tests: 2.0, Related: 3.0, BlastRadius: 4.0},
		{MustRead: 2.0, LikelyModify: 3.0, Tests: 4.0, Related: 3.0, BlastRadius: 2.0},
	}
	sg := ComputeSections(files)

	if sg.MustRead != 3.0 {
		t.Errorf("expected must_read average 3.0, got %.2f", sg.MustRead)
	}
	if sg.LikelyModify != 3.0 {
		t.Errorf("expected likely_modify average 3.0, got %.2f", sg.LikelyModify)
	}
	if sg.Tests != 3.0 {
		t.Errorf("expected tests average 3.0, got %.2f", sg.Tests)
	}
}

func TestDetectIssues(t *testing.T) {
	out := &model.AnalysisOutput{
		SchemaVersion: "2.0",
		Subject:       "test.go",
		Confidence:    0.3,
		BlastRadius:   nil,
		Stats:         nil,
	}

	issues := DetectIssues(out)

	if len(issues) < 3 {
		t.Errorf("expected at least 3 issues, got %d", len(issues))
	}

	hasSchema := false
	hasConfidence := false
	hasBlastRadius := false
	for _, i := range issues {
		switch i.Section {
		case "schema":
			hasSchema = true
		case "confidence":
			hasConfidence = true
		case "blast_radius":
			hasBlastRadius = true
		}
	}
	if !hasSchema {
		t.Error("expected schema version issue")
	}
	if !hasConfidence {
		t.Error("expected confidence issue")
	}
	if !hasBlastRadius {
		t.Error("expected blast_radius issue")
	}
}

func TestGradeTests_SubjectIsTestFile(t *testing.T) {
	out := &model.AnalysisOutput{
		Subject: "Lib/idlelib/idle_test/test_autocomplete.py",
		Tests:   []model.TestEntry{},
	}
	var fg FileGrade
	score := gradeTests(out, &fg)
	if score != 4.0 {
		t.Errorf("test file subject with empty tests: got %.2f, want 4.0", score)
	}
	if len(fg.Notes) == 0 || fg.Notes[0] != "tests: subject is a test file (tests section N/A)" {
		t.Errorf("unexpected note: %v", fg.Notes)
	}
}

func TestGradeTests_ZeroCandidates(t *testing.T) {
	out := &model.AnalysisOutput{
		Subject:   "Lib/asyncio/constants.py",
		Tests:     []model.TestEntry{},
		TestsNote: "no test files found in nearby directories",
	}
	var fg FileGrade
	score := gradeTests(out, &fg)
	if score != 3.0 {
		t.Errorf("zero candidates with empty tests: got %.2f, want 3.0", score)
	}
	if len(fg.Notes) == 0 || fg.Notes[0] != "tests: no test files found (likely untested)" {
		t.Errorf("unexpected note: %v", fg.Notes)
	}
}

func TestGradeTests_CandidatesExistNoMatch(t *testing.T) {
	out := &model.AnalysisOutput{
		Subject:   "Lib/test/libregrtest/filter.py",
		Tests:     []model.TestEntry{},
		TestsNote: "3 test files nearby but none directly test this file",
	}
	var fg FileGrade
	score := gradeTests(out, &fg)
	if score != 2.0 {
		t.Errorf("candidates exist but no match: got %.2f, want 2.0", score)
	}
	if len(fg.Notes) == 0 || fg.Notes[0] != "tests: no test files matched (candidates existed)" {
		t.Errorf("unexpected note: %v", fg.Notes)
	}
}

func TestGradeTests_NonEmpty(t *testing.T) {
	out := &model.AnalysisOutput{
		Subject: "pkg/service.go",
		Tests: []model.TestEntry{
			{File: "pkg/service_test.go", Direct: true},
			{File: "pkg/handler_test.go", Direct: false},
		},
	}
	var fg FileGrade
	score := gradeTests(out, &fg)
	if score < 3.0 {
		t.Errorf("non-empty tests with good coverage: got %.2f, want >= 3.0", score)
	}
}

func TestClamp(t *testing.T) {
	if v := clamp(5.0, 1.0, 4.0); v != 4.0 {
		t.Errorf("clamp(5, 1, 4) = %.1f, want 4.0", v)
	}
	if v := clamp(0.5, 1.0, 4.0); v != 1.0 {
		t.Errorf("clamp(0.5, 1, 4) = %.1f, want 1.0", v)
	}
	if v := clamp(2.5, 1.0, 4.0); v != 2.5 {
		t.Errorf("clamp(2.5, 1, 4) = %.1f, want 2.5", v)
	}
}
