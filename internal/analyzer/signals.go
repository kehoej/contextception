package analyzer

import (
	"fmt"

	"github.com/kehoej/contextception/internal/config"
)

// likely_modify thresholds.
const (
	likelyModifyCoChangeThreshold     = 3
	likelyModifyTightCouplingMaxFanIn = 2
	likelyModifyConsumerCoChangeMin   = 2
	likelyModifyLowCoChangeMin        = 2 // Condition A: near-threshold co-change
	likelyModifyLowModFanInMax        = 4 // Condition C: moderate fan-in ceiling
)

// likelyModifyConfidence returns "high" or "medium" for a candidate that
// qualifies as likely_modify. Must only be called when isLikelyModify is true.
func likelyModifyConfidence(sc *scoredCandidate) string {
	if sc.CoChangeFreq >= 5 {
		return "high"
	}
	if sc.IsImport && sc.Signals.Indegree <= likelyModifyTightCouplingMaxFanIn && sc.CoChangeFreq >= 1 {
		return "high"
	}
	if sc.IsImporter && sc.CoChangeFreq >= 3 {
		return "high"
	}
	// Go same-package file with prefix match: structural evidence of tight coupling.
	if sc.IsGoSamePackage && sc.HasPrefixMatch {
		return "high"
	}
	// Go same-package file with co-change evidence: structural + behavioral = high.
	if sc.IsGoSamePackage && sc.CoChangeFreq >= 1 {
		return "high"
	}
	// Java same-package file with class prefix match: structural evidence of tight coupling.
	if sc.IsJavaSamePackage && sc.HasJavaClassPrefix {
		return "high"
	}
	// Java same-package file with co-change evidence: structural + behavioral = high.
	if sc.IsJavaSamePackage && sc.CoChangeFreq >= 1 {
		return "high"
	}
	// Rust same-module file with prefix match: structural evidence of tight coupling.
	if sc.IsRustSameModule && sc.HasRustModulePrefix {
		return "high"
	}
	// Rust same-module file with co-change evidence: structural + behavioral = high.
	if sc.IsRustSameModule && sc.CoChangeFreq >= 1 {
		return "high"
	}
	return "medium"
}

// isLikelyModify returns true if the candidate is likely to need modification
// when the subject changes.
func isLikelyModify(sc *scoredCandidate) bool {
	if sc.CoChangeFreq >= likelyModifyCoChangeThreshold {
		return true
	}
	if sc.IsImport && sc.Signals.Indegree <= likelyModifyTightCouplingMaxFanIn {
		return true
	}
	if sc.IsImporter && sc.CoChangeFreq >= likelyModifyConsumerCoChangeMin {
		return true
	}
	// Go same-package: implicit package visibility creates structural coupling.
	// Only qualify if there's evidence of tight coupling (prefix match, small package).
	if sc.IsGoSamePackage && sc.Distance == 1 && (sc.HasPrefixMatch || sc.IsSmallGoPackage) {
		return true
	}
	// Java same-package: implicit package visibility creates structural coupling.
	// Only qualify if there's evidence of tight coupling (class prefix match, small package).
	if sc.IsJavaSamePackage && sc.Distance == 1 && (sc.HasJavaClassPrefix || sc.IsSmallJavaPackage) {
		return true
	}
	// Rust same-module: implicit module visibility creates structural coupling.
	// Only qualify if there's evidence of tight coupling (prefix match, small module).
	if sc.IsRustSameModule && sc.Distance == 1 && (sc.HasRustModulePrefix || sc.IsSmallRustModule) {
		return true
	}
	return false
}

// isLikelyModifyLow returns true if the candidate has partial evidence of
// co-modification but fails the isLikelyModify gate. This surfaces near-miss
// files as "low" confidence rather than making them invisible.
func isLikelyModifyLow(sc *scoredCandidate) bool {
	// A: near-threshold co-change (the primary motivating case).
	if sc.CoChangeFreq == likelyModifyLowCoChangeMin && sc.Distance <= 2 {
		return true
	}
	// B: importer with a single co-change.
	if sc.IsImporter && sc.CoChangeFreq == 1 {
		return true
	}
	// C: import with moderate fan-in and at least 1 co-change.
	if sc.IsImport && sc.Signals.Indegree > likelyModifyTightCouplingMaxFanIn &&
		sc.Signals.Indegree <= likelyModifyLowModFanInMax && sc.CoChangeFreq >= 1 {
		return true
	}
	// D: direct importer with zero co-change history — purely structural signal.
	if sc.IsImporter && sc.Distance == 1 && sc.CoChangeFreq == 0 {
		return true
	}
	return false
}

// buildSignals returns compact signal strings for a scored candidate.
// Fixed order: primary relationship, then behavioral, then metadata.
func buildSignals(sc *scoredCandidate, cfg *config.Config, stableThresh int) []string {
	var signals []string

	// Primary relationship signals.
	if sc.IsImport {
		signals = append(signals, "imports")
	}
	if sc.IsImporter {
		signals = append(signals, "imported_by")
	}
	if sc.IsGoSamePackage || sc.IsSamePackageSibling || sc.IsJavaSamePackage || sc.IsRustSameModule {
		signals = append(signals, "same_package")
	}
	if sc.IsTransitiveCaller {
		signals = append(signals, "transitive_caller")
	} else if sc.Distance == 2 && !sc.IsImport && !sc.IsImporter && !sc.IsSamePackageSibling && !sc.IsGoSamePackage && !sc.IsJavaSamePackage && !sc.IsRustSameModule {
		signals = append(signals, "two_hop")
	}

	// Behavioral signals.
	if sc.CoChangeFreq > 0 {
		signals = append(signals, fmt.Sprintf("co_change:%d", sc.CoChangeFreq))
	}
	if sc.Churn >= hotspotChurnThreshold {
		signals = append(signals, "high_churn")
		if sc.Signals.Indegree >= stableThresh {
			signals = append(signals, "hotspot")
		}
	}

	// Metadata signals.
	if sc.Signals.IsEntrypoint || cfg.IsConfigEntrypoint(sc.Path) {
		signals = append(signals, "entrypoint")
	}

	return signals
}
