// Package analyzer implements the context analysis engine.
// It traverses the dependency graph, scores candidates, and produces
// a ranked context bundle per ADR-0007 and ADR-0009.
package analyzer

import (
	"fmt"
	"math"
	"path"
	"sort"
	"strings"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/model"
)

// Schema and threshold constants.
const (
	// SchemaVersion is the current output schema version for analysis bundles.
	SchemaVersion = "3.2"

	// minStableThreshold is the floor for the adaptive stable-file threshold.
	minStableThreshold = 5

	// hotspotChurnThreshold is the normalized churn level (0.0–1.0) above which
	// a file is considered high-churn for hotspot detection.
	hotspotChurnThreshold = 0.5

	// fragilityEscalationThreshold is the instability metric level above which
	// fragility is appended to the blast radius detail string.
	fragilityEscalationThreshold = 0.7

	// hiddenCouplingMinFreq is the minimum co-change frequency for a file
	// to be surfaced as a hidden coupling partner.
	hiddenCouplingMinFreq = 3

	// maxHiddenCouplings caps hidden coupling entries per analysis.
	maxHiddenCouplings = 5

	// coChangeWindowDays is the lookback window (in days) for co-change partner queries.
	coChangeWindowDays = 90

	// subjectScore is the fixed score assigned to the subject file itself.
	subjectScore = 100.0

	// maxSiblingCandidates is the maximum number of same-package siblings to add as candidates.
	maxSiblingCandidates = 10

	// maxPackageSize is the threshold below which a package is considered "small"
	// for same-package coupling heuristics.
	maxPackageSize = 20

	// rustFallbackTestCount is the max integration tests to add when no name-matched tests exist.
	rustFallbackTestCount = 3
)

// Config configures the analyzer.
type Config struct {
	RepoConfig      *config.Config
	StableThreshold int    // Files with indegree >= this are marked stable. 0 = use adaptive default.
	OmitExternal    bool   // When true, omit external dependencies from output.
	Caps            Caps   // Output section caps. Zero values use defaults.
	Signatures      bool   // When true, include code signatures for must_read symbols.
	RepoRoot        string // Repository root path (required for signatures to read files).
}

// Analyzer orchestrates graph traversal, scoring, categorization, and explanation.
type Analyzer struct {
	idx     *db.Index
	cfg     Config
	repoCfg *config.Config
}

// New creates an Analyzer.
func New(idx *db.Index, cfg Config) *Analyzer {
	repoCfg := cfg.RepoConfig
	if repoCfg == nil {
		repoCfg = config.Empty()
	}
	return &Analyzer{idx: idx, cfg: cfg, repoCfg: repoCfg}
}

// Analyze produces a context bundle for the given file path.
// The filePath must be repo-relative and present in the index.
func (a *Analyzer) Analyze(filePath string) (*model.AnalysisOutput, error) {
	// Verify the file exists in the index.
	exists, err := a.idx.FileExists(filePath)
	if err != nil {
		return nil, fmt.Errorf("checking file existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("file %q not found in index — run `contextception index` first", filePath)
	}

	// Collect candidates via graph traversal.
	candidates, err := collectCandidates(a.idx, filePath, a.cfg.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("collecting candidates: %w", err)
	}

	// Get max indegree for normalization.
	maxIndegree, err := a.idx.GetMaxIndegree()
	if err != nil {
		return nil, fmt.Errorf("getting max indegree: %w", err)
	}

	// Score candidates.
	scored := scoreAll(candidates, maxIndegree)

	// Build scored candidate lookup for stable flag.
	scoreLookup := make(map[string]*scoredCandidate, len(scored))
	for i := range scored {
		scoreLookup[scored[i].Path] = &scored[i]
	}

	// Determine stable threshold: adaptive based on repo size, or CLI override.
	stableThresh := maxIndegree / 2
	if stableThresh < minStableThreshold {
		stableThresh = 5
	}
	if a.cfg.StableThreshold > 0 {
		stableThresh = a.cfg.StableThreshold
	}

	// Categorize into must_read, likely_modify, related, tests.
	cats := categorize(scored, filePath, a.repoCfg, a.cfg.Caps, stableThresh)

	// For Go source files: query package-level importer count for blast_radius.
	// With maxFilesPerPackage=1, non-representative files have near-zero file-level
	// importers even when their package is heavily imported.
	if strings.HasSuffix(filePath, ".go") && !strings.HasSuffix(filePath, "_test.go") {
		pkgDir := path.Dir(filePath)
		if pkgCount, err := a.idx.GetPackageImporterCount(pkgDir); err == nil {
			cats.PackageImporterCount = pkgCount
		}
	}

	// Topo sort must_read (leaf deps first).
	sortedMustRead := topoSortMustRead(a.idx, cats.MustRead, scoreLookup)

	// Get symbols: imported names from subject → must_read files.
	// Errors are intentionally ignored: symbol data is optional enrichment,
	// and some repos may not have imported_names data indexed.
	symbolMap, _ := a.idx.GetImportedNamesForSubject(filePath)
	// Get reverse symbols: what symbols do importers use from subject?
	reverseSymbolMap, _ := a.idx.GetImporterSymbols(filePath)

	// Build []MustReadEntry with symbols, stable flag, and direction.
	mustReadEntries := make([]model.MustReadEntry, len(sortedMustRead))
	for i, p := range sortedMustRead {
		entry := model.MustReadEntry{File: p}
		if syms, ok := symbolMap[p]; ok && len(syms) > 0 {
			entry.Symbols = syms
		} else if syms, ok := reverseSymbolMap[p]; ok && len(syms) > 0 {
			entry.Symbols = syms
		}
		if sc, ok := scoreLookup[p]; ok {
			if sc.Signals.Indegree >= stableThresh {
				entry.Stable = true
			}
			switch {
			case sc.IsImport && sc.IsImporter:
				entry.Direction = "mutual"
			case sc.IsImport:
				entry.Direction = "imports"
			case sc.IsImporter:
				entry.Direction = "imported_by"
			case sc.IsSamePackageSibling || sc.IsGoSamePackage || sc.IsJavaSamePackage || sc.IsRustSameModule:
				entry.Direction = "same_package"
			}
			entry.Role = classify.ClassifyRole(
				p, sc.Signals.Indegree, sc.Signals.Outdegree,
				sc.Signals.IsEntrypoint, sc.Signals.IsUtility, stableThresh,
			)
		}
		mustReadEntries[i] = entry
	}

	// Enrich must_read entries with code signatures if requested.
	if a.cfg.Signatures && a.cfg.RepoRoot != "" {
		refs := make([]MustReadEntryRef, len(mustReadEntries))
		for i := range mustReadEntries {
			refs[i] = MustReadEntryRef{
				File:        mustReadEntries[i].File,
				Symbols:     mustReadEntries[i].Symbols,
				Definitions: &mustReadEntries[i].Definitions,
			}
		}
		enrichDefinitions(a.cfg.RepoRoot, refs)
	}

	// Enrich likely_modify candidates with symbols and role.
	for i := range cats.LikelyModify {
		lm := &cats.LikelyModify[i]
		if syms, ok := symbolMap[lm.path]; ok && len(syms) > 0 {
			lm.symbols = syms // subject imports this file
		} else if syms, ok := reverseSymbolMap[lm.path]; ok && len(syms) > 0 {
			lm.symbols = syms // this file imports subject
		}
		if sc, ok := scoreLookup[lm.path]; ok {
			lm.role = classify.ClassifyRole(
				lm.path, sc.Signals.Indegree, sc.Signals.Outdegree,
				sc.Signals.IsEntrypoint, sc.Signals.IsUtility, stableThresh,
			)
		}
	}

	// Collect external dependencies for the subject file.
	var extSpecifiers []string
	if !a.cfg.OmitExternal {
		unresolvedRecords, err := a.idx.GetUnresolvedByFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("getting unresolved imports: %w", err)
		}
		extSpecifiers = make([]string, len(unresolvedRecords))
		for i, r := range unresolvedRecords {
			extSpecifiers[i] = r.Specifier
		}
		sort.Strings(extSpecifiers)
	}

	// Collect index stats (non-fatal if any fail).
	var stats *model.IndexStats
	if fc, err := a.idx.FileCount(); err == nil {
		stats = &model.IndexStats{TotalFiles: fc}
		if ec, err := a.idx.EdgeCount(); err == nil {
			stats.TotalEdges = ec
		}
		if uc, err := a.idx.UnresolvedCount(); err == nil {
			stats.UnresolvedCount = uc
		}
	}

	// Build []TestEntry from categorized test candidates.
	testEntries := make([]model.TestEntry, len(cats.Tests))
	for i, tc := range cats.Tests {
		testEntries[i] = model.TestEntry{File: tc.path, Direct: tc.direct}
	}

	// Compute bundle confidence from subject's resolution rate.
	resolved, notFound, _ := a.idx.GetSubjectResolutionRate(filePath)
	total := resolved + notFound
	var confidence float64
	if total == 0 {
		confidence = 1.0 // leaf file, nothing to miss
	} else {
		confidence = float64(resolved) / float64(total)
	}
	// Round to 2 decimal places for clean JSON.
	confidence = math.Round(confidence*100) / 100

	var confidenceNote string
	if notFound > 0 {
		confidenceNote = fmt.Sprintf(
			"%d of %d internal imports unresolved — must_read may be incomplete",
			notFound, total,
		)
	}

	output := &model.AnalysisOutput{
		SchemaVersion:  SchemaVersion,
		Subject:        filePath,
		Confidence:     confidence,
		ConfidenceNote: confidenceNote,
		External:       ensureSlice(extSpecifiers),
		MustRead:       mustReadEntries,
		LikelyModify:   groupLikelyModify(cats.LikelyModify),
		Tests:          ensureSlice(testEntries),
		Related:        groupRelated(cats.Related),
		BlastRadius:    computeBlastRadius(cats, len(testEntries), classify.IsTestFile(filePath), a.cfg.Caps.resolve()),
		Stats:          stats,
	}

	// --- Feature: Hotspot flagging ---
	// Collect files that are both high-churn AND high-indegree.
	var hotspots []string
	for _, sc := range scored {
		if sc.Path == filePath {
			continue
		}
		if sc.Churn >= hotspotChurnThreshold && sc.Signals.Indegree >= stableThresh {
			hotspots = append(hotspots, sc.Path)
		}
	}
	sort.Strings(hotspots)
	output.Hotspots = hotspots

	// --- Feature: Subject fragility (instability metric) ---
	// I = Ce/(Ca+Ce) where Ce=outdegree, Ca=indegree.
	if subjectSc, ok := scoreLookup[filePath]; ok {
		outdegree := float64(subjectSc.Signals.Outdegree)
		indegree := float64(subjectSc.Signals.Indegree)
		if outdegree+indegree > 0 {
			fragility := math.Round(outdegree/(outdegree+indegree)*100) / 100
			output.BlastRadius.Fragility = fragility
			if fragility >= fragilityEscalationThreshold {
				output.BlastRadius.Detail += fmt.Sprintf(", fragility %.2f", fragility)
			}
		}
	}

	// --- Feature: Hidden coupling detection ---
	// Surface co-change partners with no structural connection.
	candidateSet := make(map[string]bool, len(scored))
	for _, sc := range scored {
		candidateSet[sc.Path] = true
	}
	coChangePartners, _ := a.idx.GetCoChangePartners(filePath, coChangeWindowDays, 0)
	var hiddenCouplings []relatedCandidate
	for _, p := range coChangePartners {
		if candidateSet[p.Path] || p.Frequency < hiddenCouplingMinFreq {
			continue
		}
		exists, _ := a.idx.FileExists(p.Path)
		if !exists {
			continue
		}
		hiddenCouplings = append(hiddenCouplings, relatedCandidate{
			path:    p.Path,
			signals: []string{fmt.Sprintf("hidden_coupling:%d", p.Frequency)},
			score:   float64(p.Frequency),
		})
		if len(hiddenCouplings) >= maxHiddenCouplings {
			break
		}
	}
	if len(hiddenCouplings) > 0 {
		grouped := groupRelated(hiddenCouplings)
		for key, entries := range grouped {
			output.Related[key] = append(output.Related[key], entries...)
		}
	}

	// --- Feature: Circular dependency detection ---
	candidatePaths := make([]string, len(scored))
	for i, sc := range scored {
		candidatePaths[i] = sc.Path
	}
	cycles := detectCycles(a.idx, filePath, candidatePaths)
	if len(cycles) > 0 {
		output.CircularDeps = cycles
		participants := cycleParticipants(cycles)
		// Mark must_read entries that participate in cycles.
		for i := range output.MustRead {
			if participants[output.MustRead[i].File] {
				output.MustRead[i].Circular = true
			}
		}
		// Add "circular" signal to likely_modify entries for participants.
		for key, entries := range output.LikelyModify {
			for i := range entries {
				if participants[entries[i].File] {
					entries[i].Signals = append(entries[i].Signals, "circular")
				}
			}
			output.LikelyModify[key] = entries
		}
		// Append cycle count to blast_radius detail and escalate if needed.
		output.BlastRadius.Detail += fmt.Sprintf(", %d circular dep(s)", len(cycles))
		if output.BlastRadius.Level == "low" {
			output.BlastRadius.Level = "medium"
		}
	}

	if len(mustReadEntries) == 0 {
		output.MustReadNote = "this file has no internal dependencies"
	}

	if cats.TotalLikelyModify > len(cats.LikelyModify) {
		output.LikelyModifyNote = fmt.Sprintf("showing %d of %d candidates", len(cats.LikelyModify), cats.TotalLikelyModify)
	}

	resolvedCaps := a.cfg.Caps.resolve()
	if cats.TotalRelated >= resolvedCaps.MaxRelated {
		output.RelatedNote = fmt.Sprintf("showing %d of %d candidates", len(cats.Related), cats.TotalRelated)
	}

	if len(cats.Tests) == 0 && !classify.IsTestFile(filePath) {
		if cats.TestCandidateCount == 0 {
			output.TestsNote = "no test files found in nearby directories"
		} else {
			output.TestsNote = fmt.Sprintf(
				"%d test files nearby but none directly test this file",
				cats.TestCandidateCount,
			)
		}
	} else if cats.TotalTests > len(cats.Tests) {
		output.TestsNote = fmt.Sprintf("showing %d of %d matching test files", len(cats.Tests), cats.TotalTests)
	}

	return output, nil
}

// AnalyzeMulti produces a merged context bundle for multiple files.
// It calls Analyze() for each file and merges the results: deduped must_read,
// combined likely_modify, tests, related, and conservative blast_radius.
func (a *Analyzer) AnalyzeMulti(filePaths []string) (*model.AnalysisOutput, error) {
	if len(filePaths) == 0 {
		return nil, fmt.Errorf("no files provided")
	}
	if len(filePaths) == 1 {
		return a.Analyze(filePaths[0])
	}

	subjectSet := make(map[string]bool, len(filePaths))
	for _, fp := range filePaths {
		subjectSet[fp] = true
	}

	// Analyze each file independently.
	var outputs []*model.AnalysisOutput
	for _, fp := range filePaths {
		out, err := a.Analyze(fp)
		if err != nil {
			return nil, fmt.Errorf("analyzing %q: %w", fp, err)
		}
		outputs = append(outputs, out)
	}

	// Merge must_read: union, dedup, exclude subjects, preserve order by first appearance.
	seen := make(map[string]bool)
	var mergedMustRead []model.MustReadEntry
	for _, out := range outputs {
		for _, entry := range out.MustRead {
			// MustReadEntry.File is always a full repo-relative path (not grouped).
			if subjectSet[entry.File] || seen[entry.File] {
				continue
			}
			seen[entry.File] = true
			mergedMustRead = append(mergedMustRead, entry)
		}
	}

	// Apply must_read cap.
	caps := a.cfg.Caps.resolve()
	if len(mergedMustRead) > caps.MaxMustRead {
		mergedMustRead = mergedMustRead[:caps.MaxMustRead]
	}

	// Merge likely_modify: union, dedup, exclude subjects.
	lmSeen := make(map[string]bool)
	mergedLM := make(map[string][]model.LikelyModifyEntry)
	for _, out := range outputs {
		for key, entries := range out.LikelyModify {
			for _, entry := range entries {
				fullPath := key + "/" + entry.File
				if key == "." {
					fullPath = entry.File
				}
				if subjectSet[fullPath] || lmSeen[fullPath] {
					continue
				}
				lmSeen[fullPath] = true
				mergedLM[key] = append(mergedLM[key], entry)
			}
		}
	}
	// Cap likely_modify after merge.
	mergedLM = capGrouped(mergedLM, caps.MaxLikelyModify)

	// Merge tests: union, dedup, direct wins over dependency.
	testSeen := make(map[string]bool)
	testDirect := make(map[string]bool)
	var mergedTests []model.TestEntry
	for _, out := range outputs {
		for _, t := range out.Tests {
			if testSeen[t.File] {
				if t.Direct && !testDirect[t.File] {
					// Upgrade to direct.
					testDirect[t.File] = true
					for i := range mergedTests {
						if mergedTests[i].File == t.File {
							mergedTests[i].Direct = true
							break
						}
					}
				}
				continue
			}
			testSeen[t.File] = true
			testDirect[t.File] = t.Direct
			mergedTests = append(mergedTests, t)
		}
	}
	if len(mergedTests) > caps.MaxTests {
		mergedTests = mergedTests[:caps.MaxTests]
	}

	// Merge related: union, dedup, exclude subjects and must_read.
	relSeen := make(map[string]bool)
	mergedRel := make(map[string][]model.RelatedEntry)
	for _, out := range outputs {
		for key, entries := range out.Related {
			for _, entry := range entries {
				fullPath := key + "/" + entry.File
				if key == "." {
					fullPath = entry.File
				}
				if subjectSet[fullPath] || relSeen[fullPath] || seen[fullPath] {
					continue
				}
				relSeen[fullPath] = true
				mergedRel[key] = append(mergedRel[key], entry)
			}
		}
	}
	// Cap related after merge.
	mergedRel = capGrouped(mergedRel, caps.MaxRelated)

	// Merge external: union, dedup.
	extSeen := make(map[string]bool)
	var mergedExt []string
	for _, out := range outputs {
		for _, ext := range out.External {
			if !extSeen[ext] {
				extSeen[ext] = true
				mergedExt = append(mergedExt, ext)
			}
		}
	}
	sort.Strings(mergedExt)

	// Blast radius: take the highest level.
	mergedBR := mergeBlastRadius(outputs)

	// Confidence: minimum across all subjects (most conservative).
	minConfidence := 1.0
	var confidenceNotes []string
	for _, out := range outputs {
		if out.Confidence < minConfidence {
			minConfidence = out.Confidence
		}
		if out.ConfidenceNote != "" {
			confidenceNotes = append(confidenceNotes, out.ConfidenceNote)
		}
	}
	mergedConfNote := ""
	if len(confidenceNotes) > 0 {
		mergedConfNote = strings.Join(confidenceNotes, "; ")
	}

	// Stats: use first (all come from same index).
	var stats *model.IndexStats
	if outputs[0].Stats != nil {
		stats = outputs[0].Stats
	}

	subject := strings.Join(filePaths, ", ")

	output := &model.AnalysisOutput{
		SchemaVersion:  SchemaVersion,
		Subject:        subject,
		Confidence:     minConfidence,
		ConfidenceNote: mergedConfNote,
		External:       ensureSlice(mergedExt),
		MustRead:       mergedMustRead,
		LikelyModify:   mergedLM,
		Tests:          ensureSlice(mergedTests),
		Related:        mergedRel,
		BlastRadius:    mergedBR,
		Stats:          stats,
	}

	if len(mergedMustRead) == 0 {
		output.MustReadNote = "these files have no internal dependencies"
	}

	return output, nil
}

// mergeBlastRadius takes the highest blast radius level from multiple outputs.
func mergeBlastRadius(outputs []*model.AnalysisOutput) *model.BlastRadius {
	levelRank := map[string]int{"low": 0, "medium": 1, "high": 2}
	best := &model.BlastRadius{Level: "low", Detail: "merged from multi-file analysis"}
	for _, out := range outputs {
		if out.BlastRadius != nil {
			if levelRank[out.BlastRadius.Level] > levelRank[best.Level] {
				best.Level = out.BlastRadius.Level
			}
		}
	}
	return best
}

// ensureSlice returns an empty slice instead of nil, ensuring JSON serialization
// produces [] rather than null.
func ensureSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// computeBlastRadius determines the blast radius level and detail string.
// Uses reverse importer count (not totalRelated which includes 2-hop neighbors)
// and overrides to "low" when the subject is itself a test file.
// Distance-2 LOW candidates are excluded from the "high" threshold to prevent
// co-change noise in monorepos from inflating blast_radius for structurally
// contained files.
func computeBlastRadius(cats categories, testCount int, subjectIsTest bool, caps Caps) *model.BlastRadius {
	totalLM := cats.TotalLikelyModify
	shownLM := len(cats.LikelyModify)
	totalRel := cats.TotalRelated
	shownRel := len(cats.Related)
	importers := cats.ReverseImporterCount

	// For Go files: use MAX of file-level and package-level importer counts.
	// With maxFilesPerPackage=1, non-representative files have near-zero file-level
	// importers even when their package is heavily imported.
	if cats.PackageImporterCount > importers {
		importers = cats.PackageImporterCount
	}

	// Discount distance-2 LOW candidates for the "high" threshold.
	// They represent real historical coupling but not structural impact.
	effectiveLM := totalLM - cats.LikelyModifyD2LowCount

	var level string
	switch {
	case subjectIsTest:
		level = "low"
	case effectiveLM >= caps.MaxLikelyModify || importers >= highFanInThreshold:
		level = "high"
	case totalLM >= 5 || importers >= 5:
		level = "medium"
	default:
		level = "low"
	}

	detail := fmt.Sprintf("%d of %d likely_modify, %d of %d related, %d tests",
		shownLM, totalLM, shownRel, totalRel, testCount)

	return &model.BlastRadius{Level: level, Detail: detail}
}

// topoSortMustRead sorts must_read paths so foundational dependencies come first.
// Edge direction: if A imports B, B comes before A.
// Tiebreak: relevance score descending (primary), then original position (secondary).
// This prevents low-relevance zero-dep utility packages from being over-prioritized.
// Handles cycles by appending remaining nodes in original order.
func topoSortMustRead(idx *db.Index, paths []string, scoreLookup map[string]*scoredCandidate) []string {
	if len(paths) <= 1 {
		return paths
	}

	edges, err := idx.GetEdgesAmong(paths)
	if err != nil {
		return paths // graceful fallback
	}

	// Build adjacency list and in-degree map among the must_read set.
	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	// Edge: src imports dst → dst should come before src.
	// So we model dst→src edges for topo sort.
	inDegree := make(map[string]int, len(paths))
	adj := make(map[string][]string, len(paths))
	for _, p := range paths {
		inDegree[p] = 0
	}

	for _, e := range edges {
		// e.Src imports e.Dst → e.Dst comes before e.Src
		// Model as edge from e.Dst to e.Src
		adj[e.Dst] = append(adj[e.Dst], e.Src)
		inDegree[e.Src]++
	}

	// Kahn's algorithm.
	// Tiebreak: relevance score descending (primary), original position (secondary).
	posMap := make(map[string]int, len(paths))
	for i, p := range paths {
		posMap[p] = i
	}

	sortQueue := func(q []string) {
		sort.Slice(q, func(i, j int) bool {
			scoreI, scoreJ := 0.0, 0.0
			outI, outJ := 0, 0
			if sc := scoreLookup[q[i]]; sc != nil {
				scoreI = sc.Score
				outI = sc.Signals.Outdegree
			}
			if sc := scoreLookup[q[j]]; sc != nil {
				scoreJ = sc.Score
				outJ = sc.Signals.Outdegree
			}
			if scoreI != scoreJ {
				return scoreI > scoreJ
			}
			if outI != outJ {
				return outI < outJ // lower outdegree first (foundations before orchestrators)
			}
			return posMap[q[i]] < posMap[q[j]]
		})
	}

	// Collect nodes with in-degree 0.
	var queue []string
	for _, p := range paths {
		if inDegree[p] == 0 {
			queue = append(queue, p)
		}
	}
	sortQueue(queue)

	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// Process neighbors, collect new zero-in-degree nodes.
		var newZero []string
		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				newZero = append(newZero, neighbor)
			}
		}
		queue = append(queue, newZero...)
		sortQueue(queue)
	}

	// Handle cycles: append remaining nodes sorted by outdegree ascending
	// (foundations before orchestrators), then score descending, then original position.
	if len(result) < len(paths) {
		inResult := make(map[string]bool, len(result))
		for _, p := range result {
			inResult[p] = true
		}
		var remaining []string
		for _, p := range paths {
			if !inResult[p] {
				remaining = append(remaining, p)
			}
		}
		sort.Slice(remaining, func(i, j int) bool {
			outI, outJ := 0, 0
			scoreI, scoreJ := 0.0, 0.0
			if sc := scoreLookup[remaining[i]]; sc != nil {
				outI = sc.Signals.Outdegree
				scoreI = sc.Score
			}
			if sc := scoreLookup[remaining[j]]; sc != nil {
				outJ = sc.Signals.Outdegree
				scoreJ = sc.Score
			}
			if outI != outJ {
				return outI < outJ // lower outdegree first (foundations before orchestrators)
			}
			if scoreI != scoreJ {
				return scoreI > scoreJ
			}
			return posMap[remaining[i]] < posMap[remaining[j]]
		})
		result = append(result, remaining...)
	}

	return result
}

// capGrouped caps a grouped map to maxEntries total entries.
// Uses sorted keys for deterministic truncation order.
func capGrouped[T any](m map[string][]T, maxEntries int) map[string][]T {
	total := 0
	for _, entries := range m {
		total += len(entries)
	}
	if total <= maxEntries {
		return m
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make(map[string][]T)
	count := 0
	for _, k := range keys {
		for _, e := range m[k] {
			if count >= maxEntries {
				return result
			}
			result[k] = append(result[k], e)
			count++
		}
	}
	return result
}
