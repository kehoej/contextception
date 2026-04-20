package grader

import (
	"fmt"
	"strings"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/model"
)

// GradeFile applies the testing-tracker rubric to a single AnalysisOutput.
// The archetype name is for labeling only and does not affect scoring.
func GradeFile(output *model.AnalysisOutput, archetype string) FileGrade {
	fg := FileGrade{
		File:      output.Subject,
		Archetype: archetype,
	}

	fg.MustRead = gradeMustRead(output, &fg)
	fg.LikelyModify = gradeLikelyModify(output, &fg)
	fg.Tests = gradeTests(output, &fg)
	fg.Related = gradeRelated(output, &fg)
	fg.BlastRadius = gradeBlastRadius(output, &fg)
	fg.ComputeOverall()

	return fg
}

// gradeMustRead grades the must_read section (weight 0.40).
// Criteria: confidence, symbol coverage, direction coverage, topo ordering, no spurious.
func gradeMustRead(out *model.AnalysisOutput, fg *FileGrade) float64 {
	score := 4.0
	deductions := 0

	// Confidence check: low confidence suggests missing imports.
	if out.Confidence < 0.8 {
		score -= 1.0
		fg.Notes = append(fg.Notes, fmt.Sprintf("must_read: low confidence (%.2f) suggests missing imports", out.Confidence))
		deductions++
	} else if out.Confidence < 0.95 {
		score -= 0.5
		fg.Notes = append(fg.Notes, fmt.Sprintf("must_read: moderate confidence (%.2f)", out.Confidence))
	}

	// Symbol coverage: check what fraction of entries have symbols.
	// Entries with direction "same_package" are excluded from the symbol rate
	// because same-package files have no import edges to derive symbols from.
	// C# files (.cs) are also excluded because C# uses namespace-level imports
	// (using directives), so individual type symbols can't be derived from the
	// import statement — the resolver maps namespaces to representative files.
	isCSharp := strings.HasSuffix(out.Subject, ".cs")
	if len(out.MustRead) > 0 {
		withSymbols := 0
		eligibleForSymbols := 0
		withDirection := 0
		for _, e := range out.MustRead {
			if e.Direction != "" && e.Direction != "same_package" && !isCSharp {
				eligibleForSymbols++
				if len(e.Symbols) > 0 {
					withSymbols++
				}
			}
			if e.Direction != "" {
				withDirection++
			}
		}
		directionRate := float64(withDirection) / float64(len(out.MustRead))

		if eligibleForSymbols > 0 {
			symbolRate := float64(withSymbols) / float64(eligibleForSymbols)
			if symbolRate < 0.3 {
				score -= 0.5
				fg.Notes = append(fg.Notes, fmt.Sprintf("must_read: low symbol coverage (%.0f%%)", symbolRate*100))
				deductions++
			}
		}
		if directionRate < 0.5 {
			score -= 0.5
			fg.Notes = append(fg.Notes, fmt.Sprintf("must_read: low direction coverage (%.0f%%)", directionRate*100))
			deductions++
		}
	}
	// Note: empty must_read is only fine for leaf files with no imports.
	// We don't penalize heavily because leaf files legitimately have empty must_read.

	// Topo ordering: check that foundational files appear before orchestrators.
	if len(out.MustRead) >= 3 {
		if !isTopoOrdered(out.MustRead) {
			score -= 0.5
			fg.Notes = append(fg.Notes, "must_read: topo ordering not consistent (foundations should precede orchestrators)")
			deductions++
		}
	}

	if deductions == 0 && len(out.MustRead) > 0 {
		fg.Notes = append(fg.Notes, "must_read: good coverage with symbols, directions, and ordering")
	}

	return clamp(score, 1.0, 4.0)
}

// isTopoOrdered checks if foundational files (low outdegree) appear before orchestrators (high outdegree).
func isTopoOrdered(entries []model.MustReadEntry) bool {
	lastFoundationIdx := -1
	firstOrchestratorIdx := -1
	for i, e := range entries {
		if e.Role == "foundation" {
			lastFoundationIdx = i
		}
		if e.Role == "orchestrator" && firstOrchestratorIdx == -1 {
			firstOrchestratorIdx = i
		}
	}
	if lastFoundationIdx >= 0 && firstOrchestratorIdx >= 0 {
		return lastFoundationIdx < firstOrchestratorIdx
	}
	return true // can't determine ordering, assume OK
}

// gradeLikelyModify grades the likely_modify section (weight 0.20).
func gradeLikelyModify(out *model.AnalysisOutput, fg *FileGrade) float64 {
	score := 4.0

	totalEntries := countLikelyModify(out.LikelyModify)

	if totalEntries == 0 {
		// Empty likely_modify is OK for leaf/foundation files with low outdegree.
		fg.Notes = append(fg.Notes, "likely_modify: empty result (acceptable for leaf/foundation files)")
		return 3.0
	}

	// Check confidence tier distribution.
	highCount, medCount, lowCount := countLikelyModifyTiers(out.LikelyModify)

	// All same tier is suspicious for larger sets.
	if totalEntries >= 3 && (highCount == totalEntries || lowCount == totalEntries) {
		score -= 0.5
		fg.Notes = append(fg.Notes, "likely_modify: all entries in same confidence tier")
	}

	// Check signal variety.
	signalTypes := collectSignalTypes(out.LikelyModify)
	if len(signalTypes) < 2 && totalEntries >= 3 {
		score -= 0.5
		fg.Notes = append(fg.Notes, "likely_modify: low signal variety")
	}

	if score >= 3.5 {
		fg.Notes = append(fg.Notes, fmt.Sprintf("likely_modify: good distribution (high=%d, med=%d, low=%d)", highCount, medCount, lowCount))
	}

	return clamp(score, 1.0, 4.0)
}

// gradeTests grades the tests section (weight 0.15).
func gradeTests(out *model.AnalysisOutput, fg *FileGrade) float64 {
	score := 4.0

	if len(out.Tests) == 0 {
		// Subject is a test file — empty tests is correct by design.
		if classify.IsTestFile(out.Subject) {
			fg.Notes = append(fg.Notes, "tests: subject is a test file (tests section N/A)")
			return 4.0
		}
		// No test candidates in the entire pool — likely no tests exist for this file.
		if out.TestsNote == "no test files found in nearby directories" {
			fg.Notes = append(fg.Notes, "tests: no test files found (likely untested)")
			return 3.0
		}
		// Test candidates existed but none matched — possible detection failure.
		fg.Notes = append(fg.Notes, "tests: no test files matched (candidates existed)")
		return 2.0
	}

	directCount := 0
	dependencyCount := 0
	for _, t := range out.Tests {
		if t.Direct {
			directCount++
		} else {
			dependencyCount++
		}
	}

	if directCount == 0 {
		score -= 1.0
		fg.Notes = append(fg.Notes, "tests: no direct test files (only dependency tier)")
	}

	// All tests in dependency tier — no direct tests is a problem.
	// All-direct is the ideal state and should not be penalized.
	if len(out.Tests) >= 2 && directCount == 0 {
		score -= 0.5
		fg.Notes = append(fg.Notes, "tests: all tests in dependency tier")
	}

	if score >= 3.5 {
		fg.Notes = append(fg.Notes, fmt.Sprintf("tests: good coverage (direct=%d, dependency=%d)", directCount, dependencyCount))
	}

	return clamp(score, 1.0, 4.0)
}

// gradeRelated grades the related section (weight 0.15).
func gradeRelated(out *model.AnalysisOutput, fg *FileGrade) float64 {
	score := 4.0

	totalEntries := countRelated(out.Related)

	if totalEntries == 0 {
		fg.Notes = append(fg.Notes, "related: empty (may be OK for isolated files)")
		return 2.5
	}

	// Check signal informativeness.
	hasUsefulSignals := false
	for _, entries := range out.Related {
		for _, e := range entries {
			for _, s := range e.Signals {
				if strings.Contains(s, "distance:2") || strings.Contains(s, "co_change") ||
					strings.Contains(s, "structural") || strings.Contains(s, "hidden_coupling") ||
					strings.Contains(s, "two_hop") || strings.Contains(s, "transitive_caller") ||
					strings.Contains(s, "same_package") || strings.Contains(s, "high_churn") ||
					strings.Contains(s, "hotspot") || strings.Contains(s, "imports") ||
					strings.Contains(s, "imported_by") {
					hasUsefulSignals = true
				}
			}
		}
	}

	if !hasUsefulSignals && totalEntries >= 3 {
		score -= 0.5
		fg.Notes = append(fg.Notes, "related: signals not informative")
	}

	if len(out.MustReadNote) > 0 && strings.Contains(out.MustReadNote, "overflow") {
		// Overflow from must_read to related — check it actually happened.
		fg.Notes = append(fg.Notes, "related: includes overflow from must_read")
	}

	if score >= 3.5 {
		fg.Notes = append(fg.Notes, fmt.Sprintf("related: %d entries with informative signals", totalEntries))
	}

	return clamp(score, 1.0, 4.0)
}

// gradeBlastRadius grades the blast_radius section (weight 0.10).
func gradeBlastRadius(out *model.AnalysisOutput, fg *FileGrade) float64 {
	if out.BlastRadius == nil {
		fg.Notes = append(fg.Notes, "blast_radius: missing")
		return 1.0
	}

	score := 4.0
	br := out.BlastRadius

	// Level must be one of the valid values.
	validLevels := map[string]bool{"low": true, "medium": true, "high": true}
	if !validLevels[br.Level] {
		score -= 2.0
		fg.Notes = append(fg.Notes, fmt.Sprintf("blast_radius: invalid level %q", br.Level))
	}

	// Detail should be non-empty.
	if br.Detail == "" {
		score -= 1.0
		fg.Notes = append(fg.Notes, "blast_radius: missing detail string")
	}

	// Cross-check: high blast_radius should correlate with many likely_modify entries.
	likelyCount := countLikelyModify(out.LikelyModify)
	if br.Level == "high" && likelyCount < 3 {
		score -= 0.5
		fg.Notes = append(fg.Notes, "blast_radius: 'high' level but few likely_modify entries")
	}
	if br.Level == "low" && likelyCount > 10 {
		score -= 0.5
		fg.Notes = append(fg.Notes, "blast_radius: 'low' level but many likely_modify entries")
	}

	if score >= 3.5 {
		fg.Notes = append(fg.Notes, fmt.Sprintf("blast_radius: %s level, fragility=%.2f", br.Level, br.Fragility))
	}

	return clamp(score, 1.0, 4.0)
}

// --- helpers ---

func countLikelyModify(lm map[string][]model.LikelyModifyEntry) int {
	total := 0
	for _, entries := range lm {
		total += len(entries)
	}
	return total
}

func countLikelyModifyTiers(lm map[string][]model.LikelyModifyEntry) (high, med, low int) {
	for _, entries := range lm {
		for _, e := range entries {
			switch e.Confidence {
			case "high":
				high++
			case "medium":
				med++
			case "low":
				low++
			}
		}
	}
	return
}

func collectSignalTypes(lm map[string][]model.LikelyModifyEntry) map[string]bool {
	types := map[string]bool{}
	for _, entries := range lm {
		for _, e := range entries {
			for _, s := range e.Signals {
				// Extract signal type (before colon).
				if idx := strings.IndexByte(s, ':'); idx > 0 {
					types[s[:idx]] = true
				} else {
					types[s] = true
				}
			}
		}
	}
	return types
}

func countRelated(r map[string][]model.RelatedEntry) int {
	total := 0
	for _, entries := range r {
		total += len(entries)
	}
	return total
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// DetectIssues scans analysis output for common problems.
func DetectIssues(out *model.AnalysisOutput) []Issue {
	var issues []Issue

	if out.SchemaVersion != "3.2" {
		issues = append(issues, Issue{
			Severity: "warning",
			Section:  "schema",
			Message:  fmt.Sprintf("unexpected schema version %q (expected 3.2)", out.SchemaVersion),
		})
	}

	if out.Confidence < 0.5 {
		issues = append(issues, Issue{
			Severity: "error",
			File:     out.Subject,
			Section:  "confidence",
			Message:  fmt.Sprintf("very low confidence (%.2f) — many unresolved imports", out.Confidence),
		})
	}

	if out.BlastRadius == nil {
		issues = append(issues, Issue{
			Severity: "error",
			File:     out.Subject,
			Section:  "blast_radius",
			Message:  "blast_radius is nil",
		})
	}

	if out.Stats == nil {
		issues = append(issues, Issue{
			Severity: "warning",
			File:     out.Subject,
			Section:  "stats",
			Message:  "index stats missing from output",
		})
	}

	return issues
}
