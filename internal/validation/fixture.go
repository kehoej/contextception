package validation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kehoej/contextception/internal/model"
)

// Fixture defines expected analysis output for a single subject file.
type Fixture struct {
	Subject           string   `json:"subject"`
	MustRead          []string `json:"must_read"`
	MustReadForbidden []string `json:"must_read_forbidden"`
	MustReadOrdered   []string `json:"must_read_ordered,omitempty"`
	Related           []string `json:"related"`
	RelatedForbidden  []string `json:"related_forbidden,omitempty"`
	Tests             []string `json:"tests"`
	IgnoreMustContain []string `json:"ignore_must_contain"`
	External          []string `json:"external,omitempty"`
	ExpectStable      []string `json:"expect_stable,omitempty"`
}

// ComparisonResult holds the outcome of comparing actual output to a fixture.
type ComparisonResult struct {
	Subject string

	MustReadRecall    float64
	MustReadPrecision float64
	MustReadMissing   []string
	MustReadExtra     []string

	RelatedRecall  float64
	RelatedMissing []string

	TestsRecall  float64
	TestsMissing []string

	ForbiddenViolations        []string // must_read_forbidden items found in actual must_read
	RelatedForbiddenViolations []string // related_forbidden items found in actual related
	OrderingViolations         []string // must_read_ordered pairs where order is wrong
	IgnoreMissing              []string // ignore_must_contain items found in positive sections

	ExternalRecall  float64
	ExternalMissing []string

	StableMissing []string // expect_stable entries not found as stable in must_read

	Pass bool
}

// Thresholds configures pass/fail cutoffs for fixture comparison.
type Thresholds struct {
	MustReadRecall    float64
	MustReadPrecision float64
	RelatedRecall     float64
	TestsRecall       float64
	ExternalRecall    float64
}

// DefaultThresholds returns the targets from the testing strategy.
func DefaultThresholds() Thresholds {
	return Thresholds{
		MustReadRecall:    0.95,
		MustReadPrecision: 0.80,
		RelatedRecall:     0.80,
		TestsRecall:       0.80,
		ExternalRecall:    1.0,
	}
}

// LoadFixture reads a single fixture JSON file.
func LoadFixture(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fixture %s: %w", path, err)
	}
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse fixture %s: %w", path, err)
	}
	return &f, nil
}

// LoadFixturesDir loads all .json fixtures from a directory.
func LoadFixturesDir(dir string) ([]*Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir %s: %w", dir, err)
	}
	var fixtures []*Fixture
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		f, err := LoadFixture(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		fixtures = append(fixtures, f)
	}
	return fixtures, nil
}

// extractMustReadPaths extracts flat file paths from v3 MustReadEntry slice.
func extractMustReadPaths(entries []model.MustReadEntry) []string {
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.File
	}
	return result
}

// flattenRelatedEntries expands grouped RelatedEntry maps into full paths.
func flattenRelatedEntries(m map[string][]model.RelatedEntry) []string {
	var result []string
	for key, entries := range m {
		for _, e := range entries {
			if key == "." {
				result = append(result, e.File)
			} else {
				result = append(result, key+"/"+e.File)
			}
		}
	}
	return result
}

// flattenLikelyModifyEntries expands grouped LikelyModifyEntry maps into full paths.
func flattenLikelyModifyEntries(m map[string][]model.LikelyModifyEntry) []string {
	var result []string
	for key, entries := range m {
		for _, e := range entries {
			if key == "." {
				result = append(result, e.File)
			} else {
				result = append(result, key+"/"+e.File)
			}
		}
	}
	return result
}

// extractTestPaths extracts flat file paths from TestEntry slice.
func extractTestPaths(entries []model.TestEntry) []string {
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.File
	}
	return result
}

// Compare checks actual analysis output against fixture expectations.
func Compare(fixture *Fixture, actual *model.AnalysisOutput, th Thresholds) ComparisonResult {
	r := ComparisonResult{Subject: fixture.Subject}

	actualMustRead := extractMustReadPaths(actual.MustRead)
	actualRelated := flattenRelatedEntries(actual.Related)

	// must_read
	r.MustReadRecall = Recall(fixture.MustRead, actualMustRead)
	r.MustReadPrecision = Precision(fixture.MustRead, actualMustRead)
	r.MustReadMissing = Missing(fixture.MustRead, actualMustRead)
	r.MustReadExtra = Extra(fixture.MustRead, actualMustRead)

	// related — likely_modify is a promotion from related, so files appearing
	// in likely_modify should satisfy related expectations.
	actualLikelyModify := flattenLikelyModifyEntries(actual.LikelyModify)
	actualRelatedOrLM := append(actualRelated, actualLikelyModify...)
	r.RelatedRecall = Recall(fixture.Related, actualRelatedOrLM)
	r.RelatedMissing = Missing(fixture.Related, actualRelatedOrLM)

	// tests
	actualTestPaths := extractTestPaths(actual.Tests)
	if len(fixture.Tests) > 0 {
		r.TestsRecall = Recall(fixture.Tests, actualTestPaths)
		r.TestsMissing = Missing(fixture.Tests, actualTestPaths)
	} else {
		r.TestsRecall = 1.0 // no expectations → pass
	}

	// forbidden check: fail if any forbidden file is in actual must_read
	mustReadSet := toSet(actualMustRead)
	for _, f := range fixture.MustReadForbidden {
		if mustReadSet[f] {
			r.ForbiddenViolations = append(r.ForbiddenViolations, f)
		}
	}

	// related_forbidden check: fail if any forbidden file is in actual related
	relatedSet := toSet(actualRelated)
	for _, f := range fixture.RelatedForbidden {
		if relatedSet[f] {
			r.RelatedForbiddenViolations = append(r.RelatedForbiddenViolations, f)
		}
	}

	// must_read_ordered check: verify that consecutive pairs maintain order
	if len(fixture.MustReadOrdered) > 1 {
		posMap := make(map[string]int, len(actualMustRead))
		for i, p := range actualMustRead {
			posMap[p] = i
		}
		for i := 0; i < len(fixture.MustReadOrdered)-1; i++ {
			a := fixture.MustReadOrdered[i]
			b := fixture.MustReadOrdered[i+1]
			posA, okA := posMap[a]
			posB, okB := posMap[b]
			if okA && okB && posA >= posB {
				r.OrderingViolations = append(r.OrderingViolations,
					fmt.Sprintf("%s (pos %d) should come before %s (pos %d)", a, posA, b, posB))
			}
		}
	}

	// external check
	if len(fixture.External) > 0 {
		r.ExternalRecall = Recall(fixture.External, actual.External)
		r.ExternalMissing = Missing(fixture.External, actual.External)
	} else {
		r.ExternalRecall = 1.0 // no expectations → pass
	}

	// expect_stable check: verify each expected path appears in must_read with Stable=true
	if len(fixture.ExpectStable) > 0 {
		stableSet := make(map[string]bool)
		for _, e := range actual.MustRead {
			if e.Stable {
				stableSet[e.File] = true
			}
		}
		for _, path := range fixture.ExpectStable {
			if !stableSet[path] {
				r.StableMissing = append(r.StableMissing, path)
			}
		}
	}

	// ignore check: IgnoreMustContain fixtures are satisfied if the file
	// is NOT in must_read, related, or likely_modify. The tests section is
	// excluded because test files appearing there is expected behavior.
	allPositive := toSet(actualMustRead)
	for _, p := range actualRelated {
		allPositive[p] = true
	}
	likelyModPaths := flattenLikelyModifyEntries(actual.LikelyModify)
	for _, p := range likelyModPaths {
		allPositive[p] = true
	}
	for _, ig := range fixture.IgnoreMustContain {
		if allPositive[ig] {
			r.IgnoreMissing = append(r.IgnoreMissing, ig)
		}
	}

	// pass/fail
	r.Pass = r.MustReadRecall >= th.MustReadRecall &&
		r.MustReadPrecision >= th.MustReadPrecision &&
		r.RelatedRecall >= th.RelatedRecall &&
		r.TestsRecall >= th.TestsRecall &&
		r.ExternalRecall >= th.ExternalRecall &&
		len(r.ForbiddenViolations) == 0 &&
		len(r.RelatedForbiddenViolations) == 0 &&
		len(r.OrderingViolations) == 0 &&
		len(r.IgnoreMissing) == 0 &&
		len(r.StableMissing) == 0

	return r
}

// FormatResult returns a human-readable summary of a comparison result.
func FormatResult(r ComparisonResult) string {
	var b strings.Builder
	status := "PASS"
	if !r.Pass {
		status = "FAIL"
	}
	fmt.Fprintf(&b, "[%s] %s\n", status, r.Subject)
	fmt.Fprintf(&b, "  must_read  recall=%.2f precision=%.2f\n", r.MustReadRecall, r.MustReadPrecision)
	if len(r.MustReadMissing) > 0 {
		fmt.Fprintf(&b, "    missing: %v\n", r.MustReadMissing)
	}
	if len(r.MustReadExtra) > 0 {
		fmt.Fprintf(&b, "    extra:   %v\n", r.MustReadExtra)
	}
	fmt.Fprintf(&b, "  related    recall=%.2f\n", r.RelatedRecall)
	if len(r.RelatedMissing) > 0 {
		fmt.Fprintf(&b, "    missing: %v\n", r.RelatedMissing)
	}
	if len(r.TestsMissing) > 0 {
		fmt.Fprintf(&b, "  tests      recall=%.2f\n", r.TestsRecall)
		fmt.Fprintf(&b, "    missing: %v\n", r.TestsMissing)
	}
	if len(r.ForbiddenViolations) > 0 {
		fmt.Fprintf(&b, "  FORBIDDEN in must_read: %v\n", r.ForbiddenViolations)
	}
	if len(r.RelatedForbiddenViolations) > 0 {
		fmt.Fprintf(&b, "  FORBIDDEN in related: %v\n", r.RelatedForbiddenViolations)
	}
	if len(r.OrderingViolations) > 0 {
		fmt.Fprintf(&b, "  ORDERING violations: %v\n", r.OrderingViolations)
	}
	if len(r.IgnoreMissing) > 0 {
		fmt.Fprintf(&b, "  FOUND IN POSITIVE (should be ignored): %v\n", r.IgnoreMissing)
	}
	if len(r.ExternalMissing) > 0 {
		fmt.Fprintf(&b, "  external  recall=%.2f\n", r.ExternalRecall)
		fmt.Fprintf(&b, "    missing: %v\n", r.ExternalMissing)
	}
	if len(r.StableMissing) > 0 {
		fmt.Fprintf(&b, "  STABLE MISSING (expected stable=true in must_read): %v\n", r.StableMissing)
	}
	return b.String()
}
