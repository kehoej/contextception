package schema

import (
	"encoding/json"
	"testing"
)

func TestRiskTriage_JSONRoundTrip(t *testing.T) {
	original := &RiskTriage{
		Critical: []string{"core.go"},
		Test:     []string{"handler.go"},
		Review:   []string{"util.go", "config.go"},
		Safe:     []string{"readme.md"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded RiskTriage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Critical) != 1 || decoded.Critical[0] != "core.go" {
		t.Errorf("critical: got %v, want [core.go]", decoded.Critical)
	}
	if len(decoded.Test) != 1 || decoded.Test[0] != "handler.go" {
		t.Errorf("test: got %v, want [handler.go]", decoded.Test)
	}
	if len(decoded.Review) != 2 {
		t.Errorf("review: got %d items, want 2", len(decoded.Review))
	}
	if len(decoded.Safe) != 1 {
		t.Errorf("safe: got %d items, want 1", len(decoded.Safe))
	}

	// Verify JSON field names are correct.
	var raw map[string]any
	json.Unmarshal(data, &raw)
	for _, key := range []string{"critical", "test", "review", "safe"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestAggregateRisk_JSONRoundTrip(t *testing.T) {
	original := &AggregateRisk{
		Score:             72,
		Percentile:        85,
		RegressionRisk:    "history.go: 10 importers, 3 untested",
		TestCoverageRatio: 0.57,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AggregateRisk
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Score != 72 {
		t.Errorf("score: got %d, want 72", decoded.Score)
	}
	if decoded.Percentile != 85 {
		t.Errorf("percentile: got %d, want 85", decoded.Percentile)
	}
	if decoded.RegressionRisk != original.RegressionRisk {
		t.Errorf("regression_risk: got %q, want %q", decoded.RegressionRisk, original.RegressionRisk)
	}
	if decoded.TestCoverageRatio != 0.57 {
		t.Errorf("test_coverage_ratio: got %f, want 0.57", decoded.TestCoverageRatio)
	}
}

func TestAggregateRisk_OmitemptyPercentile(t *testing.T) {
	// Percentile=0 should be omitted from JSON.
	original := &AggregateRisk{Score: 30, TestCoverageRatio: 0.5}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["percentile"]; ok {
		t.Error("percentile=0 should be omitted via omitempty")
	}
	// regression_risk="" should also be omitted.
	if _, ok := raw["regression_risk"]; ok {
		t.Error("empty regression_risk should be omitted via omitempty")
	}
	// score and test_coverage_ratio should always be present.
	if _, ok := raw["score"]; !ok {
		t.Error("score should always be present")
	}
	if _, ok := raw["test_coverage_ratio"]; !ok {
		t.Error("test_coverage_ratio should always be present")
	}
}

func TestTestSuggestion_JSONRoundTrip(t *testing.T) {
	original := &TestSuggestion{
		File:          "pkg/handler.go",
		SuggestedTest: "Test handler.go's use of Query from db.go",
		Reason:        "handler.go imports Query but has no direct test",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded TestSuggestion
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.File != original.File {
		t.Errorf("file: got %q, want %q", decoded.File, original.File)
	}
	if decoded.SuggestedTest != original.SuggestedTest {
		t.Errorf("suggested_test: got %q, want %q", decoded.SuggestedTest, original.SuggestedTest)
	}
	if decoded.Reason != original.Reason {
		t.Errorf("reason: got %q, want %q", decoded.Reason, original.Reason)
	}

	// Verify JSON field names.
	var raw map[string]any
	json.Unmarshal(data, &raw)
	for _, key := range []string{"file", "suggested_test", "reason"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestChangedFile_RiskFieldsOmitempty(t *testing.T) {
	// A changed file with no risk data should omit risk fields.
	cf := ChangedFile{File: "readme.md", Status: "modified", Indexed: false}
	data, err := json.Marshal(cf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	for _, key := range []string{"risk_score", "risk_tier", "risk_factors", "risk_narrative"} {
		if _, ok := raw[key]; ok {
			t.Errorf("zero-value %q should be omitted via omitempty", key)
		}
	}

	// A changed file with risk data should include them.
	cf2 := ChangedFile{
		File: "core.go", Status: "modified", Indexed: true,
		RiskScore: 72, RiskTier: "TEST",
		RiskFactors:   []string{"modified", "5 importers"},
		RiskNarrative: "Modified. 5 importers.",
	}
	data2, _ := json.Marshal(cf2)
	var raw2 map[string]any
	json.Unmarshal(data2, &raw2)
	for _, key := range []string{"risk_score", "risk_tier", "risk_factors", "risk_narrative"} {
		if _, ok := raw2[key]; !ok {
			t.Errorf("non-zero %q should be present", key)
		}
	}
}

func TestChangeReport_RiskTriageOmitempty(t *testing.T) {
	// Report with nil risk_triage should omit the field.
	report := ChangeReport{SchemaVersion: "1.0", RefRange: "a..b"}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)
	if _, ok := raw["risk_triage"]; ok {
		t.Error("nil risk_triage should be omitted")
	}
	if _, ok := raw["aggregate_risk"]; ok {
		t.Error("nil aggregate_risk should be omitted")
	}
	if _, ok := raw["test_suggestions"]; ok {
		t.Error("nil test_suggestions should be omitted")
	}
}
