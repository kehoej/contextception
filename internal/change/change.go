// Package change provides reusable logic for building PR-level change reports.
// Extracted from the analyze-change CLI command so the GitHub App worker can
// call the same pipeline programmatically.
package change

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/model"
)

// ChangeSchemaVersion is the current schema version for change reports.
const ChangeSchemaVersion = "1.0"

// Config holds parameters for building a change report.
type Config struct {
	RepoRoot    string
	RepoCfg     *config.Config
	AnalyzerCfg analyzer.Config
}

// BuildReport produces a ChangeReport by diffing base..head, analyzing
// each changed file, and aggregating the results.
func BuildReport(idx *db.Index, cfg Config, base, head string) (*model.ChangeReport, error) {
	refRange := base + ".." + head

	// Get changed files from git diff.
	diffs, err := GitDiffFiles(cfg.RepoRoot, base, head)
	if err != nil {
		return nil, fmt.Errorf("running git diff: %w", err)
	}

	// Fallback: if no committed changes found, diff the working tree against base.
	// This supports the pre-PR workflow where changes aren't committed yet.
	workingTree := false
	if len(diffs) == 0 {
		diffs, err = GitDiffWorkingTree(cfg.RepoRoot, base)
		if err != nil {
			return nil, fmt.Errorf("running git diff (working tree): %w", err)
		}
		if len(diffs) > 0 {
			workingTree = true
			refRange = base + "..working-tree"
		}
	}

	if len(diffs) == 0 {
		return &model.ChangeReport{
			SchemaVersion: ChangeSchemaVersion,
			RefRange:      refRange,
			ChangedFiles:  []model.ChangedFile{},
			BlastRadius:   &model.BlastRadius{Level: "low", Detail: "no changed files"},
			MustRead:      []model.MustReadEntry{},
			LikelyModify:  map[string][]model.LikelyModifyEntry{},
			Tests:         []model.TestEntry{},
		}, nil
	}
	_ = workingTree // available for future use (e.g., labeling output)

	// Filter to analyzable files (existing, non-deleted, in the index).
	var analyzable []string
	changedFiles := make([]model.ChangedFile, len(diffs))
	for i, d := range diffs {
		cf := model.ChangedFile{
			File:   d.Path,
			Status: d.Status,
		}

		if d.Status != "deleted" {
			exists, _ := idx.FileExists(d.Path)
			cf.Indexed = exists
			if exists {
				analyzable = append(analyzable, d.Path)
			}
		}

		changedFiles[i] = cf
	}

	// Build the analyzer.
	acfg := cfg.AnalyzerCfg
	acfg.RepoConfig = cfg.RepoCfg
	acfg.RepoRoot = cfg.RepoRoot
	a := analyzer.New(idx, acfg)

	// Analyze all analyzable files together.
	var analysis *model.AnalysisOutput
	if len(analyzable) > 0 {
		if len(analyzable) == 1 {
			analysis, err = a.Analyze(analyzable[0])
		} else {
			analysis, err = a.AnalyzeMulti(analyzable)
		}
		if err != nil {
			return nil, fmt.Errorf("analyzing changed files: %w", err)
		}
	}

	// Run per-file analysis for individual blast radius and risk scoring.
	perFileAnalysis := make(map[string]*model.AnalysisOutput)
	perFileBlast := make(map[string]*model.BlastRadius)
	for _, file := range analyzable {
		single, err := a.Analyze(file)
		if err != nil {
			continue
		}
		perFileAnalysis[file] = single
		perFileBlast[file] = single.BlastRadius
	}

	// Annotate changed files with per-file blast radius.
	for i := range changedFiles {
		if br, ok := perFileBlast[changedFiles[i].File]; ok {
			changedFiles[i].BlastRadius = br
		}
	}

	// Compute per-file risk scores.
	maxIndegree, _ := idx.GetMaxIndegree()
	changedPaths := make([]string, len(diffs))
	for i, d := range diffs {
		changedPaths[i] = d.Path
	}
	for i := range changedFiles {
		pfa := perFileAnalysis[changedFiles[i].File]
		score, factors := analyzer.ComputeRiskScore(changedFiles[i], pfa, idx, changedPaths, maxIndegree)
		changedFiles[i].RiskScore = score
		changedFiles[i].RiskTier = analyzer.AssignRiskTier(score)
		changedFiles[i].RiskFactors = factors
		changedFiles[i].RiskNarrative = analyzer.GenerateNarrative(changedFiles[i], pfa, idx)
	}

	// Build risk triage.
	triage := buildRiskTriage(changedFiles)

	// Build aggregate risk.
	aggRisk := buildAggregateRisk(changedFiles, perFileAnalysis, triage)

	// Generate test suggestions.
	testSuggestions := analyzer.GenerateTestSuggestions(changedFiles, perFileAnalysis)

	// Detect coupling between changed files.
	coupling := DetectCoupling(idx, analyzable)

	// Detect test coverage gaps.
	testGaps := DetectTestGaps(analyzable, analysis)

	// Extract hidden coupling from analysis.
	changedSet := make(map[string]bool, len(diffs))
	for _, d := range diffs {
		changedSet[d.Path] = true
	}
	hiddenCoupling := ExtractHiddenCoupling(analysis, changedSet)

	// Build summary.
	summary := BuildSummary(changedFiles, perFileBlast)

	// Build the change report.
	report := &model.ChangeReport{
		SchemaVersion:   "1.0",
		RefRange:        refRange,
		ChangedFiles:    changedFiles,
		Summary:         summary,
		Coupling:        coupling,
		TestGaps:        testGaps,
		HiddenCoupling:  hiddenCoupling,
		RiskTriage:      triage,
		AggregateRisk:   aggRisk,
		TestSuggestions: testSuggestions,
	}

	if analysis != nil {
		report.BlastRadius = analysis.BlastRadius
		report.MustRead = analysis.MustRead
		report.LikelyModify = analysis.LikelyModify
		report.Tests = analysis.Tests
		report.Hotspots = analysis.Hotspots
		report.CircularDeps = analysis.CircularDeps
		report.Stats = analysis.Stats
	} else {
		report.BlastRadius = &model.BlastRadius{Level: "low", Detail: "no analyzable files in diff"}
		report.MustRead = []model.MustReadEntry{}
		report.LikelyModify = map[string][]model.LikelyModifyEntry{}
		report.Tests = []model.TestEntry{}
	}

	return report, nil
}

// buildRiskTriage groups changed files by risk tier.
func buildRiskTriage(files []model.ChangedFile) *model.RiskTriage {
	triage := &model.RiskTriage{
		Critical: []string{},
		Test:     []string{},
		Review:   []string{},
		Safe:     []string{},
	}
	for _, f := range files {
		switch f.RiskTier {
		case analyzer.TierCritical:
			triage.Critical = append(triage.Critical, f.File)
		case analyzer.TierTest:
			triage.Test = append(triage.Test, f.File)
		case analyzer.TierReview:
			triage.Review = append(triage.Review, f.File)
		default:
			triage.Safe = append(triage.Safe, f.File)
		}
	}
	sort.Strings(triage.Critical)
	sort.Strings(triage.Test)
	sort.Strings(triage.Review)
	sort.Strings(triage.Safe)
	return triage
}

// buildAggregateRisk computes the aggregate risk for the entire PR.
func buildAggregateRisk(files []model.ChangedFile, analyses map[string]*model.AnalysisOutput, triage *model.RiskTriage) *model.AggregateRisk {
	maxScore := 0
	testedCount := 0
	totalNonTest := 0

	for _, f := range files {
		if f.RiskScore > maxScore {
			maxScore = f.RiskScore
		}
		if !isTestFile(f.File) {
			totalNonTest++
			a := analyses[f.File]
			if a != nil {
				for _, t := range a.Tests {
					if t.Direct {
						testedCount++
						break
					}
				}
			}
		}
	}

	var coverageRatio float64
	if totalNonTest > 0 {
		coverageRatio = float64(testedCount) / float64(totalNonTest)
	}

	// Generate regression risk summary.
	// Triggers for: CRITICAL tier files, or any modified file with 10+ importers.
	regressionRisk := ""
	var regressionFiles []string
	if len(triage.Critical) > 0 {
		regressionFiles = triage.Critical
	} else {
		// Find high-importer files across all tiers (foundation files).
		for _, f := range files {
			a := analyses[f.File]
			if a == nil || f.Status != "modified" {
				continue
			}
			importerCount := 0
			for _, entry := range a.MustRead {
				if entry.Direction == "imported_by" {
					importerCount++
				}
			}
			if importerCount >= 10 {
				regressionFiles = append(regressionFiles, f.File)
			}
		}
	}
	if len(regressionFiles) > 0 {
		regressionRisk = buildRegressionSummary(regressionFiles, analyses)
	} else if allAdded(files) {
		regressionRisk = "No modifications to existing code. Low regression risk."
	}

	return &model.AggregateRisk{
		Score:             maxScore,
		RegressionRisk:    regressionRisk,
		TestCoverageRatio: coverageRatio,
	}
}

// buildRegressionSummary generates a brief summary of regression risk from critical files.
func buildRegressionSummary(criticalFiles []string, analyses map[string]*model.AnalysisOutput) string {
	var parts []string
	for _, file := range criticalFiles {
		a := analyses[file]
		if a == nil {
			continue
		}
		importerCount := 0
		untestedImporters := 0
		for _, entry := range a.MustRead {
			if entry.Direction == "imported_by" {
				importerCount++
			}
		}
		// Count untested importers (rough heuristic: importers not in test files).
		for _, entry := range a.MustRead {
			if entry.Direction == "imported_by" {
				hasTest := false
				for _, t := range a.Tests {
					if t.Direct && strings.Contains(t.File, strings.TrimSuffix(filepath.Base(entry.File), filepath.Ext(entry.File))) {
						hasTest = true
						break
					}
				}
				if !hasTest {
					untestedImporters++
				}
			}
		}
		base := filepath.Base(file)
		if untestedImporters > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d importers, %d untested", base, importerCount, untestedImporters))
		} else if importerCount > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d importers", base, importerCount))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	summary := strings.Join(parts, "; ")
	if len(summary) > 300 {
		summary = summary[:297] + "..."
	}
	return summary
}

// allAdded returns true if all files in the changeset are added.
func allAdded(files []model.ChangedFile) bool {
	for _, f := range files {
		if f.Status != "added" {
			return false
		}
	}
	return true
}

// isTestFile is a local helper to avoid circular import with classify.
func isTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.")
}

// DiffEntry represents a file changed in a git diff.
type DiffEntry struct {
	Path   string
	Status string // "added", "modified", "deleted", "renamed"
}

// GitDiffFiles returns the list of changed files between two refs.
func GitDiffFiles(repoRoot, base, head string) ([]DiffEntry, error) {
	cmd := exec.Command("git", "diff", "--name-status", "--no-renames", base+".."+head)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}
	return parseDiffOutput(out), nil
}

// GitDiffWorkingTree returns files changed in the working tree vs a base ref.
// This includes both staged and unstaged changes — everything that differs from base.
func GitDiffWorkingTree(repoRoot, base string) ([]DiffEntry, error) {
	cmd := exec.Command("git", "diff", "--name-status", "--no-renames", base)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff (working tree) failed: %w", err)
	}
	return parseDiffOutput(out), nil
}

// parseDiffOutput parses git diff --name-status output into DiffEntry slice.
func parseDiffOutput(out []byte) []DiffEntry {
	var entries []DiffEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status := ParseGitStatus(parts[0])
		path := strings.TrimSpace(parts[1])

		// Normalize to forward slashes and make repo-relative.
		path = strings.ReplaceAll(filepath.Clean(path), string(os.PathSeparator), "/")

		entries = append(entries, DiffEntry{Path: path, Status: status})
	}
	return entries
}

// ParseGitStatus converts git's single-letter status code to a human-readable string.
// Handles: A (added), D (deleted), M (modified), R### (renamed). Defaults to "modified".
func ParseGitStatus(s string) string {
	if strings.HasPrefix(s, "R") {
		return "renamed"
	}
	switch s {
	case "A":
		return "added"
	case "D":
		return "deleted"
	default:
		return "modified"
	}
}

// DetectCoupling finds structural connections between changed files.
func DetectCoupling(idx *db.Index, files []string) []model.CouplingPair {
	if len(files) < 2 {
		return nil
	}

	fileSet := make(map[string]bool, len(files))
	for _, f := range files {
		fileSet[f] = true
	}

	edges, err := idx.GetEdgesAmong(files)
	if err != nil {
		return nil
	}

	// Track seen pairs to avoid duplicates.
	type pair struct{ a, b string }
	seen := make(map[pair]bool)
	var pairs []model.CouplingPair

	for _, e := range edges {
		if !fileSet[e.Src] || !fileSet[e.Dst] {
			continue
		}
		// Normalize pair order.
		a, b := e.Src, e.Dst
		if a > b {
			a, b = b, a
		}
		p := pair{a, b}
		if seen[p] {
			// Upgrade to mutual if we see both directions.
			for i := range pairs {
				if (pairs[i].FileA == a && pairs[i].FileB == b) && pairs[i].Direction != "mutual" {
					pairs[i].Direction = "mutual"
				}
			}
			continue
		}
		seen[p] = true

		direction := "a_imports_b"
		if e.Src == b {
			direction = "b_imports_a"
		}
		pairs = append(pairs, model.CouplingPair{
			FileA:     a,
			FileB:     b,
			Direction: direction,
		})
	}

	return pairs
}

// DetectTestGaps finds changed files that have no direct test coverage.
func DetectTestGaps(analyzable []string, analysis *model.AnalysisOutput) []string {
	if analysis == nil {
		return nil
	}

	// Build set of files with direct test coverage.
	testedFiles := make(map[string]bool)
	for _, t := range analysis.Tests {
		if t.Direct {
			testedFiles[t.File] = true
		}
	}

	var gaps []string
	for _, file := range analyzable {
		if classify.IsTestFile(file) {
			continue
		}
		if !testedFiles[file] && !HasTestForFile(file, analysis) {
			gaps = append(gaps, file)
		}
	}
	sort.Strings(gaps)
	return gaps
}

// HasTestForFile checks if any test file appears to test the given file.
// Uses stem matching first, then falls back to same-directory matching for Go files
// (where any _test.go in the same package tests all files in that package).
func HasTestForFile(file string, analysis *model.AnalysisOutput) bool {
	if analysis == nil {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	dir := filepath.Dir(file)
	for _, t := range analysis.Tests {
		if !t.Direct {
			continue
		}
		// Stem match: test filename contains the source stem.
		if strings.Contains(filepath.Base(t.File), base) {
			return true
		}
		// Same-directory match for Go: any _test.go in the same package covers all files.
		if strings.HasSuffix(file, ".go") && filepath.Dir(t.File) == dir {
			return true
		}
	}
	return false
}

// BuildSummary creates aggregate statistics from the changed files.
func BuildSummary(files []model.ChangedFile, perFileBlast map[string]*model.BlastRadius) model.ChangeSummary {
	s := model.ChangeSummary{TotalFiles: len(files)}
	for _, f := range files {
		switch f.Status {
		case "added":
			s.Added++
		case "modified":
			s.Modified++
		case "deleted":
			s.Deleted++
		case "renamed":
			s.Renamed++
		}
		if f.Indexed {
			s.IndexedFiles++
		}
		if classify.IsTestFile(f.File) {
			s.TestFiles++
		}
		if br, ok := perFileBlast[f.File]; ok && br != nil && br.Level == "high" {
			s.HighRiskFiles++
		}
	}
	return s
}

// ExtractHiddenCoupling parses hidden_coupling signals from AnalysisOutput.Related
// and returns entries for partners not in the changed file set.
func ExtractHiddenCoupling(analysis *model.AnalysisOutput, changedFiles map[string]bool) []model.HiddenCouplingEntry {
	if analysis == nil {
		return nil
	}

	var entries []model.HiddenCouplingEntry
	for _, related := range analysis.Related {
		for _, entry := range related {
			for _, sig := range entry.Signals {
				if !strings.HasPrefix(sig, "hidden_coupling:") {
					continue
				}
				freqStr := strings.TrimPrefix(sig, "hidden_coupling:")
				freq, err := strconv.Atoi(freqStr)
				if err != nil {
					continue
				}
				// The partner is the related file; find which changed file triggered it.
				// In multi-file analysis, the subject is merged — we associate with any
				// changed file since the signal came from the combined analysis.
				if !changedFiles[entry.File] {
					entries = append(entries, model.HiddenCouplingEntry{
						ChangedFile: analysis.Subject,
						Partner:     entry.File,
						Frequency:   freq,
					})
				}
			}
		}
	}

	return entries
}
