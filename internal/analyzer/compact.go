package analyzer

import (
	"fmt"
	"strings"

	"github.com/kehoej/contextception/internal/model"
)

// FormatCompact produces a token-optimized text summary of an analysis output.
// Targets ~75% fewer tokens than full JSON while preserving all essential information.
func FormatCompact(output *model.AnalysisOutput) string {
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
	var parts []string
	for _, entries := range lm {
		for _, e := range entries {
			parts = append(parts, fmt.Sprintf("%s (%s)", e.File, e.Confidence))
		}
	}
	return parts
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
