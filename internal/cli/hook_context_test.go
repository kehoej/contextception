package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestFormatHookContext_WithEntries(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject: "internal/cli/analyze.go",
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

	result := formatHookContext(output)

	// Check header.
	if !strings.Contains(result, "[contextception] Dependency context for internal/cli/analyze.go") {
		t.Error("missing header")
	}

	// Check must-read entries.
	if !strings.Contains(result, "internal/analyzer/analyzer.go (Analyze, AnalyzeMulti) [imports]") {
		t.Error("missing analyzer entry with symbols and direction")
	}
	if !strings.Contains(result, "internal/db/db.go (OpenIndex) [imports, stable]") {
		t.Error("missing db entry with stable tag")
	}
	if !strings.Contains(result, "internal/model/model.go [imports, circular]") {
		t.Error("missing model entry with circular tag")
	}

	// Check tests (direct only).
	if !strings.Contains(result, "internal/cli/analyze_test.go") {
		t.Error("missing direct test")
	}
	if strings.Contains(result, "internal/analyzer/analyzer_test.go") {
		t.Error("non-direct test should not appear")
	}

	// Check blast radius.
	if !strings.Contains(result, "Blast radius: medium (5 reverse importers)") {
		t.Error("missing blast radius")
	}
}

func TestFormatHookContext_Empty(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject: "empty.go",
	}

	result := formatHookContext(output)
	if !strings.Contains(result, "[contextception] Dependency context for empty.go") {
		t.Error("missing header for empty output")
	}
	if strings.Contains(result, "Must-read") {
		t.Error("should not have must-read section for empty output")
	}
	if strings.Contains(result, "Tests:") {
		t.Error("should not have tests section for empty output")
	}
}

func TestFormatHookContext_Nil(t *testing.T) {
	result := formatHookContext(nil)
	if result != "" {
		t.Error("nil output should produce empty string")
	}
}

func TestEmitHookAllow_JSONStructure(t *testing.T) {
	// Test that the envelope has the correct structure.
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

	// Verify required fields.
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

	// additionalContext should be omitted (not present as empty string).
	if strings.Contains(string(data), "additionalContext") {
		t.Error("empty additionalContext should be omitted from JSON")
	}
}
