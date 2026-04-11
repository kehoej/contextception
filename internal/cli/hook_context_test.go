package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/model"
)

func TestHookContextCompactFormat(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject: "internal/cli/analyze.go",
		Confidence: 0.95,
		MustRead: []model.MustReadEntry{
			{File: "internal/analyzer/analyzer.go", Symbols: []string{"Analyze", "AnalyzeMulti"}, Direction: "imports"},
			{File: "internal/db/db.go", Symbols: []string{"OpenIndex"}, Direction: "imports", Stable: true},
			{File: "internal/model/model.go", Direction: "imports", Circular: true},
		},
		Tests: []model.TestEntry{
			{File: "internal/cli/analyze_test.go", Direct: true},
			{File: "internal/analyzer/analyzer_test.go", Direct: false},
		},
		BlastRadius: &model.BlastRadius{
			Level:  "medium",
			Detail: "5 reverse importers",
		},
	}

	// This mirrors how hook_context.go formats the output.
	result := "[contextception] " + analyzer.FormatCompact(output)

	// Check header.
	if !strings.Contains(result, "[contextception]") {
		t.Error("missing contextception prefix")
	}
	if !strings.Contains(result, "internal/cli/analyze.go") {
		t.Error("missing subject")
	}

	// Check must-read entries are present.
	if !strings.Contains(result, "internal/analyzer/analyzer.go") {
		t.Error("missing analyzer entry")
	}
	if !strings.Contains(result, "internal/db/db.go") {
		t.Error("missing db entry")
	}

	// Check tests.
	if !strings.Contains(result, "internal/cli/analyze_test.go") {
		t.Error("missing direct test")
	}

	// Check blast radius.
	if !strings.Contains(result, "blast: medium") {
		t.Error("missing blast radius")
	}
}

func TestHookContextCompactFormat_Empty(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:    "empty.go",
		Confidence: 1.0,
	}

	result := "[contextception] " + analyzer.FormatCompact(output)
	if !strings.Contains(result, "empty.go") {
		t.Error("missing subject for empty output")
	}
	// Should not have sections for empty data.
	if strings.Contains(result, "Must read") {
		t.Error("should not have must-read section for empty output")
	}
}

func TestEmitHookAllow_JSONStructure(t *testing.T) {
	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			AdditionalContext:  "test context",
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	hso := parsed["hookSpecificOutput"]
	if hso["hookEventName"] != "PreToolUse" {
		t.Errorf("hookEventName = %q, want PreToolUse", hso["hookEventName"])
	}
	if hso["permissionDecision"] != "allow" {
		t.Errorf("permissionDecision = %q, want allow", hso["permissionDecision"])
	}
	if hso["additionalContext"] != "test context" {
		t.Errorf("additionalContext = %q, want 'test context'", hso["additionalContext"])
	}
}

func TestEmitHookAllow_OmitsEmptyContext(t *testing.T) {
	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), "additionalContext") {
		t.Error("empty additionalContext should be omitted from JSON")
	}
}
