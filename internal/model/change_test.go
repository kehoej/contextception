package model

import (
	"testing"

	"github.com/kehoej/contextception/schema"
)

// TestTypeAliases verifies that model type aliases resolve to the correct schema types.
func TestTypeAliases(t *testing.T) {
	// Verify new risk-related aliases are assignable from schema types.
	// The explicit type on the left is intentional — it's a compile-time check
	// that the alias resolves correctly. If the type doesn't match, this won't build.
	var rt RiskTriage = schema.RiskTriage{Critical: []string{"a.go"}} //nolint:staticcheck
	if len(rt.Critical) != 1 {
		t.Errorf("RiskTriage.Critical: got %d, want 1", len(rt.Critical))
	}

	var ar AggregateRisk = schema.AggregateRisk{Score: 80} //nolint:staticcheck
	if ar.Score != 80 {
		t.Errorf("AggregateRisk.Score: got %d, want 80", ar.Score)
	}

	var ts TestSuggestion = schema.TestSuggestion{File: "b.go"} //nolint:staticcheck
	if ts.File != "b.go" {
		t.Errorf("TestSuggestion.File: got %q, want b.go", ts.File)
	}

	// Verify ChangedFile risk fields are accessible via model alias.
	cf := ChangedFile{
		RiskScore: 42,
		RiskTier:  "REVIEW",
	}
	if cf.RiskScore != 42 {
		t.Errorf("RiskScore: got %d, want 42", cf.RiskScore)
	}
	if cf.RiskTier != "REVIEW" {
		t.Errorf("RiskTier: got %q, want REVIEW", cf.RiskTier)
	}

	// Verify ChangeReport risk fields are accessible.
	report := ChangeReport{
		RiskTriage:    &RiskTriage{Critical: []string{"a.go"}},
		AggregateRisk: &AggregateRisk{Score: 80},
		TestSuggestions: []TestSuggestion{
			{File: "b.go", SuggestedTest: "test", Reason: "reason"},
		},
	}
	if len(report.RiskTriage.Critical) != 1 {
		t.Errorf("RiskTriage.Critical: got %d items, want 1", len(report.RiskTriage.Critical))
	}
	if report.AggregateRisk.Score != 80 {
		t.Errorf("AggregateRisk.Score: got %d, want 80", report.AggregateRisk.Score)
	}
	if len(report.TestSuggestions) != 1 {
		t.Errorf("TestSuggestions: got %d items, want 1", len(report.TestSuggestions))
	}
}
