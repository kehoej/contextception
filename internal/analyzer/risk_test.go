package analyzer

import (
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestComputeRiskScore_AddedFile(t *testing.T) {
	file := model.ChangedFile{File: "pkg/new.go", Status: "added", Indexed: false}
	score, factors := ComputeRiskScore(file, nil, nil, nil, 10)
	if score < 5 || score > 25 {
		t.Errorf("added file score=%d, want 5-25", score)
	}
	if len(factors) == 0 {
		t.Error("expected at least one risk factor")
	}
	found := false
	for _, f := range factors {
		if strings.Contains(f, "new file") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'new file' factor, got %v", factors)
	}
}

func TestComputeRiskScore_ModifiedWithImporters(t *testing.T) {
	analysis := &model.AnalysisOutput{
		MustRead: []model.MustReadEntry{
			{File: "a.go", Direction: "imported_by"},
			{File: "b.go", Direction: "imported_by"},
			{File: "c.go", Direction: "imported_by"},
		},
		BlastRadius: &model.BlastRadius{Level: "high", Fragility: 0.7},
		Tests:       []model.TestEntry{},
	}
	file := model.ChangedFile{File: "pkg/core.go", Status: "modified", Indexed: true}
	score, factors := ComputeRiskScore(file, analysis, nil, nil, 10)
	// Base 30 + structural risk (importers + fragility) + no-tests multiplier
	if score < 40 {
		t.Errorf("modified file with 3 importers + fragility: score=%d, want >= 40", score)
	}
	t.Logf("score=%d, factors=%v", score, factors)
}

func TestComputeRiskScore_DivByZero_CaPlsCeZero(t *testing.T) {
	analysis := &model.AnalysisOutput{
		BlastRadius: &model.BlastRadius{Level: "low", Fragility: 0.0},
		Tests:       []model.TestEntry{{File: "test.go", Direct: true}},
	}
	file := model.ChangedFile{File: "pkg/leaf.go", Status: "modified", Indexed: true}
	score, _ := ComputeRiskScore(file, analysis, nil, nil, 10)
	// Should not panic and should produce a reasonable score.
	if score < 0 || score > 100 {
		t.Errorf("score=%d, want 0-100", score)
	}
}

func TestComputeRiskScore_ClampAt100(t *testing.T) {
	// Simulate extreme risk: many importers, high fragility, cycles, mutual deps.
	analysis := &model.AnalysisOutput{
		MustRead: []model.MustReadEntry{
			{File: "a.go", Direction: "imported_by"},
			{File: "b.go", Direction: "imported_by"},
			{File: "c.go", Direction: "imported_by"},
			{File: "d.go", Direction: "imported_by"},
			{File: "e.go", Direction: "imported_by"},
			{File: "f.go", Direction: "mutual"},
			{File: "g.go", Direction: "mutual"},
			{File: "h.go", Direction: "mutual"},
		},
		BlastRadius:  &model.BlastRadius{Level: "high", Fragility: 0.95},
		CircularDeps: [][]string{{"a", "b", "a"}, {"c", "d", "c"}, {"e", "f", "e"}},
		Tests:        []model.TestEntry{},
	}
	file := model.ChangedFile{File: "pkg/hub.go", Status: "modified", Indexed: true}
	score, _ := ComputeRiskScore(file, analysis, nil, nil, 5)
	if score != 100 {
		t.Errorf("extreme risk score=%d, want 100 (clamped)", score)
	}
}

func TestComputeRiskScore_NilAnalysis(t *testing.T) {
	file := model.ChangedFile{File: "pkg/unknown.go", Status: "modified", Indexed: true}
	score, factors := ComputeRiskScore(file, nil, nil, nil, 10)
	// Nil analysis: base score only (30 for modified).
	if score != 30 {
		t.Errorf("nil analysis score=%d, want 30", score)
	}
	if len(factors) == 0 {
		t.Error("expected factors even with nil analysis")
	}
}

func TestComputeRiskScore_MaxIndegreeZero(t *testing.T) {
	analysis := &model.AnalysisOutput{
		MustRead: []model.MustReadEntry{
			{File: "a.go", Direction: "imported_by"},
		},
		BlastRadius: &model.BlastRadius{Level: "medium"},
		Tests:       []model.TestEntry{},
	}
	file := model.ChangedFile{File: "pkg/core.go", Status: "modified", Indexed: true}
	// maxIndegree=0 should not panic and normalizedImporters should be 0.
	score, _ := ComputeRiskScore(file, analysis, nil, nil, 0)
	if score < 0 || score > 100 {
		t.Errorf("maxIndegree=0: score=%d, want 0-100", score)
	}
}

func TestComputeRiskScore_DeletedFile(t *testing.T) {
	file := model.ChangedFile{File: "pkg/old.go", Status: "deleted", Indexed: false}
	score, _ := ComputeRiskScore(file, nil, nil, nil, 10)
	if score != 5 {
		t.Errorf("deleted file score=%d, want 5", score)
	}
}

func TestComputeRiskScore_RenamedFile(t *testing.T) {
	file := model.ChangedFile{File: "pkg/renamed.go", Status: "renamed", Indexed: false}
	score, _ := ComputeRiskScore(file, nil, nil, nil, 10)
	if score != 5 {
		t.Errorf("renamed file score=%d, want 5", score)
	}
}

func TestAssignRiskTier(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{0, TierSafe},
		{10, TierSafe},
		{20, TierSafe},
		{21, TierReview},
		{50, TierReview},
		{51, TierTest},
		{75, TierTest},
		{76, TierCritical},
		{100, TierCritical},
	}
	for _, tt := range tests {
		got := AssignRiskTier(tt.score)
		if got != tt.want {
			t.Errorf("AssignRiskTier(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestGenerateNarrative_ModifiedWithSignals(t *testing.T) {
	analysis := &model.AnalysisOutput{
		MustRead: []model.MustReadEntry{
			{File: "a.go", Direction: "imported_by"},
			{File: "b.go", Direction: "imported_by"},
			{File: "c.go", Direction: "imports"},
			{File: "d.go", Direction: "imports"},
			{File: "e.go", Direction: "imports"},
		},
		BlastRadius: &model.BlastRadius{Level: "medium", Fragility: 0.73},
		Tests:       []model.TestEntry{},
	}
	file := model.ChangedFile{File: "pkg/core.go", Status: "modified"}
	narrative := GenerateNarrative(file, analysis, nil)
	if !strings.Contains(narrative, "Modified") {
		t.Errorf("narrative should contain 'Modified': %q", narrative)
	}
	if !strings.Contains(narrative, "2 files import") {
		t.Errorf("narrative should mention importers: %q", narrative)
	}
	if !strings.Contains(narrative, "no direct tests") {
		t.Errorf("narrative should mention test gap: %q", narrative)
	}
	if !strings.Contains(narrative, "fragility") {
		t.Errorf("narrative should mention fragility: %q", narrative)
	}
}

func TestGenerateNarrative_NoSignals(t *testing.T) {
	analysis := &model.AnalysisOutput{
		BlastRadius: &model.BlastRadius{Level: "low"},
		Tests:       []model.TestEntry{{File: "test.go", Direct: true}},
	}
	file := model.ChangedFile{File: "pkg/leaf.go", Status: "modified"}
	narrative := GenerateNarrative(file, analysis, nil)
	if !strings.Contains(narrative, "Modified") {
		t.Errorf("narrative should contain 'Modified': %q", narrative)
	}
	if !strings.Contains(narrative, "has direct tests") {
		t.Errorf("narrative should mention tests: %q", narrative)
	}
}

func TestGenerateNarrative_DeletedFile(t *testing.T) {
	analysis := &model.AnalysisOutput{
		MustRead: []model.MustReadEntry{
			{File: "a.go", Direction: "imported_by"},
			{File: "b.go", Direction: "imported_by"},
		},
	}
	file := model.ChangedFile{File: "pkg/old.go", Status: "deleted"}
	narrative := GenerateNarrative(file, analysis, nil)
	if !strings.Contains(narrative, "Deleted") {
		t.Errorf("narrative should say 'Deleted': %q", narrative)
	}
	if !strings.Contains(narrative, "2 files imported") {
		t.Errorf("narrative should mention importers: %q", narrative)
	}
}

func TestGenerateNarrative_AddedFile(t *testing.T) {
	file := model.ChangedFile{File: "pkg/new.go", Status: "added"}
	narrative := GenerateNarrative(file, nil, nil)
	if !strings.Contains(narrative, "New file") {
		t.Errorf("narrative should say 'New file': %q", narrative)
	}
}

func TestGenerateNarrative_Empty(t *testing.T) {
	file := model.ChangedFile{}
	narrative := GenerateNarrative(file, nil, nil)
	if narrative != "No changes detected." {
		t.Errorf("empty file narrative = %q, want 'No changes detected.'", narrative)
	}
}

func TestGenerateNarrative_Truncation(t *testing.T) {
	// Create analysis that would produce a very long narrative.
	entries := make([]model.MustReadEntry, 50)
	for i := range entries {
		entries[i] = model.MustReadEntry{File: "f.go", Direction: "imported_by"}
	}
	analysis := &model.AnalysisOutput{
		MustRead:    entries,
		BlastRadius: &model.BlastRadius{Level: "high", Fragility: 0.99},
		Tests:       []model.TestEntry{},
	}
	file := model.ChangedFile{File: "pkg/core.go", Status: "modified"}
	narrative := GenerateNarrative(file, analysis, nil)
	if len(narrative) > narrativeMaxLen {
		t.Errorf("narrative length=%d, want <= %d", len(narrative), narrativeMaxLen)
	}
}

func TestGenerateTestSuggestions_UntestedImport(t *testing.T) {
	changedFiles := []model.ChangedFile{
		{File: "pkg/handler.go", Status: "modified", RiskScore: 70, RiskTier: TierTest},
	}
	analyses := map[string]*model.AnalysisOutput{
		"pkg/handler.go": {
			MustRead: []model.MustReadEntry{
				{File: "pkg/db.go", Direction: "imports", Symbols: []string{"Query", "Connect"}},
			},
			Tests: []model.TestEntry{},
		},
	}
	suggestions := GenerateTestSuggestions(changedFiles, analyses)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one test suggestion")
	}
	if suggestions[0].File != "pkg/handler.go" {
		t.Errorf("suggestion file = %q, want pkg/handler.go", suggestions[0].File)
	}
	if !strings.Contains(suggestions[0].SuggestedTest, "Query") {
		t.Errorf("suggestion should reference imported symbols: %q", suggestions[0].SuggestedTest)
	}
}

func TestGenerateTestSuggestions_AllTested(t *testing.T) {
	changedFiles := []model.ChangedFile{
		{File: "pkg/handler.go", Status: "modified", RiskScore: 70, RiskTier: TierTest},
	}
	analyses := map[string]*model.AnalysisOutput{
		"pkg/handler.go": {
			Tests: []model.TestEntry{{File: "pkg/handler_test.go", Direct: true}},
		},
	}
	suggestions := GenerateTestSuggestions(changedFiles, analyses)
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions for tested file, got %d", len(suggestions))
	}
}

func TestGenerateTestSuggestions_NoImports(t *testing.T) {
	changedFiles := []model.ChangedFile{
		{File: "pkg/standalone.go", Status: "modified", RiskScore: 80, RiskTier: TierCritical},
	}
	analyses := map[string]*model.AnalysisOutput{
		"pkg/standalone.go": {
			MustRead: []model.MustReadEntry{}, // no imports
			Tests:    []model.TestEntry{},
		},
	}
	suggestions := GenerateTestSuggestions(changedFiles, analyses)
	// Should generate a generic suggestion.
	if len(suggestions) == 0 {
		t.Fatal("expected a generic test suggestion")
	}
	if !strings.Contains(suggestions[0].Reason, "no direct test") {
		t.Errorf("reason should mention no tests: %q", suggestions[0].Reason)
	}
}

func TestGenerateTestSuggestions_EmptyAnalyses(t *testing.T) {
	changedFiles := []model.ChangedFile{
		{File: "pkg/handler.go", Status: "modified", RiskScore: 80, RiskTier: TierCritical},
	}
	suggestions := GenerateTestSuggestions(changedFiles, map[string]*model.AnalysisOutput{})
	if len(suggestions) != 0 {
		t.Errorf("expected no suggestions with empty analyses, got %d", len(suggestions))
	}
}

func TestGenerateTestSuggestions_SafeTier(t *testing.T) {
	changedFiles := []model.ChangedFile{
		{File: "pkg/util.go", Status: "modified", RiskScore: 15, RiskTier: TierSafe},
	}
	analyses := map[string]*model.AnalysisOutput{
		"pkg/util.go": {Tests: []model.TestEntry{}},
	}
	suggestions := GenerateTestSuggestions(changedFiles, analyses)
	if len(suggestions) != 0 {
		t.Errorf("SAFE tier should not get suggestions, got %d", len(suggestions))
	}
}

func TestComputeRiskScore_NonCodeFile(t *testing.T) {
	file := model.ChangedFile{File: "README.md", Status: "modified", Indexed: false}
	score, factors := ComputeRiskScore(file, nil, nil, nil, 10)
	if score > 20 {
		t.Errorf("non-code unindexed file score=%d, want <= 20 (SAFE tier)", score)
	}
	found := false
	for _, f := range factors {
		if f == "non-code file" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'non-code file' factor, got %v", factors)
	}
}

func TestClampScore(t *testing.T) {
	if clampScore(-5) != 0 {
		t.Error("negative score should clamp to 0")
	}
	if clampScore(150) != 100 {
		t.Error("score > 100 should clamp to 100")
	}
	if clampScore(50) != 50 {
		t.Error("score 50 should stay 50")
	}
}
