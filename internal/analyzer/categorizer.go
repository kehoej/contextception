package analyzer

import (
	"path"
	"sort"
	"strings"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/config"
)

// Default cap values.
const (
	defaultMaxMustRead     = 10
	defaultMaxRelated      = 10
	defaultMaxLikelyModify = 15
	defaultMaxTests        = 5
	maxDependencyTests     = 2
	highFanInThreshold     = 50
)

// Caps controls the maximum number of entries in each output section.
// Zero values use defaults.
type Caps struct {
	MaxMustRead     int    // default 10
	MaxRelated      int    // default 10
	MaxLikelyModify int    // default 15
	MaxTests        int    // default 5
	Mode            string // workflow mode: plan, implement, review (adjusts defaults)
	TokenBudget     int    // target token budget (overrides caps to fit)
}

// modePresets maps workflow mode names to their default caps.
var modePresets = map[string][4]int{
	"plan":      {15, 5, 3, 15}, // must_read, likely_modify, tests, related
	"implement": {10, 20, 5, 5},
	"review":    {5, 10, 10, 5},
}

// Token estimation constants (chars per token ≈ 4).
const (
	tokensPerMustRead     = 80
	tokensPerLikelyModify = 60
	tokensPerTest         = 30
	tokensPerRelated      = 40
	tokensOverhead        = 100
)

// resolve returns a Caps with all zero values replaced by defaults.
// Precedence: mode sets base caps → individual --max-* overrides → token budget constrains.
func (c Caps) resolve() Caps {
	// Step 1: Apply mode presets for any caps that are still zero.
	if preset, ok := modePresets[c.Mode]; ok {
		if c.MaxMustRead <= 0 {
			c.MaxMustRead = preset[0]
		}
		if c.MaxLikelyModify <= 0 {
			c.MaxLikelyModify = preset[1]
		}
		if c.MaxTests <= 0 {
			c.MaxTests = preset[2]
		}
		if c.MaxRelated <= 0 {
			c.MaxRelated = preset[3]
		}
	}

	// Step 2: Fill remaining zeros with defaults.
	if c.MaxMustRead <= 0 {
		c.MaxMustRead = defaultMaxMustRead
	}
	if c.MaxRelated <= 0 {
		c.MaxRelated = defaultMaxRelated
	}
	if c.MaxLikelyModify <= 0 {
		c.MaxLikelyModify = defaultMaxLikelyModify
	}
	if c.MaxTests <= 0 {
		c.MaxTests = defaultMaxTests
	}

	// Step 3: Apply token budget constraints.
	if c.TokenBudget > 0 {
		c = c.applyTokenBudget()
	}

	return c
}

// applyTokenBudget adjusts caps to fit within the target token budget.
// Budget allocation: must_read 40%, likely_modify 30%, related 20%, tests 10%.
func (c Caps) applyTokenBudget() Caps {
	available := c.TokenBudget - tokensOverhead
	if available <= 0 {
		available = 50 // minimum viable budget
	}

	mustReadTokens := available * 40 / 100
	likelyModifyTokens := available * 30 / 100
	relatedTokens := available * 20 / 100
	testTokens := available * 10 / 100

	mustReadCap := mustReadTokens / tokensPerMustRead
	likelyModifyCap := likelyModifyTokens / tokensPerLikelyModify
	testCap := testTokens / tokensPerTest
	relatedCap := relatedTokens / tokensPerRelated

	// Bound each cap: min 3, max = current cap * 3.
	c.MaxMustRead = boundCap(mustReadCap, 3, c.MaxMustRead*3)
	c.MaxLikelyModify = boundCap(likelyModifyCap, 3, c.MaxLikelyModify*3)
	c.MaxTests = boundCap(testCap, 3, c.MaxTests*3)
	c.MaxRelated = boundCap(relatedCap, 3, c.MaxRelated*3)

	return c
}

// boundCap returns v clamped to [min, max].
func boundCap(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// likelyModifyCandidate holds a candidate that qualifies for likely_modify.
type likelyModifyCandidate struct {
	path       string
	confidence string // "high", "medium", "low"
	signals    []string
	confRank   int      // 0=high, 1=medium, 2=low (for sorting)
	score      float64  // for tiebreaking within same confidence
	symbols    []string // imported symbol names connecting to subject
	role       string   // file role hint (barrel, foundation, etc.)
}

// relatedCandidate holds a candidate for the related section.
type relatedCandidate struct {
	path    string
	signals []string
	score   float64
}

// testCandidate holds a test file path and whether it directly tests the subject.
type testCandidate struct {
	path   string
	direct bool
}

type categories struct {
	MustRead               []string                // flat paths, subject excluded
	LikelyModify           []likelyModifyCandidate // with signals
	Related                []relatedCandidate      // with signals and score
	Tests                  []testCandidate         // test file paths with direct/dependency tier
	TotalTests             int                     // total test candidates before capping
	TotalLikelyModify      int                     // total likely_modify candidates before capping
	TotalRelated           int                     // total related candidates before capping
	ReverseImporterCount   int                     // count of distance-1 reverse importers (for blast_radius)
	PackageImporterCount   int                     // package-level reverse importers (Go only, for blast_radius)
	LikelyModifyD2LowCount int                     // count of distance-2 LOW likely_modify candidates (for blast_radius discount)
	TestCandidateCount     int                     // test files in candidate pool before filtering
}

// categorize assigns scored candidates into must_read, likely_modify, related, and tests.
func categorize(scored []scoredCandidate, subject string, cfg *config.Config, caps Caps, stableThresh int) categories {
	caps = caps.resolve()
	subjectIsTest := classify.IsTestFile(subject)

	// Build scored candidate lookup for signal generation and score access.
	scoreLookup := make(map[string]*scoredCandidate, len(scored))
	for i := range scored {
		scoreLookup[scored[i].Path] = &scored[i]
	}

	var mustRead []string
	var testCandidates []scoredCandidate
	var forwardImports []string            // distance-1, IsImport=true
	var samePackageSiblings []string       // distance-1, IsSamePackageSibling=true (Go only)
	var reverseOnly []string               // distance-1, IsImporter=true but NOT IsImport
	var twoHopCandidates []scoredCandidate // collected for tier-based filling

	for _, sc := range scored {
		if sc.Path == subject {
			continue
		}

		// Config-ignored: unconditionally skip.
		if cfg.IsIgnored(sc.Path) {
			continue
		}

		// Config-generated: skip.
		if cfg.IsGenerated(sc.Path) {
			continue
		}

		// Test files: route to test candidates (unless subject itself is a test file).
		if !subjectIsTest && classify.IsTestFile(sc.Path) {
			testCandidates = append(testCandidates, sc)
			continue
		}

		// Docs/examples/conftest go to ignore always.
		if classify.IsIgnorable(sc.Path) {
			continue
		}

		if sc.Distance <= 1 {
			if sc.IsImport {
				forwardImports = append(forwardImports, sc.Path)
			} else if sc.IsSamePackageSibling || sc.IsGoSamePackage || sc.IsJavaSamePackage || sc.IsRustSameModule {
				samePackageSiblings = append(samePackageSiblings, sc.Path)
			} else {
				reverseOnly = append(reverseOnly, sc.Path)
			}
		} else if sc.Distance == 2 {
			if classify.IsInitFile(sc.Path) && !classify.IsInitFile(subject) {
				continue
			}
			twoHopCandidates = append(twoHopCandidates, sc)
		}
	}

	// Unified allocation: forward imports fill must_read first (up to caps.MaxMustRead),
	// then Go same-package siblings, then reverse importers. Overflow goes to related.
	var overflow []string

	for _, p := range forwardImports {
		if len(mustRead) < caps.MaxMustRead {
			mustRead = append(mustRead, p)
		} else {
			overflow = append(overflow, p)
		}
	}

	for _, p := range samePackageSiblings {
		if len(mustRead) < caps.MaxMustRead {
			mustRead = append(mustRead, p)
		} else {
			overflow = append(overflow, p)
		}
	}

	highFanIn := len(reverseOnly) >= highFanInThreshold
	if highFanIn {
		// High fan-in: skip reverse importers in must_read entirely.
		overflow = append(overflow, reverseOnly...)
	} else {
		for _, p := range reverseOnly {
			if len(mustRead) < caps.MaxMustRead {
				mustRead = append(mustRead, p)
			} else {
				overflow = append(overflow, p)
			}
		}
	}

	// Tier-based 2-hop filling: co-change partners first, then structural-only.
	remaining := caps.MaxRelated
	var twoHopRelated []string
	// Tier 1: co-change partners.
	for _, sc := range twoHopCandidates {
		if remaining <= 0 {
			break
		}
		if sc.CoChangeFreq > 0 {
			twoHopRelated = append(twoHopRelated, sc.Path)
			remaining--
		}
	}
	// Tier 2: structural-only 2-hop neighbors fill remaining slots.
	for _, sc := range twoHopCandidates {
		if remaining <= 0 {
			break
		}
		if sc.CoChangeFreq == 0 {
			twoHopRelated = append(twoHopRelated, sc.Path)
			remaining--
		}
	}

	// Build the combined related list: overflow first (distance-1), then 2-hop.
	allRelatedPaths := append(overflow, twoHopRelated...)

	// --- likely_modify extraction ---
	// Scan all non-ignored candidates at distance <= 2.
	// Files qualifying for likely_modify are promoted out of related.
	likelyModifySet := make(map[string]bool)
	var likelyModifyCandidates []likelyModifyCandidate
	var d2LowCount int

	for _, sc := range scored {
		if sc.Path == subject || cfg.IsIgnored(sc.Path) {
			continue
		}
		if sc.Distance > 2 {
			continue
		}
		// Skip test files and ignorable files (docs, examples, conftest).
		if !subjectIsTest && classify.IsTestFile(sc.Path) {
			continue
		}
		if classify.IsIgnorable(sc.Path) {
			continue
		}
		scPtr := scoreLookup[sc.Path]
		if scPtr == nil {
			continue
		}

		var conf string
		if isLikelyModify(scPtr) {
			conf = likelyModifyConfidence(scPtr)
		} else if isLikelyModifyLow(scPtr) {
			conf = "low"
		}
		if conf == "" {
			continue
		}
		// Same-directory Python importers with zero co-change: co-location implies
		// tight coupling, stronger evidence than distant importers. Only boost
		// condition D (purely structural, no co-change) to avoid changing files
		// that already have co-change evidence for their tier assignment.
		if conf == "low" && strings.HasSuffix(subject, ".py") &&
			path.Dir(sc.Path) == path.Dir(subject) && scPtr.IsImporter &&
			sc.CoChangeFreq == 0 {
			conf = "medium"
		}

		// Mutual dependencies (both import and imported_by) have strong
		// structural coupling — bidirectionality is rare and significant.
		if conf == "low" && scPtr.IsImport && scPtr.IsImporter {
			conf = "medium"
		}

		if conf == "low" && sc.Distance >= 2 {
			d2LowCount++
		}

		signals := buildSignals(scPtr, cfg, stableThresh)
		// Python same-directory files are tightly coupled — surface this as a distinct signal.
		if strings.HasSuffix(subject, ".py") && path.Dir(sc.Path) == path.Dir(subject) {
			signals = append(signals, "same_directory")
		}
		confRank := 2
		if conf == "high" {
			confRank = 0
		} else if conf == "medium" {
			confRank = 1
		}

		likelyModifyCandidates = append(likelyModifyCandidates, likelyModifyCandidate{
			path:       sc.Path,
			confidence: conf,
			signals:    signals,
			confRank:   confRank,
			score:      sc.Score,
		})
		likelyModifySet[sc.Path] = true
	}

	// Sort likely_modify: by confidence priority, then score desc, then path.
	sort.Slice(likelyModifyCandidates, func(i, j int) bool {
		if likelyModifyCandidates[i].confRank != likelyModifyCandidates[j].confRank {
			return likelyModifyCandidates[i].confRank < likelyModifyCandidates[j].confRank
		}
		if likelyModifyCandidates[i].score != likelyModifyCandidates[j].score {
			return likelyModifyCandidates[i].score > likelyModifyCandidates[j].score
		}
		return likelyModifyCandidates[i].path < likelyModifyCandidates[j].path
	})

	// Cap likely_modify to prevent unbounded lists for central files.
	totalLikelyModify := len(likelyModifyCandidates)
	if len(likelyModifyCandidates) > caps.MaxLikelyModify {
		likelyModifyCandidates = likelyModifyCandidates[:caps.MaxLikelyModify]
	}

	// Rebuild set to only include candidates that survived the cap.
	// Capped-out candidates should remain eligible for related.
	likelyModifySet = make(map[string]bool, len(likelyModifyCandidates))
	for _, lm := range likelyModifyCandidates {
		likelyModifySet[lm.path] = true
	}

	// Build related: exclude files promoted to likely_modify. Sort by score desc.
	var relatedCandidates []relatedCandidate
	for _, p := range allRelatedPaths {
		if likelyModifySet[p] {
			continue // Promoted to likely_modify.
		}
		sc := scoreLookup[p]
		if sc == nil {
			continue
		}
		signals := buildSignals(sc, cfg, stableThresh)
		relatedCandidates = append(relatedCandidates, relatedCandidate{
			path:    p,
			signals: signals,
			score:   sc.Score,
		})
	}

	// Sort related by score desc, then path asc for determinism.
	sort.Slice(relatedCandidates, func(i, j int) bool {
		if relatedCandidates[i].score != relatedCandidates[j].score {
			return relatedCandidates[i].score > relatedCandidates[j].score
		}
		return relatedCandidates[i].path < relatedCandidates[j].path
	})

	// Cap related to keep output focused.
	totalRelated := len(relatedCandidates)
	if len(relatedCandidates) > caps.MaxRelated {
		relatedCandidates = relatedCandidates[:caps.MaxRelated]
	}

	// --- Test extraction ---
	// For Rust subjects, filter test candidates to same-crate AND module-adjacent.
	// Cross-crate test files appear via graph traversal through shared dependencies.
	// Within the same crate, tests.rs files test their specific module — a tests.rs
	// deep in a submodule tree is irrelevant to the subject unless it's nearby.
	if !subjectIsTest && strings.HasSuffix(subject, ".rs") {
		crateRoot := rustCrateRoot(subject)
		if crateRoot != "" {
			subjectDir := path.Dir(subject)
			var filteredTests []scoredCandidate
			for _, tc := range testCandidates {
				tcCrate := rustCrateRoot(tc.Path)
				// Must be same crate (or crate's integration tests/).
				isSameCrate := tcCrate == crateRoot
				var testsPrefix string
				if crateRoot == "." {
					testsPrefix = "tests/"
				} else {
					testsPrefix = crateRoot + "/tests/"
				}
				isIntegrationTest := strings.HasPrefix(tc.Path, testsPrefix)

				if !isSameCrate && !isIntegrationTest {
					continue // Cross-crate: always drop.
				}

				// Within same crate, tests.rs files are module-specific.
				// Only keep if the test is module-adjacent to the subject:
				// same directory or direct child directory.
				tcDir := path.Dir(tc.Path)
				if path.Base(tc.Path) == "tests.rs" && isSameCrate {
					adjacent := tcDir == subjectDir || // same dir
						path.Dir(tcDir) == subjectDir // direct child dir
					if !adjacent {
						continue // Unrelated submodule test.
					}
				}

				filteredTests = append(filteredTests, tc)
			}
			testCandidates = filteredTests
		}
	}

	tests, totalTests := filterTestCandidates(testCandidates, subject, mustRead, caps.MaxTests)

	// Rust inline test injection: files with #[cfg(test)] are direct tests
	// for their own module. The subject itself is always direct. Siblings
	// with inline tests are direct only when the subject lacks its own
	// inline tests; otherwise they become dependency tier (they test their
	// own module, not the subject).
	if !subjectIsTest && strings.HasSuffix(subject, ".rs") {
		existingTests := make(map[string]bool, len(tests))
		for _, t := range tests {
			existingTests[t.path] = true
		}
		// Check if subject has inline tests.
		subjectHasInlineTests := false
		for _, sc := range scored {
			if sc.Path == subject && sc.HasInlineTests {
				subjectHasInlineTests = true
				break
			}
		}
		for _, sc := range scored {
			if !sc.HasInlineTests || sc.Distance > 1 {
				continue
			}
			if existingTests[sc.Path] {
				continue
			}
			if len(tests) >= caps.MaxTests {
				break
			}
			// Subject itself is always direct.
			// Siblings: direct only if subject lacks own inline tests.
			isDirect := (sc.Path == subject) || !subjectHasInlineTests
			tests = append(tests, testCandidate{path: sc.Path, direct: isDirect})
			totalTests++
		}
	}

	return categories{
		MustRead:               mustRead,
		LikelyModify:           likelyModifyCandidates,
		Related:                relatedCandidates,
		Tests:                  tests,
		TotalTests:             totalTests,
		TotalLikelyModify:      totalLikelyModify,
		TotalRelated:           totalRelated,
		ReverseImporterCount:   len(reverseOnly),
		LikelyModifyD2LowCount: d2LowCount,
		TestCandidateCount:     len(testCandidates),
	}
}

// filterTestCandidates selects relevant test files from test candidates.
// Tests are split into two tiers:
//   - Direct tests: tests for the subject itself (stem match, co-located same dir or __tests__ with import/stem)
//   - Dependency tests: tests for must_read files (near must_read, stem match must_read, co-changes)
//
// Direct tests are prioritized over dependency tests, each sorted by score.
// Dependency tests are sub-capped at maxDependencyTests (2) to prevent saturation.
// Returns (selected test paths, total candidates before capping).
func filterTestCandidates(candidates []scoredCandidate, subject string, mustReadPaths []string, capTests int) ([]testCandidate, int) {
	subjectDir := path.Dir(subject)
	subjectStem := extractSubjectStem(path.Base(subject))

	// Build set of must_read directories for proximity check.
	mustReadDirs := make(map[string]bool, len(mustReadPaths))
	for _, p := range mustReadPaths {
		mustReadDirs[path.Dir(p)] = true
	}

	var direct []scoredCandidate
	var dependency []scoredCandidate

	for _, sc := range candidates {
		testDir := path.Dir(sc.Path)
		testStem := extractTestStem(path.Base(sc.Path))

		// Rust fallback: files in a tests/ directory may not follow test_/_ test naming.
		// Extract stem by stripping .rs extension (handles tests/foo.rs → stem "foo").
		if testStem == "" && strings.HasSuffix(sc.Path, ".rs") {
			if strings.HasPrefix(testDir, "tests") || strings.Contains(testDir, "/tests") {
				base := path.Base(sc.Path)
				testStem = strings.TrimSuffix(base, ".rs")
			}
		}

		// --- Direct test criteria ---
		sameDir := testDir == subjectDir
		testsSubdir := testDir == subjectDir+"/__tests__" ||
			strings.HasPrefix(testDir, subjectDir+"/__tests__/")
		importsSubject := sc.IsImporter
		subjectStemMatch := testStem != "" && subjectStem != "" && testStem == subjectStem

		// I-017: Cross-directory stem matches require the test to import the subject.
		// Without this, generic stems like "entity" or "config" match hundreds of
		// unrelated test files in large repos with repeated module names.
		if subjectStemMatch && !sameDir && !testsSubdir {
			subjectStemMatch = importsSubject
		}

		// I-015: __init__.py subjects match test_init.py when the test imports the subject.
		// The barrel stem suppression (extractSubjectStem returns "") prevents normal stem
		// matching, but test_init.py is the standard Python test pattern for __init__.py.
		// Requiring importsSubject prevents false positives from unrelated test_init.py files.
		// I-018: Additionally require directory correspondence — the test's parent directory
		// must match the subject's parent directory. Without this, high-fan-in __init__.py
		// files (e.g., light with 319 importers) match test_init.py in every component that
		// imports them, crowding out the correct test at tests/components/light/test_init.py.
		isInitPy := path.Base(subject) == "__init__.py"
		correspondingDir := sameDir || path.Base(testDir) == path.Base(subjectDir)
		initMatch := isInitPy && testStem == "init" && importsSubject && correspondingDir

		// __init__.py parent-directory stem matching: for barrel files like
		// pipelines/__init__.py, the test is typically test_pipelines.py.
		// extractSubjectStem returns "" for __init__.py (to prevent "index" stem
		// pollution), so we match via the parent directory name instead.
		// A test qualifies when its stem matches the parent dir AND it's either
		// in a tests/ directory (structural location) or imports the subject.
		initParentMatch := false
		if isInitPy {
			parentDirStem := path.Base(subjectDir)
			if parentDirStem != "" && parentDirStem != "." && parentDirStem != "src" {
				inTestsDir := hasTestDirComponent(testDir)
				dirMatches := testStem == parentDirStem
				if !dirMatches {
					if normDir := normalizePyDirStem(parentDirStem); normDir != "" {
						dirMatches = testStem == normDir
					}
				}
				// Fuzzy: tokenizedata/__init__.py → test_tokenize.py
				// stemContains("tokenize","tokenizedata",0.6,5) → prefix match, ratio 0.67
				if !dirMatches && stemContains(testStem, parentDirStem, 0.6, 5) {
					dirMatches = true
				}
				if dirMatches && (inTestsDir || importsSubject) {
					initParentMatch = true
				}
			}
		}

		// Regular Python files: match tests whose stem equals the parent directory.
		// E.g., Lib/asyncio/events.py matches test_asyncio.py because the parent
		// directory "asyncio" equals the test stem. Gated by test-directory location
		// or import evidence to prevent false positives.
		parentDirStemMatch := false
		if !isInitPy && strings.HasSuffix(subject, ".py") {
			parentDirStem := path.Base(subjectDir)
			if parentDirStem != "" && parentDirStem != "." && parentDirStem != "src" {
				dirMatches := testStem == parentDirStem
				if !dirMatches {
					if normDir := normalizePyDirStem(parentDirStem); normDir != "" {
						dirMatches = testStem == normDir
					}
				}
				// Fuzzy: libregrtest/filter.py → test_regrtest.py
				// Requires importsSubject for safety (stricter than initParentMatch).
				if !dirMatches && importsSubject && stemContains(testStem, parentDirStem, 0.6, 5) {
					dirMatches = true
				}
				if dirMatches && (hasTestDirComponent(testDir) || importsSubject) {
					parentDirStemMatch = true
				}
			}
		}

		// Nested test directory match: test_<package>/test_<module>.py tests <package>/<module>.py.
		// E.g., Lib/test/test_asyncio/test_events.py tests Lib/asyncio/events.py.
		// The directory name (stripped of test_ prefix) must match the subject's parent
		// directory, AND the filename stems must match. This 3-way correspondence is a
		// strong signal with very low false-positive risk.
		nestedTestDirMatch := false
		if !isInitPy && strings.HasSuffix(subject, ".py") && testStem != "" && subjectStem != "" {
			stemMatches := testStem == subjectStem
			if !stemMatches {
				if normStem := normalizePyStem(subjectStem); normStem != "" {
					stemMatches = testStem == normStem
				}
			}
			if stemMatches {
				testDirBase := path.Base(testDir)
				if strings.HasPrefix(testDirBase, "test_") {
					testDirStem := testDirBase[5:] // strip "test_" prefix
					subjectDirBase := path.Base(subjectDir)
					if testDirStem == subjectDirBase {
						nestedTestDirMatch = true
					} else if normDir := normalizePyDirStem(subjectDirBase); normDir != "" && testDirStem == normDir {
						nestedTestDirMatch = true
					}
				}
			}
		}

		// Nested test dir importer: test_<package>/test_*.py where the test imports
		// the subject but filename stems don't match. This catches tests organized by
		// package directory that test a different module within the same package.
		// E.g., asyncio/constants.py → test_asyncio/test_base_events.py (imports constants).
		nestedTestDirImporter := false
		if strings.HasSuffix(subject, ".py") && importsSubject {
			testDirBase := path.Base(testDir)
			if strings.HasPrefix(testDirBase, "test_") {
				testDirStem := testDirBase[5:]
				subjectDirBase := path.Base(subjectDir)
				if testDirStem == subjectDirBase {
					nestedTestDirImporter = true
				} else if normDir := normalizePyDirStem(subjectDirBase); normDir != "" && testDirStem == normDir {
					nestedTestDirImporter = true
				}
			}
		}

		// testsSubdir alone is too loose — a __tests__/ dir may hold tests for
		// many sibling files. Require an import or stem match to confirm the
		// test actually targets the subject.
		testsSubdirQualified := testsSubdir && (importsSubject || subjectStemMatch)

		// sameDir alone is too loose — a directory may hold tests for many
		// sibling files. Require an import or stem match (same as testsSubdir).
		sameDirQualified := sameDir && (importsSubject || subjectStemMatch)

		// Java mirror directory: src/main/java/pkg/... ↔ src/test/java/pkg/...
		javaMirrorDir := isJavaTestMirror(subjectDir, testDir)
		mirrorDirQualified := javaMirrorDir && (importsSubject || subjectStemMatch)

		// A test qualifies as "direct" when structural signals confirm it targets
		// the subject: co-located (same dir or __tests__/) with an import or stem
		// match, or stem match from any location. Importing the subject alone is
		// insufficient — many consumers import a file without being tests *for* it.
		isDirect := sameDirQualified || testsSubdirQualified || subjectStemMatch || initMatch || initParentMatch || parentDirStemMatch || nestedTestDirMatch || nestedTestDirImporter || mirrorDirQualified

		// Rust integration test classification: tests/ directory at crate root.
		if !isDirect && strings.HasSuffix(subject, ".rs") && !classify.IsTestFile(subject) {
			if matches, directMatch := rustIntegrationTestMatch(subject, sc.Path); matches {
				isDirect = directMatch
			}
		}

		if isDirect {
			direct = append(direct, sc)
			continue
		}

		// --- Dependency test criteria ---
		coChanges := sc.CoChangeFreq > 0
		nearMustRead := mustReadDirs[testDir]
		if !nearMustRead {
			for dir := range mustReadDirs {
				if testDir == dir+"/__tests__" || strings.HasPrefix(testDir, dir+"/__tests__/") {
					nearMustRead = true
					break
				}
			}
		}
		stemMatch := testMatchesMustReadStem(sc.Path, mustReadPaths)

		isDep := coChanges || nearMustRead || stemMatch

		// Rust: same-crate integration tests qualify as dependency tests.
		if !isDep && strings.HasSuffix(subject, ".rs") && !classify.IsTestFile(subject) {
			isDep = isSameCrateIntegrationTest(subject, sc.Path)
		}

		if isDep {
			dependency = append(dependency, sc)
		}
	}

	// Go package-level test fallback: Go tests test the entire package, not
	// individual files. When no direct tests found for a .go subject, promote
	// same-directory _test.go files as direct tests (capped at 2).
	if len(direct) == 0 && strings.HasSuffix(subject, ".go") && !strings.HasSuffix(subject, "_test.go") {
		promoted := 0
		for _, sc := range candidates {
			if promoted >= 2 {
				break
			}
			if path.Dir(sc.Path) == subjectDir && strings.HasSuffix(sc.Path, "_test.go") {
				direct = append(direct, sc)
				promoted++
			}
		}
	}

	// Rust integration test fallback: when no direct tests found for a .rs subject,
	// promote integration tests from the crate's tests/ directory as direct (capped at 2).
	if len(direct) == 0 && strings.HasSuffix(subject, ".rs") && !classify.IsTestFile(subject) {
		promoted := 0
		for _, sc := range candidates {
			if promoted >= 2 {
				break
			}
			if matches, _ := rustIntegrationTestMatch(subject, sc.Path); matches {
				direct = append(direct, sc)
				promoted++
			}
		}
	}

	// Sort each tier by score desc, then path asc.
	sortByScore := func(s []scoredCandidate) {
		sort.Slice(s, func(i, j int) bool {
			if s[i].Score != s[j].Score {
				return s[i].Score > s[j].Score
			}
			return s[i].Path < s[j].Path
		})
	}
	sortByScore(direct)
	sortByScore(dependency)

	// Direct tests fill slots first (unlimited up to capTests).
	// When no direct tests exist, dependency tests are suppressed entirely —
	// they test dependencies, not the subject itself. When direct tests DO
	// exist, dependency tests are sub-capped at maxDependencyTests (2).
	totalBeforeCap := len(direct) + len(dependency)

	remainingSlots := capTests - len(direct)
	if remainingSlots < 0 {
		remainingSlots = 0
	}
	if len(direct) == 0 {
		remainingSlots = 0 // No direct tests → dependency tests are noise
	} else if remainingSlots > maxDependencyTests {
		remainingSlots = maxDependencyTests
	}
	if len(dependency) > remainingSlots {
		dependency = dependency[:remainingSlots]
	}
	combined := append(direct, dependency...)

	cap := capTests
	if len(combined) < cap {
		cap = len(combined)
	}
	result := make([]testCandidate, cap)
	for i := 0; i < cap; i++ {
		isDirect := i < len(direct)
		result[i] = testCandidate{path: combined[i].Path, direct: isDirect}
	}
	return result, totalBeforeCap
}

// hasTestDirComponent returns true if any path component is "test" or "tests".
func hasTestDirComponent(dir string) bool {
	for _, part := range strings.Split(dir, "/") {
		if part == "test" || part == "tests" {
			return true
		}
	}
	return false
}

// extractTestStem extracts the base name stem from a test file, stripping
// test markers and extensions. Returns "" for unrecognized patterns.
// Examples: "subtitle-formatter.test.ts" → "subtitle-formatter"
//
//	"test_models.py" → "models"
//	"models_test.py" → "models"
func extractTestStem(base string) string {
	// TS/JS: *.test.ext, *.spec.ext
	for _, mid := range []string{".test.", ".spec."} {
		if idx := strings.Index(base, mid); idx > 0 {
			return base[:idx]
		}
	}
	// Python: test_*.py
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return base[5 : len(base)-3]
	}
	// Python: *_test.py
	if strings.HasSuffix(base, "_test.py") {
		return base[:len(base)-8]
	}
	// Go: *_test.go (strip platform suffixes like _linux, _darwin_amd64)
	if strings.HasSuffix(base, "_test.go") {
		stem := base[:len(base)-8]
		return classify.StripGoPlatformSuffix(stem)
	}
	// Java: FooTest.java, FooTests.java, TestFoo.java
	if strings.HasSuffix(base, ".java") {
		stem := strings.TrimSuffix(base, ".java")
		if strings.HasSuffix(stem, "Tests") {
			return stem[:len(stem)-5]
		}
		if strings.HasSuffix(stem, "Test") {
			return stem[:len(stem)-4]
		}
		if strings.HasPrefix(stem, "Test") {
			return stem[4:]
		}
	}
	// Rust: foo_test.rs, test_bar.rs
	if strings.HasSuffix(base, "_test.rs") {
		return base[:len(base)-8]
	}
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".rs") {
		return base[5 : len(base)-3]
	}
	return ""
}

// extractSubjectStem extracts the base name stem from a non-test file,
// stripping only the final extension. Returns "" for empty input.
// Examples: "subtitle.agent.ts" → "subtitle.agent"
//
//	"logger.ts" → "logger"
//	"models.py" → "models"
func extractSubjectStem(base string) string {
	// Barrel files (index.ts, __init__.py) produce generic stems like "index"
	// that match every index.test.ts across the repo. Disable stem matching;
	// barrel file tests are still found via sameDir, importsSubject, or testsSubdir+import.
	if classify.IsBarrelFile(base) {
		return ""
	}
	if idx := strings.LastIndex(base, "."); idx > 0 {
		stem := base[:idx]
		if strings.HasSuffix(base, ".go") {
			stem = classify.StripGoPlatformSuffix(stem)
		}
		return stem
	}
	return ""
}

// normalizePyDirStem strips one leading underscore from a Python directory name.
// Returns "" if no normalization was applicable.
// "_pyrepl" → "pyrepl", "__private" → "_private", "normal" → ""
func normalizePyDirStem(stem string) string {
	if len(stem) > 1 && stem[0] == '_' {
		return stem[1:]
	}
	return ""
}

// normalizePyStem strips trailing digits from a Python filename stem.
// Returns "" if no digits were stripped or result would be < 2 chars.
// "dbapi2" → "dbapi", "sha256" → "sha", "v2" → "" (too short), "config" → ""
func normalizePyStem(stem string) string {
	i := len(stem)
	for i > 0 && stem[i-1] >= '0' && stem[i-1] <= '9' {
		i--
	}
	if i == len(stem) || i < 2 {
		return ""
	}
	return stem[:i]
}

// stemContains returns true when the shorter string is a prefix or suffix of the
// longer one, subject to minimum length and ratio constraints. This enables fuzzy
// matching like "tokenize" ↔ "tokenizedata" or "regrtest" ↔ "libregrtest" while
// rejecting short or disproportionate stems like "log" or "pydoc" ↔ "pydoc_data".
func stemContains(a, b string, minRatio float64, minLen int) bool {
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	if len(shorter) < minLen {
		return false
	}
	if float64(len(shorter))/float64(len(longer)) < minRatio {
		return false
	}
	return strings.HasPrefix(longer, shorter) || strings.HasSuffix(longer, shorter)
}

// isJavaTestMirror detects the Maven/Gradle convention where source files in
// src/main/java/pkg/... have corresponding tests in src/test/java/pkg/...
func isJavaTestMirror(sourceDir, testDir string) bool {
	// Look for /src/main/ in sourceDir and corresponding /src/test/ in testDir.
	mainIdx := strings.Index(sourceDir, "/src/main/java/")
	if mainIdx < 0 && strings.HasPrefix(sourceDir, "src/main/java/") {
		mainIdx = 0
	}
	if mainIdx < 0 {
		return false
	}

	var prefix, suffix string
	if mainIdx == 0 {
		prefix = ""
		suffix = sourceDir[len("src/main/java/"):]
	} else {
		prefix = sourceDir[:mainIdx+1]
		suffix = sourceDir[mainIdx+len("/src/main/java/"):]
	}

	expectedTestDir := prefix + "src/test/java/" + suffix
	return testDir == expectedTestDir
}

// testMatchesMustReadStem returns true if the test file's stem matches
// the stem (filename without extension) of any must_read file.
func testMatchesMustReadStem(testPath string, mustReadPaths []string) bool {
	testStem := extractTestStem(path.Base(testPath))
	if testStem == "" {
		return false
	}
	for _, p := range mustReadPaths {
		mrBase := path.Base(p)
		if idx := strings.Index(mrBase, "."); idx > 0 {
			if mrBase[:idx] == testStem {
				return true
			}
		}
	}
	return false
}
