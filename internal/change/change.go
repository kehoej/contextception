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

	// Run per-file analysis for individual blast radius.
	perFileBlast := make(map[string]*model.BlastRadius)
	for _, file := range analyzable {
		single, err := a.Analyze(file)
		if err != nil {
			continue
		}
		perFileBlast[file] = single.BlastRadius
	}

	// Annotate changed files with per-file blast radius.
	for i := range changedFiles {
		if br, ok := perFileBlast[changedFiles[i].File]; ok {
			changedFiles[i].BlastRadius = br
		}
	}

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
		SchemaVersion:  "1.0",
		RefRange:       refRange,
		ChangedFiles:   changedFiles,
		Summary:        summary,
		Coupling:       coupling,
		TestGaps:       testGaps,
		HiddenCoupling: hiddenCoupling,
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

	return entries, nil
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
func HasTestForFile(file string, analysis *model.AnalysisOutput) bool {
	if analysis == nil {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	for _, t := range analysis.Tests {
		if t.Direct && strings.Contains(filepath.Base(t.File), base) {
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
