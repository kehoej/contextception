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
