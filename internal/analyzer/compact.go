package analyzer

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/kehoej/contextception/internal/model"
)

// FormatCompact produces a token-optimized text summary of an analysis output.
// Targets ~60-75% fewer tokens than full JSON while preserving all essential information.
func FormatCompact(output *model.AnalysisOutput) string {
	if output == nil {
		return ""
	}

	var b strings.Builder

	// Header line with key metrics.
	conf := fmt.Sprintf("confidence: %.2f", output.Confidence)
	blast := "blast: n/a"
	if output.BlastRadius != nil {
		blast = "blast: " + output.BlastRadius.Level
	}
	fmt.Fprintf(&b, "Context: %s (%s, %s)\n", output.Subject, conf, blast)

	// Must read — one line per file with reason.
	if len(output.MustRead) > 0 {
		fmt.Fprintf(&b, "\nMust read (%d):\n", len(output.MustRead))
		for _, entry := range output.MustRead {
			reason := compactMustReadReason(entry)
			fmt.Fprintf(&b, "  %-40s %s\n", entry.File, reason)
		}
	}

	// Likely modify — inline list.
	lmParts := compactLikelyModify(output.LikelyModify)
	if len(lmParts) > 0 {
		fmt.Fprintf(&b, "\nLikely modify: %s\n", strings.Join(lmParts, ", "))
	}

	// Tests — inline list.
	if len(output.Tests) > 0 {
		var testParts []string
		for _, t := range output.Tests {
			tier := "direct"
			if !t.Direct {
				tier = "dep"
			}
			testParts = append(testParts, fmt.Sprintf("%s (%s)", t.File, tier))
		}
		fmt.Fprintf(&b, "Tests: %s\n", strings.Join(testParts, ", "))
	}

	// Warnings — collected inline.
	warnings := compactWarnings(output)
	if len(warnings) > 0 {
		fmt.Fprintf(&b, "Warnings: %s\n", strings.Join(warnings, "; "))
	}

	return b.String()
}

// FormatCompactChange produces a token-optimized text summary of a change report.
func FormatCompactChange(report *model.ChangeReport) string {
	if report == nil {
		return ""
	}

	var b strings.Builder

	// Header.
	blast := "n/a"
	if report.BlastRadius != nil {
		blast = report.BlastRadius.Level
	}
	fmt.Fprintf(&b, "Change: %s (blast: %s, files: %d)\n",
		report.RefRange, blast, report.Summary.TotalFiles)

	// Changed files summary.
	parts := []string{}
	if report.Summary.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", report.Summary.Added))
	}
	if report.Summary.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", report.Summary.Modified))
	}
	if report.Summary.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", report.Summary.Deleted))
	}
	if len(parts) > 0 {
		fmt.Fprintf(&b, "Changed: %s\n", strings.Join(parts, ", "))
	}

	// High-risk files.
	if report.Summary.HighRiskFiles > 0 {
		var highRisk []string
		for _, f := range report.ChangedFiles {
			if f.BlastRadius != nil && f.BlastRadius.Level == "high" {
				highRisk = append(highRisk, f.File)
			}
		}
		if len(highRisk) > 0 {
			fmt.Fprintf(&b, "High risk: %s\n", strings.Join(highRisk, ", "))
		}
	}

	// Risk triage — single-line badge.
	if badge := FormatRiskBadge(report); badge != "" {
		fmt.Fprintf(&b, "%s\n", badge)
	}

	// Must read — inline.
	if len(report.MustRead) > 0 {
		var mrParts []string
		for _, entry := range report.MustRead {
			mrParts = append(mrParts, entry.File)
		}
		fmt.Fprintf(&b, "\nMust read (%d): %s\n", len(mrParts), strings.Join(mrParts, ", "))
	}

	// Likely modify — inline.
	lmParts := compactLikelyModify(report.LikelyModify)
	if len(lmParts) > 0 {
		fmt.Fprintf(&b, "Likely modify: %s\n", strings.Join(lmParts, ", "))
	}

	// Test gaps.
	if len(report.TestGaps) > 0 {
		fmt.Fprintf(&b, "Test gaps: %s\n", strings.Join(report.TestGaps, ", "))
	}

	// Coupling.
	if len(report.Coupling) > 0 {
		var cParts []string
		for _, c := range report.Coupling {
			arrow := "->"
			if c.Direction == "mutual" {
				arrow = "<->"
			}
			cParts = append(cParts, fmt.Sprintf("%s %s %s", c.FileA, arrow, c.FileB))
		}
		fmt.Fprintf(&b, "Coupling: %s\n", strings.Join(cParts, "; "))
	}

	// Warnings.
	var warnings []string
	if len(report.CircularDeps) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d circular deps", len(report.CircularDeps)))
	}
	if len(report.Hotspots) > 0 {
		warnings = append(warnings, fmt.Sprintf("hotspots: %s", strings.Join(report.Hotspots, ", ")))
	}
	if len(warnings) > 0 {
		fmt.Fprintf(&b, "Warnings: %s\n", strings.Join(warnings, "; "))
	}

	return b.String()
}

// compactMustReadReason generates a short reason string for a must-read entry.
func compactMustReadReason(entry model.MustReadEntry) string {
	var parts []string

	// Direction-based reason.
	switch entry.Direction {
	case "imports":
		if len(entry.Symbols) > 0 {
			parts = append(parts, "imports "+strings.Join(entry.Symbols, ", "))
		} else {
			parts = append(parts, "imports")
		}
	case "imported_by":
		parts = append(parts, "imported by")
	case "mutual":
		parts = append(parts, "mutual import")
	case "same_package":
		parts = append(parts, "same package")
	case "":
		// no direction
	default:
		parts = append(parts, entry.Direction)
	}

	// Tags.
	if entry.Role != "" && entry.Role != "unknown" {
		parts = append(parts, entry.Role)
	}
	if entry.Stable {
		parts = append(parts, "stable")
	}
	if entry.Circular {
		parts = append(parts, "circular")
	}

	if len(parts) == 0 {
		return ""
	}
	return "-> " + strings.Join(parts, ", ")
}

// compactLikelyModify builds inline entries like "file.py (high), other.py (medium)".
func compactLikelyModify(lm map[string][]model.LikelyModifyEntry) []string {
	// Sort keys for deterministic output.
	keys := make([]string, 0, len(lm))
	for k := range lm {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		for _, e := range lm[k] {
			parts = append(parts, fmt.Sprintf("%s (%s)", e.File, e.Confidence))
		}
	}
	return parts
}

// FormatRiskTriage produces a structured risk triage output for a change report.
func FormatRiskTriage(report *model.ChangeReport) string {
	if report == nil {
		return ""
	}

	var b strings.Builder

	fmt.Fprintf(&b, "RISK TRIAGE (%d files changed)\n", report.Summary.TotalFiles)
	b.WriteString("════════════════════════════════════════════════════════════\n")

	// Regression risk summary.
	if report.AggregateRisk != nil && report.AggregateRisk.RegressionRisk != "" {
		fmt.Fprintf(&b, "Regression risk: %s\n", report.AggregateRisk.RegressionRisk)
	}

	if report.RiskTriage == nil {
		return b.String()
	}

	// CRITICAL section — full details.
	if len(report.RiskTriage.Critical) > 0 {
		fmt.Fprintf(&b, "\nCRITICAL (%d file%s):\n", len(report.RiskTriage.Critical), pluralS(len(report.RiskTriage.Critical)))
		for _, file := range report.RiskTriage.Critical {
			cf := findChangedFile(report, file)
			if cf != nil {
				fmt.Fprintf(&b, "  %-40s score: %d\n", file, cf.RiskScore)
				if cf.RiskNarrative != "" {
					fmt.Fprintf(&b, "    %s\n", cf.RiskNarrative)
				}
			}
		}
	}

	// TEST section — with scores.
	if len(report.RiskTriage.Test) > 0 {
		fmt.Fprintf(&b, "\nTEST (%d file%s):\n", len(report.RiskTriage.Test), pluralS(len(report.RiskTriage.Test)))
		for _, file := range report.RiskTriage.Test {
			cf := findChangedFile(report, file)
			if cf != nil {
				fmt.Fprintf(&b, "  %-40s score: %d\n", file, cf.RiskScore)
				if cf.RiskNarrative != "" {
					fmt.Fprintf(&b, "    %s\n", cf.RiskNarrative)
				}
			}
		}
	}

	// REVIEW section — listed with scores.
	if len(report.RiskTriage.Review) > 0 {
		fmt.Fprintf(&b, "\nREVIEW (%d file%s):\n", len(report.RiskTriage.Review), pluralS(len(report.RiskTriage.Review)))
		for _, file := range report.RiskTriage.Review {
			cf := findChangedFile(report, file)
			if cf != nil {
				fmt.Fprintf(&b, "  %-40s score: %d\n", file, cf.RiskScore)
			}
		}
	}

	// SAFE section — count only.
	if len(report.RiskTriage.Safe) > 0 {
		fmt.Fprintf(&b, "\nSAFE (%d file%s)\n", len(report.RiskTriage.Safe), pluralS(len(report.RiskTriage.Safe)))
	}

	return b.String()
}

// FormatRiskBadge produces a human-readable risk summary for CI output.
// Designed to be understood without knowledge of the scoring system.
func FormatRiskBadge(report *model.ChangeReport) string {
	if report == nil || report.AggregateRisk == nil || report.RiskTriage == nil {
		return ""
	}

	score := report.AggregateRisk.Score
	triage := report.RiskTriage
	total := len(triage.Critical) + len(triage.Test) + len(triage.Review) + len(triage.Safe)

	// Risk level label.
	var level string
	switch {
	case score <= 20:
		level = "LOW RISK"
	case score <= 50:
		level = "MODERATE RISK"
	case score <= 75:
		level = "HIGH RISK"
	default:
		level = "CRITICAL RISK"
	}

	// Build the actionable summary, naming the top-risk files.
	var detail string
	switch {
	case len(triage.Critical) > 0:
		names := topFileNames(report, triage.Critical, 3)
		detail = fmt.Sprintf("%d file%s need%s careful review: %s",
			len(triage.Critical), pluralS(len(triage.Critical)),
			verbS(len(triage.Critical)), names)
	case len(triage.Test) > 0:
		names := topFileNames(report, triage.Test, 3)
		detail = fmt.Sprintf("%d file%s should be tested before merging: %s",
			len(triage.Test), pluralS(len(triage.Test)), names)
	case len(triage.Review) > 0:
		names := topFileNames(report, triage.Review, 3)
		detail = fmt.Sprintf("%d to review (%s), %d safe",
			len(triage.Review), names, len(triage.Safe))
	default:
		detail = fmt.Sprintf("all %d files are low risk", total)
	}

	return fmt.Sprintf("%s (%d files) — %s", level, total, detail)
}

// topFileNames returns disambiguated names of the highest-risk files from a tier list,
// sorted by risk score descending. Shows up to maxNames, with "+N more" if truncated.
func topFileNames(report *model.ChangeReport, files []string, maxNames int) string {
	// Sort by risk score descending.
	type scored struct {
		fullPath string
		score    int
	}
	var items []scored
	for _, f := range files {
		cf := findChangedFile(report, f)
		s := 0
		if cf != nil {
			s = cf.RiskScore
		}
		items = append(items, scored{fullPath: f, score: s})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	// Collect the top N paths for disambiguation.
	var topPaths []string
	for i, item := range items {
		if i >= maxNames {
			break
		}
		topPaths = append(topPaths, item.fullPath)
	}

	names := shortestUniqueNames(topPaths)
	result := strings.Join(names, ", ")
	if len(files) > maxNames {
		result += fmt.Sprintf(" +%d more", len(files)-maxNames)
	}
	return result
}

// shortestUniqueNames returns the shortest path suffix for each file that
// disambiguates it from the others. If all basenames are unique, returns basenames.
// If duplicates exist, adds parent directories until names are unique.
// Example: ["schema/change.go", "internal/model/change.go"] → ["schema/change.go", "model/change.go"]
func shortestUniqueNames(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	// Start with basenames.
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = path.Base(p)
	}

	// Check for duplicates and resolve by adding parent path segments.
	for depth := 1; depth < 10; depth++ {
		dupes := findDuplicates(names)
		if len(dupes) == 0 {
			break
		}
		for i := 0; i < len(names) && i < len(paths); i++ {
			if !dupes[names[i]] {
				continue
			}
			// Add one more parent directory segment.
			names[i] = pathSuffix(paths[i], depth+1)
		}
	}

	return names
}

// findDuplicates returns a set of names that appear more than once.
func findDuplicates(names []string) map[string]bool {
	counts := make(map[string]int, len(names))
	for _, n := range names {
		counts[n]++
	}
	dupes := make(map[string]bool)
	for n, c := range counts {
		if c > 1 {
			dupes[n] = true
		}
	}
	return dupes
}

// pathSuffix returns the last n segments of a path.
// pathSuffix("internal/model/change.go", 2) → "model/change.go"
func pathSuffix(p string, n int) string {
	parts := strings.Split(p, "/")
	if n >= len(parts) {
		return p
	}
	return strings.Join(parts[len(parts)-n:], "/")
}

// verbS returns "s" for count == 1 (third-person singular), "" otherwise.
// "1 file needs" vs "2 files need".
func verbS(n int) string {
	if n == 1 {
		return "s"
	}
	return ""
}

// findChangedFile returns the ChangedFile for a given path, or nil.
func findChangedFile(report *model.ChangeReport, path string) *model.ChangedFile {
	for i := range report.ChangedFiles {
		if report.ChangedFiles[i].File == path {
			return &report.ChangedFiles[i]
		}
	}
	return nil
}

// pluralS returns "s" for counts != 1, empty string for 1.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// compactWarnings collects warning-worthy conditions.
func compactWarnings(output *model.AnalysisOutput) []string {
	var warnings []string

	if len(output.CircularDeps) > 0 {
		for _, cycle := range output.CircularDeps {
			warnings = append(warnings, "circular dep: "+strings.Join(cycle, " -> "))
		}
	}

	if len(output.Hotspots) > 0 {
		warnings = append(warnings, "hotspots: "+strings.Join(output.Hotspots, ", "))
	}

	if output.ConfidenceNote != "" {
		warnings = append(warnings, output.ConfidenceNote)
	}

	return warnings
}
