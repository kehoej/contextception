package analyzer

import (
	"fmt"
	"math"
	"path"
	"strings"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/model"
)

// Risk tier thresholds.
const (
	tierSafeMax     = 20
	tierReviewMax   = 50
	tierTestMax     = 75
	riskScoreMax    = 100
	riskScoreV1     = 1   // score_version for percentile filtering
	narrativeMaxLen = 200 // max characters for risk narrative
)

// RiskTier constants.
const (
	TierSafe     = "SAFE"
	TierReview   = "REVIEW"
	TierTest     = "TEST"
	TierCritical = "CRITICAL"
)

// ComputeRiskScore calculates a per-file risk score (0-100).
//
// Formula:
//  1. Base score by status (added/modified/deleted/renamed)
//  2. Structural risk for modified files (importers, co-change, fragility, mutual deps, cycles)
//  3. Coverage adjustment (direct tests reduce, no tests increase)
//  4. Clamp to [0, 100]
func ComputeRiskScore(file model.ChangedFile, analysis *model.AnalysisOutput, idx *db.Index, changedPaths []string, maxIndegree int) (int, []string) {
	var factors []string

	// Step 1: Base score by status.
	base := baseScore(file, analysis, &factors)

	// Nil guard: if not indexed or no analysis, return base only.
	// Non-code files (docs, config, etc.) get reduced scores since they can't
	// cause code regressions.
	if analysis == nil || !file.Indexed {
		if !isCodeFile(file.File) {
			base = base / 3 // docs/config: 30 -> 10 (SAFE)
			if base < 5 {
				base = 5
			}
			factors = append(factors, "non-code file")
		}
		return clampScore(base), factors
	}

	// Step 2: Structural risk (modified files only).
	structural := 0.0
	if file.Status == "modified" {
		structural = structuralRisk(file, analysis, idx, changedPaths, maxIndegree, &factors)
	}

	raw := float64(base) + structural

	// Step 3: Coverage adjustment.
	multiplier := coverageMultiplier(file, analysis, idx, &factors)
	result := raw * multiplier

	return clampScore(int(math.Round(result))), factors
}

// baseScore returns the base risk score for a file based on its change status.
func baseScore(file model.ChangedFile, analysis *model.AnalysisOutput, factors *[]string) int {
	switch file.Status {
	case "added":
		// Added files with exports are higher risk.
		if analysis != nil && hasExports(file.File, analysis) {
			*factors = append(*factors, "new file with exports")
			return 20
		}
		*factors = append(*factors, "new file")
		return 10
	case "modified":
		*factors = append(*factors, "modified")
		return 30
	case "deleted":
		*factors = append(*factors, "deleted")
		return 5
	case "renamed":
		*factors = append(*factors, "renamed")
		return 5
	default:
		*factors = append(*factors, "modified")
		return 30
	}
}

// hasExports checks if a file has exported symbols (Go exported or TS/JS exports).
func hasExports(filePath string, analysis *model.AnalysisOutput) bool {
	if analysis == nil {
		return false
	}
	// Check if this file appears as an import target for other files.
	// If it has reverse importers in the analysis, it effectively has exports.
	for _, entry := range analysis.MustRead {
		if entry.Direction == "imported_by" {
			return true
		}
	}
	// Go files with uppercase symbols or TS/JS files that are commonly exported.
	ext := path.Ext(filePath)
	return ext == ".go" || ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx"
}

// countEffectiveImporters returns the effective importer count for a file.
// For Go files, uses max(direct_reverse_importers, package_importers).
func countEffectiveImporters(file model.ChangedFile, analysis *model.AnalysisOutput, idx *db.Index) int {
	count := 0
	if analysis != nil {
		for _, entry := range analysis.MustRead {
			if entry.Direction == "imported_by" {
				count++
			}
		}
	}
	// For Go files: use max(direct, package-level) importer count.
	if idx != nil && strings.HasSuffix(file.File, ".go") && !strings.HasSuffix(file.File, "_test.go") {
		pkgDir := path.Dir(file.File)
		if pkgCount, err := idx.GetPackageImporterCount(pkgDir); err == nil && pkgCount > count {
			count = pkgCount
		}
	}
	return count
}

// structuralRisk computes the structural risk component for modified files.
func structuralRisk(file model.ChangedFile, analysis *model.AnalysisOutput, idx *db.Index, changedPaths []string, maxIndegree int, factors *[]string) float64 {
	var risk float64

	importerCount := countEffectiveImporters(file, analysis, idx)

	var normalizedImporters float64
	if maxIndegree > 0 {
		normalizedImporters = float64(importerCount) / float64(maxIndegree)
		if normalizedImporters > 1.0 {
			normalizedImporters = 1.0
		}
	}
	importerRisk := normalizedImporters * 3.0
	if importerRisk > 0 {
		*factors = append(*factors, fmt.Sprintf("%d importers", importerCount))
	}
	risk += importerRisk

	// Co-change with other PR files.
	coChangeTotal := 0
	if idx != nil && len(changedPaths) > 1 {
		partners, err := idx.GetCoChangePartners(file.File, coChangeWindowDays, 0)
		if err == nil {
			changedSet := make(map[string]bool, len(changedPaths))
			for _, p := range changedPaths {
				changedSet[p] = true
			}
			for _, p := range partners {
				if changedSet[p.Path] && p.Path != file.File {
					coChangeTotal += p.Frequency
				}
			}
		}
	}
	coChangeRisk := float64(coChangeTotal) * 2.0
	if coChangeRisk > 10 {
		coChangeRisk = 10 // cap co-change contribution
	}
	if coChangeTotal > 0 {
		*factors = append(*factors, fmt.Sprintf("co-change freq %d", coChangeTotal))
	}
	risk += coChangeRisk

	// Fragility: Ce/(Ca+Ce) where Ce=outdegree, Ca=indegree.
	if analysis.BlastRadius != nil {
		fragility := analysis.BlastRadius.Fragility
		if fragility > 0 {
			fragilityRisk := fragility * 15.0
			*factors = append(*factors, fmt.Sprintf("fragility %.2f", fragility))
			risk += fragilityRisk
		}
	}

	// Mutual dependencies.
	mutualCount := 0
	for _, entry := range analysis.MustRead {
		if entry.Direction == "mutual" {
			mutualCount++
		}
	}
	mutualRisk := float64(mutualCount) * 5.0
	if mutualCount > 0 {
		*factors = append(*factors, fmt.Sprintf("%d mutual deps", mutualCount))
	}
	risk += mutualRisk

	// Circular dependencies.
	cycleCount := len(analysis.CircularDeps)
	cycleRisk := float64(cycleCount) * 10.0
	if cycleCount > 0 {
		*factors = append(*factors, fmt.Sprintf("%d cycles", cycleCount))
	}
	risk += cycleRisk

	return risk
}

// coverageMultiplier adjusts risk based on test coverage.
// Uses index-based stem matching (not analysis.Tests which may be capped by mode).
func coverageMultiplier(file model.ChangedFile, analysis *model.AnalysisOutput, idx *db.Index, factors *[]string) float64 {
	if classify.IsTestFile(file.File) {
		return 0.7 // test files themselves are lower risk
	}

	// Check for direct tests via index-based stem matching.
	hasDirectTest := false
	hasDepTest := false

	if idx != nil {
		dir := path.Dir(file.File)
		ext := path.Ext(file.File)
		stem := strings.TrimSuffix(path.Base(file.File), ext)

		// Look for convention-matched test files.
		switch {
		case strings.HasSuffix(file.File, ".go"):
			testFiles, err := idx.GetTestFilesInDir(dir, "_test.go")
			if err == nil {
				for _, tf := range testFiles {
					tfBase := path.Base(tf)
					if strings.Contains(tfBase, stem) {
						hasDirectTest = true
						break
					}
				}
			}
		case strings.HasSuffix(file.File, ".py"):
			// Python: test_<stem>.py
			testName := "test_" + stem + ".py"
			testFiles, err := idx.GetTestFilesByName("", []string{testName})
			if err == nil && len(testFiles) > 0 {
				hasDirectTest = true
			}
		}
	}

	// Fallback: check analysis.Tests when index-based stem matching didn't find tests.
	// This catches cross-file test patterns (e.g., cli_test.go testing analyze.go,
	// server_test.go testing tools.go) that stem matching misses.
	if !hasDirectTest && analysis != nil {
		for _, t := range analysis.Tests {
			if t.Direct {
				hasDirectTest = true
				break
			}
		}
		if !hasDirectTest {
			for _, t := range analysis.Tests {
				if !t.Direct {
					hasDepTest = true
					break
				}
			}
		}
	}

	if hasDirectTest {
		*factors = append(*factors, "has direct tests")
		return 0.7
	}
	if hasDepTest {
		*factors = append(*factors, "has dependency tests only")
		return 0.85
	}
	*factors = append(*factors, "no direct tests")
	return 1.2
}

// AssignRiskTier maps a risk score to a tier label.
func AssignRiskTier(score int) string {
	switch {
	case score <= tierSafeMax:
		return TierSafe
	case score <= tierReviewMax:
		return TierReview
	case score <= tierTestMax:
		return TierTest
	default:
		return TierCritical
	}
}

// GenerateNarrative produces a human-readable risk explanation for a file.
// idx is optional — when provided, Go files use package-level importer counts.
func GenerateNarrative(file model.ChangedFile, analysis *model.AnalysisOutput, idx *db.Index) string {
	// Edge case: empty diff.
	if file.File == "" {
		return "No changes detected."
	}

	// Edge case: all-added PR.
	if file.Status == "added" {
		return "New file. No modifications to existing code. Low regression risk."
	}

	// Edge case: deleted file.
	if file.Status == "deleted" {
		importerCount := countEffectiveImporters(file, analysis, idx)
		if importerCount > 0 {
			return fmt.Sprintf("Deleted. %d files imported this. Check for broken imports.", importerCount)
		}
		return "Deleted. No known importers."
	}

	// Modified file: build narrative from signals.
	var parts []string

	// Status.
	parts = append(parts, "Modified")

	// Importer count (uses package-level for Go files when idx available).
	importerCount := countEffectiveImporters(file, analysis, idx)
	if importerCount > 0 {
		parts = append(parts, fmt.Sprintf("%d files import this directly", importerCount))
	}

	// Test gap signal (skip for test files — they don't need tests for themselves).
	if analysis != nil && !classify.IsTestFile(file.File) {
		hasTest := false
		for _, t := range analysis.Tests {
			if t.Direct {
				hasTest = true
				break
			}
		}
		if !hasTest {
			parts = append(parts, "no direct tests")
		} else {
			parts = append(parts, "has direct tests")
		}
	}

	// Fragility — recompute using effective importer count for consistency.
	// The per-file analysis uses file-level indegree which can be 0 for Go package files,
	// producing fragility=1.0 even when the package has many importers.
	if analysis != nil && analysis.BlastRadius != nil {
		outdegree := 0
		for _, entry := range analysis.MustRead {
			if entry.Direction == "imports" {
				outdegree++
			}
		}
		effectiveIndegree := importerCount // uses package-level count from above
		total := float64(outdegree + effectiveIndegree)
		if total > 0 {
			fragility := float64(outdegree) / total
			if fragility > 0.01 { // skip near-zero
				parts = append(parts, fmt.Sprintf("fragility: %.2f", fragility))
			}
		}
	}

	// Role.
	// Try to determine from must_read entries or blast radius.
	if analysis != nil {
		for _, entry := range analysis.MustRead {
			if entry.Role != "" && entry.Role != "unknown" {
				// The subject's role isn't in must_read (those are its deps).
				// We'd need classify.ClassifyRole but don't have the signals here.
				break
			}
		}
	}

	narrative := strings.Join(parts, ". ") + "."

	// Truncate to max length.
	if len(narrative) > narrativeMaxLen {
		narrative = narrative[:narrativeMaxLen-3] + "..."
	}

	return narrative
}

// GenerateTestSuggestions creates test suggestions for high-risk untested files.
func GenerateTestSuggestions(changedFiles []model.ChangedFile, analyses map[string]*model.AnalysisOutput) []model.TestSuggestion {
	var suggestions []model.TestSuggestion

	for _, file := range changedFiles {
		tier := AssignRiskTier(file.RiskScore)
		analysis := analyses[file.File]
		if analysis == nil {
			continue
		}

		// Generate suggestions for TEST/CRITICAL tier, or REVIEW tier with high importer count.
		if tier == TierSafe {
			continue
		}
		if tier == TierReview {
			importerCount := 0
			for _, entry := range analysis.MustRead {
				if entry.Direction == "imported_by" {
					importerCount++
				}
			}
			if importerCount < 10 {
				continue // REVIEW with few importers: skip
			}
		}

		// Check if file has direct tests.
		hasDirectTest := false
		for _, t := range analysis.Tests {
			if t.Direct {
				hasDirectTest = true
				break
			}
		}
		if hasDirectTest {
			continue
		}

		// Find imported symbols to suggest testing.
		for _, entry := range analysis.MustRead {
			if entry.Direction != "imports" || len(entry.Symbols) == 0 {
				continue
			}
			symbolList := strings.Join(entry.Symbols, ", ")
			suggestions = append(suggestions, model.TestSuggestion{
				File:          file.File,
				SuggestedTest: fmt.Sprintf("Test %s's use of %s from %s", path.Base(file.File), symbolList, path.Base(entry.File)),
				Reason:        fmt.Sprintf("%s imports %s but has no direct test", path.Base(file.File), symbolList),
			})
			break // one suggestion per file
		}

		// If no symbol-specific suggestion, generate a generic one.
		if len(suggestions) == 0 || suggestions[len(suggestions)-1].File != file.File {
			suggestions = append(suggestions, model.TestSuggestion{
				File:          file.File,
				SuggestedTest: fmt.Sprintf("Test %s (risk tier: %s)", path.Base(file.File), tier),
				Reason:        fmt.Sprintf("%s has risk score %d but no direct test coverage", path.Base(file.File), file.RiskScore),
			})
		}
	}

	return suggestions
}

// isCodeFile returns true if the file has a recognized code extension.
func isCodeFile(filePath string) bool {
	ext := strings.ToLower(path.Ext(filePath))
	switch ext {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".java", ".rs",
		".rb", ".php", ".c", ".cpp", ".h", ".hpp", ".cs", ".swift",
		".kt", ".kts", ".scala", ".sh", ".bash", ".zsh":
		return true
	}
	return false
}

// clampScore ensures the score is within [0, riskScoreMax].
func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > riskScoreMax {
		return riskScoreMax
	}
	return score
}
