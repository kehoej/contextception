package analyzer

import (
	"strings"

	"github.com/kehoej/contextception/internal/model"
)

// splitPackage returns (group_key, relative_file) for a repo-relative path.
// packages/X/... and apps/X/... → key = first 2 path components.
// Otherwise → key = first component. Top-level files → key = ".".
func splitPackage(path string) (string, string) {
	parts := strings.SplitN(path, "/", 3)

	if len(parts) == 1 {
		// Top-level file.
		return ".", parts[0]
	}

	if (parts[0] == "packages" || parts[0] == "apps") && len(parts) >= 3 {
		// packages/X/rest or apps/X/rest → key = first 2 components.
		return parts[0] + "/" + parts[1], parts[2]
	}

	// key = first component, rest is the file.
	return parts[0], strings.Join(parts[1:], "/")
}

// groupLikelyModify groups likely_modify candidates by package key.
func groupLikelyModify(entries []likelyModifyCandidate) map[string][]model.LikelyModifyEntry {
	if len(entries) == 0 {
		return map[string][]model.LikelyModifyEntry{}
	}
	groups := make(map[string][]model.LikelyModifyEntry)
	for _, e := range entries {
		key, rel := splitPackage(e.path)
		groups[key] = append(groups[key], model.LikelyModifyEntry{
			File:       rel,
			Confidence: e.confidence,
			Signals:    e.signals,
			Symbols:    e.symbols,
			Role:       e.role,
		})
	}
	return groups
}

// groupRelated groups related candidates by package key.
func groupRelated(entries []relatedCandidate) map[string][]model.RelatedEntry {
	if len(entries) == 0 {
		return map[string][]model.RelatedEntry{}
	}
	groups := make(map[string][]model.RelatedEntry)
	for _, e := range entries {
		key, rel := splitPackage(e.path)
		groups[key] = append(groups[key], model.RelatedEntry{
			File:    rel,
			Signals: e.signals,
		})
	}
	return groups
}
