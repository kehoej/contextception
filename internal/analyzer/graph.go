package analyzer

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/db"
)

// candidate represents a file discovered during graph traversal.
type candidate struct {
	Path                 string
	Distance             int  // 0 = subject, 1 = direct, 2 = two-hop
	IsImport             bool // subject imports this file
	IsImporter           bool // this file imports the subject
	IsTransitiveCaller   bool // imports a barrel file that re-exports the subject
	IsSamePackageSibling bool // Go: same-package file with implicit visibility (no import edge)
	IsGoSamePackage      bool // Go: any .go file in the same directory as the subject (superset of IsSamePackageSibling)
	HasPrefixMatch       bool // Go: shares an underscore-delimited filename prefix with subject
	IsSmallGoPackage     bool // Go: directory has ≤ 20 non-test .go files
	IsJavaSamePackage    bool // Java: same-package file (same directory, .java)
	HasJavaClassPrefix   bool // Java: shares a CamelCase class name prefix with subject
	IsSmallJavaPackage   bool // Java: directory has ≤ 20 non-test .java files
	IsRustSameModule     bool // Rust: same-directory .rs file (same module)
	HasRustModulePrefix  bool // Rust: shares underscore-delimited filename prefix with subject
	IsSmallRustModule    bool // Rust: directory has ≤ 20 non-test .rs files
	HasInlineTests       bool // Rust: file contains #[cfg(test)] inline test module
	IsCSharpSameNamespace  bool // C#: same-directory .cs file (same namespace)
	HasCSharpClassPrefix   bool // C#: shares a CamelCase class name prefix with subject
	IsSmallCSharpNamespace bool // C#: directory has ≤ 20 non-test .cs files
	Signals              db.SignalRow
	Churn                float64 // normalized 0.0-1.0
	CoChangeFreq         int     // raw co-change frequency with subject
}

// collectCandidates performs a 2-hop graph traversal from the subject file
// and enriches candidates with git history signals.
func collectCandidates(idx *db.Index, subject string, repoRoot string) ([]candidate, error) {
	candidates := make(map[string]*candidate)

	// Subject itself at distance 0.
	candidates[subject] = &candidate{
		Path:     subject,
		Distance: 0,
	}

	// Forward deps: files that subject imports (distance 1).
	imports, err := idx.GetDirectImports(subject)
	if err != nil {
		return nil, err
	}
	for _, path := range imports {
		if _, exists := candidates[path]; !exists {
			candidates[path] = &candidate{Path: path, Distance: 1}
		}
		candidates[path].IsImport = true
	}

	// Reverse deps: files that import the subject (distance 1).
	importers, err := idx.GetDirectImporters(subject)
	if err != nil {
		return nil, err
	}
	for _, path := range importers {
		if _, exists := candidates[path]; !exists {
			candidates[path] = &candidate{Path: path, Distance: 1}
		}
		candidates[path].IsImporter = true
	}

	// Build the set of all distance-0 and distance-1 files for exclusion.
	directSet := make(map[string]bool, len(candidates))
	directPaths := make([]string, 0, len(candidates))
	for p := range candidates {
		directSet[p] = true
		directPaths = append(directPaths, p)
	}

	// 2-hop: forward edges from direct neighbors.
	twoHopForward, err := idx.GetImportsOf(directPaths, directSet)
	if err != nil {
		return nil, err
	}
	for _, path := range twoHopForward {
		if _, exists := candidates[path]; !exists {
			candidates[path] = &candidate{Path: path, Distance: 2}
		}
	}

	// 2-hop: reverse edges from direct neighbors.
	twoHopReverse, err := idx.GetImportersOf(directPaths, directSet)
	if err != nil {
		return nil, err
	}
	for _, path := range twoHopReverse {
		if _, exists := candidates[path]; !exists {
			candidates[path] = &candidate{Path: path, Distance: 2}
		}
	}

	// Targeted 3-hop: reverse importers through barrel files only.
	// When the subject is re-exported through a barrel file (index.ts, __init__.py),
	// the file that imports the barrel is a "transitive caller" — semantically
	// close despite being at graph distance 3. We assign Distance=2 to keep them
	// within the 2-hop categorization boundary.
	var barrelPaths []string
	for _, path := range importers { // distance-1 reverse importers
		if classify.IsBarrelFile(path) {
			barrelPaths = append(barrelPaths, path)
		}
	}
	if len(barrelPaths) > 0 {
		// Only exclude direct neighbors (distance 0-1) from barrel-piercing results.
		// Distance-2 candidates found by normal traversal should still be upgraded
		// with the IsTransitiveCaller flag.
		barrelImporters, err := idx.GetImportersOf(barrelPaths, directSet)
		if err != nil {
			return nil, err
		}
		for _, path := range barrelImporters {
			if c, exists := candidates[path]; exists {
				// Upgrade existing candidate with transitive caller flag.
				c.IsTransitiveCaller = true
			} else {
				candidates[path] = &candidate{
					Path: path, Distance: 2, IsTransitiveCaller: true,
				}
			}
		}
	}

	// Targeted test expansion: find test files that import candidate source files.
	// Test files at graph distance 3+ are missed by the standard 2-hop traversal
	// but are relevant when they test a dependency or consumer of the subject.
	// Only use distance 0-1 candidates as seeds to prevent fan-out across the
	// entire codebase in large repos with many 2-hop candidates.
	var candidateSourcePaths []string
	for p, c := range candidates {
		if c.Distance <= 1 && !classify.IsTestFile(p) {
			candidateSourcePaths = append(candidateSourcePaths, p)
		}
	}
	if len(candidateSourcePaths) > 0 {
		allExclude := make(map[string]bool, len(candidates))
		for p := range candidates {
			allExclude[p] = true
		}
		testImporters, testErr := idx.GetImportersOf(candidateSourcePaths, allExclude)
		if testErr == nil {
			for _, path := range testImporters {
				if classify.IsTestFile(path) {
					candidates[path] = &candidate{Path: path, Distance: 2}
				}
			}
		}
	}

	// Go/Rust same-directory test discovery: _test.go files aren't connected via
	// imports but co-located in the same directory. Query the index for test files
	// in the subject's directory that haven't already been found.
	if strings.HasSuffix(subject, ".go") && !strings.HasSuffix(subject, "_test.go") {
		subjectDir := path.Dir(subject)
		testFiles, err := idx.GetTestFilesInDir(subjectDir, "_test.go")
		if err == nil {
			for _, tf := range testFiles {
				if _, exists := candidates[tf]; !exists {
					candidates[tf] = &candidate{Path: tf, Distance: 1, IsImporter: true}
				}
			}
		}

		// Go same-package non-test sibling discovery: with maxSamePackageFiles=1,
		// most siblings are not in the graph. Discover up to 10 for likely_modify
		// and must_read candidacy, since Go's package visibility creates implicit coupling.
		// Siblings sharing a filename prefix with the subject are prioritized.
		siblings, err := idx.GetGoSiblingsInDir(subjectDir, subject)
		if err == nil {
			packageSize := len(siblings) + 1 // +1 for subject itself
			isSmallPkg := packageSize <= maxPackageSize
			siblings = prioritizeByPrefix(siblings, subject)

			// For large packages: only add prefix-matched siblings to avoid noise.
			// Non-prefix siblings in a 100+ file package are rarely relevant.
			subjectStem := strings.TrimSuffix(path.Base(subject), ".go")
			if !isSmallPkg {
				var filtered []string
				for _, sf := range siblings {
					sfStem := strings.TrimSuffix(path.Base(sf), ".go")
					if shareUnderscorePrefix(subjectStem, sfStem) {
						filtered = append(filtered, sf)
					}
				}
				siblings = filtered
			}

			count := 0
			for _, sf := range siblings {
				if _, exists := candidates[sf]; !exists {
					candidates[sf] = &candidate{Path: sf, Distance: 1, IsSamePackageSibling: true}
					count++
					if count >= maxSiblingCandidates {
						break
					}
				}
			}

			// Post-process: mark ALL Go candidates in subject's directory
			// with IsGoSamePackage/HasPrefixMatch/IsSmallGoPackage.
			// This catches files discovered via edge resolution (IsImport=true)
			// that are actually same-package files.
			for _, c := range candidates {
				if c.Path != subject && path.Dir(c.Path) == subjectDir &&
					strings.HasSuffix(c.Path, ".go") && !strings.HasSuffix(c.Path, "_test.go") {
					c.IsGoSamePackage = true
					c.IsSmallGoPackage = isSmallPkg
					c.Distance = 1 // Go package visibility is implicit — same-package = direct neighbor
					candidateStem := strings.TrimSuffix(path.Base(c.Path), ".go")
					if shareUnderscorePrefix(subjectStem, candidateStem) {
						c.HasPrefixMatch = true
					}
				}
			}
		}
	}

	// Java mirror-directory test discovery: Java tests live in src/test/java/pkg/...
	// mirroring src/main/java/pkg/... with no import edges (same-package visibility).
	// Without this discovery, mirror-dir tests are invisible to graph traversal.
	if strings.HasSuffix(subject, ".java") && !classify.IsTestFile(subject) {
		subjectDir := path.Dir(subject)
		mirrorDir := javaTestMirrorDir(subjectDir)
		if mirrorDir != "" {
			testFiles, err := idx.GetTestFilesInDir(mirrorDir, ".java")
			if err == nil {
				for _, tf := range testFiles {
					if _, exists := candidates[tf]; !exists {
						candidates[tf] = &candidate{Path: tf, Distance: 1, IsImporter: true}
					}
				}
			}
		}

		// Java same-package non-test sibling discovery: same-package classes have
		// implicit visibility (package-private access), creating structural coupling
		// analogous to Go packages. Discover up to 10 siblings for likely_modify
		// and must_read candidacy. Class-prefix-matched siblings are prioritized.
		siblings, err := idx.GetJavaSiblingsInDir(subjectDir, subject)
		if err == nil {
			packageSize := len(siblings) + 1 // +1 for subject itself
			isSmallPkg := packageSize <= maxPackageSize
			siblings = prioritizeByJavaClassPrefix(siblings, subject)

			// For large packages: only add class-prefix-matched siblings.
			subjectStem := strings.TrimSuffix(path.Base(subject), ".java")
			if !isSmallPkg {
				var filtered []string
				for _, sf := range siblings {
					sfStem := strings.TrimSuffix(path.Base(sf), ".java")
					if shareJavaClassPrefix(subjectStem, sfStem) {
						filtered = append(filtered, sf)
					}
				}
				siblings = filtered
			}

			count := 0
			for _, sf := range siblings {
				if _, exists := candidates[sf]; !exists {
					candidates[sf] = &candidate{Path: sf, Distance: 1, IsJavaSamePackage: true}
					count++
					if count >= maxSiblingCandidates {
						break
					}
				}
			}

			// Post-process: mark ALL Java candidates in subject's directory
			// with IsJavaSamePackage/HasJavaClassPrefix/IsSmallJavaPackage.
			for _, c := range candidates {
				if c.Path != subject && path.Dir(c.Path) == subjectDir &&
					strings.HasSuffix(c.Path, ".java") && !classify.IsTestFile(c.Path) {
					c.IsJavaSamePackage = true
					c.IsSmallJavaPackage = isSmallPkg
					c.Distance = 1 // Java package visibility is implicit — same-package = direct neighbor
					candidateStem := strings.TrimSuffix(path.Base(c.Path), ".java")
					if shareJavaClassPrefix(subjectStem, candidateStem) {
						c.HasJavaClassPrefix = true
					}
				}
			}
		}
	}

	// Rust same-module discovery: Rust modules are directory-scoped (like Go packages).
	// Files in the same directory share implicit visibility via the module system.
	if strings.HasSuffix(subject, ".rs") && !classify.IsTestFile(subject) {
		subjectDir := path.Dir(subject)

		// Rust crate-level integration test discovery: tests/ directory at crate root.
		crateRoot := rustCrateRoot(subject)
		if crateRoot != "" {
			var testsDir string
			if crateRoot == "." {
				testsDir = "tests"
			} else {
				testsDir = crateRoot + "/tests"
			}
			testFiles, err := idx.GetTestFilesInDir(testsDir, ".rs")
			if err == nil {
				modPath := rustSourceModPath(subject, crateRoot)
				if modPath == "" {
					modPath = rustCrateNameFromPath(crateRoot)
				}
				matchedCount := 0
				for _, tf := range testFiles {
					if _, exists := candidates[tf]; exists {
						continue
					}
					testBase := path.Base(tf)
					testStem := strings.TrimSuffix(testBase, ".rs")
					if testStem == "mod" || testStem == "main" {
						continue
					}
					if matches, _ := rustTestModuleMatch(testStem, modPath); matches {
						candidates[tf] = &candidate{Path: tf, Distance: 1, IsImporter: true}
						matchedCount++
					}
				}
				// Fallback: if no name-matched tests, add up to 3 from tests/ dir.
				if matchedCount == 0 {
					added := 0
					for _, tf := range testFiles {
						if _, exists := candidates[tf]; exists {
							continue
						}
						testStem := strings.TrimSuffix(path.Base(tf), ".rs")
						if testStem == "mod" || testStem == "main" {
							continue
						}
						candidates[tf] = &candidate{Path: tf, Distance: 2}
						added++
						if added >= rustFallbackTestCount {
							break
						}
					}
				}
			}
		}

		// Rust same-directory _test.rs discovery (rare but possible convention).
		testFiles2, err := idx.GetTestFilesInDir(subjectDir, "_test.rs")
		if err == nil {
			for _, tf := range testFiles2 {
				if _, exists := candidates[tf]; !exists {
					candidates[tf] = &candidate{Path: tf, Distance: 1, IsImporter: true}
				}
			}
		}

		// Rust same-module sibling discovery.
		siblings, err := idx.GetRustSiblingsInDir(subjectDir, subject)
		if err == nil {
			moduleSize := len(siblings) + 1 // +1 for subject itself
			isSmallMod := moduleSize <= maxPackageSize
			siblings = prioritizeByRustPrefix(siblings, subject)

			// For large modules: only add prefix-matched siblings to avoid noise.
			subjectStem := strings.TrimSuffix(path.Base(subject), ".rs")
			if !isSmallMod {
				var filtered []string
				for _, sf := range siblings {
					sfStem := strings.TrimSuffix(path.Base(sf), ".rs")
					if shareUnderscorePrefix(subjectStem, sfStem) {
						filtered = append(filtered, sf)
					}
				}
				siblings = filtered
			}

			count := 0
			for _, sf := range siblings {
				if _, exists := candidates[sf]; !exists {
					candidates[sf] = &candidate{Path: sf, Distance: 1, IsRustSameModule: true}
					count++
					if count >= maxSiblingCandidates {
						break
					}
				}
			}

			// Post-process: mark ALL Rust candidates in subject's directory
			// with IsRustSameModule/HasRustModulePrefix/IsSmallRustModule.
			for _, c := range candidates {
				if c.Path != subject && path.Dir(c.Path) == subjectDir &&
					strings.HasSuffix(c.Path, ".rs") && !classify.IsTestFile(c.Path) {
					c.IsRustSameModule = true
					c.IsSmallRustModule = isSmallMod
					c.Distance = 1 // Rust module visibility is implicit — same-module = direct neighbor
					candidateStem := strings.TrimSuffix(path.Base(c.Path), ".rs")
					if shareUnderscorePrefix(subjectStem, candidateStem) {
						c.HasRustModulePrefix = true
					}
				}
			}
		}

		// Rust inline test detection: scan subject and same-directory
		// siblings for #[cfg(test)] modules.
		if repoRoot != "" {
			// Check subject itself.
			if content, err := os.ReadFile(filepath.Join(repoRoot, subject)); err == nil {
				if classify.HasInlineRustTests(content) {
					candidates[subject].HasInlineTests = true
				}
			}
			// Check same-directory siblings already in candidates.
			for _, c := range candidates {
				if c.Path == subject || c.Distance > 1 {
					continue
				}
				if !strings.HasSuffix(c.Path, ".rs") || classify.IsTestFile(c.Path) {
					continue
				}
				if path.Dir(c.Path) != subjectDir {
					continue
				}
				if content, err := os.ReadFile(filepath.Join(repoRoot, c.Path)); err == nil {
					if classify.HasInlineRustTests(content) {
						c.HasInlineTests = true
					}
				}
			}
		}
	}

	// C# same-namespace discovery: C# namespaces provide implicit type visibility
	// within the same directory, analogous to Java packages and Go packages.
	if strings.HasSuffix(subject, ".cs") && !classify.IsTestFile(subject) {
		subjectDir := path.Dir(subject)
		siblings, err := idx.GetCSharpSiblingsInDir(subjectDir, subject)
		if err == nil {
			namespaceSize := len(siblings) + 1
			isSmallNs := namespaceSize <= maxPackageSize
			subjectStem := strings.TrimSuffix(path.Base(subject), ".cs")
			siblings = prioritizeByJavaClassPrefix(siblings, subject) // CamelCase prefix works for C# too

			// For large namespaces: only add class-prefix-matched siblings.
			if !isSmallNs {
				var filtered []string
				for _, sf := range siblings {
					sfStem := strings.TrimSuffix(path.Base(sf), ".cs")
					if shareJavaClassPrefix(subjectStem, sfStem) {
						filtered = append(filtered, sf)
					}
				}
				siblings = filtered
			}

			count := 0
			for _, sf := range siblings {
				if _, exists := candidates[sf]; !exists {
					candidates[sf] = &candidate{Path: sf, Distance: 1, IsCSharpSameNamespace: true}
					count++
					if count >= maxSiblingCandidates {
						break
					}
				}
			}

			// Post-process: mark ALL C# candidates in subject's directory
			// with IsCSharpSameNamespace/HasCSharpClassPrefix/IsSmallCSharpNamespace.
			for _, c := range candidates {
				if c.Path != subject && path.Dir(c.Path) == subjectDir &&
					strings.HasSuffix(c.Path, ".cs") && !classify.IsTestFile(c.Path) {
					c.IsCSharpSameNamespace = true
					c.IsSmallCSharpNamespace = isSmallNs
					c.Distance = 1
					candidateStem := strings.TrimSuffix(path.Base(c.Path), ".cs")
					if shareJavaClassPrefix(subjectStem, candidateStem) {
						c.HasCSharpClassPrefix = true
					}
				}
			}
		}
	}

	// Python tests/ directory test discovery: Python projects commonly place tests
	// in a root-level tests/ directory (e.g., tests/test_configuration_utils.py),
	// which has no import edges back to the source files. Without this discovery,
	// these tests are invisible to graph traversal.
	if strings.HasSuffix(subject, ".py") && !classify.IsTestFile(subject) {
		subjectBase := path.Base(subject)
		// Use a set to deduplicate candidate test basenames.
		basenameSet := make(map[string]bool)
		addBasename := func(name string) {
			if !basenameSet[name] {
				basenameSet[name] = true
			}
		}

		if subjectBase == "__init__.py" {
			// For __init__.py: use parent directory name → test_<parent>.py
			parentDir := path.Base(path.Dir(subject))
			if parentDir != "" && parentDir != "." && parentDir != "src" {
				addBasename("test_" + parentDir + ".py")
				if normDir := normalizePyDirStem(parentDir); normDir != "" {
					addBasename("test_" + normDir + ".py")
				}
				// Strip common data/utility suffixes from parent dir to find related tests.
				// E.g., tokenizedata → test_tokenize.py, pydoc_data → test_pydoc.py
				for _, suffix := range []string{"_data", "data", "_lib", "lib", "_utils", "utils", "_core", "core"} {
					if strings.HasSuffix(parentDir, suffix) {
						stripped := parentDir[:len(parentDir)-len(suffix)]
						if len(stripped) >= 3 {
							addBasename("test_" + stripped + ".py")
						}
						break // only strip the first matching suffix
					}
				}
			}
		} else {
			// For regular files: test_<stem>.py
			stem := strings.TrimSuffix(subjectBase, ".py")
			addBasename("test_" + stem + ".py")
			if normStem := normalizePyStem(stem); normStem != "" {
				addBasename("test_" + normStem + ".py")
			}
		}

		// Also try parent directory name as secondary stem for deeply nested files.
		// e.g., models/bert/modeling_bert.py → test_modeling_bert.py already covered,
		// but also try test_bert.py for broader matches.
		if subjectBase != "__init__.py" {
			parentDir := path.Base(path.Dir(subject))
			if parentDir != "" && parentDir != "." && parentDir != "src" {
				addBasename("test_" + parentDir + ".py")
				if normParent := normalizePyDirStem(parentDir); normParent != "" {
					addBasename("test_" + normParent + ".py")
				}
			}
		}

		basenames := make([]string, 0, len(basenameSet))
		for name := range basenameSet {
			basenames = append(basenames, name)
		}

		if len(basenames) > 0 {
			// Search globally across the entire index for matching test files.
			// This handles any test directory layout (tests/, test/, Lib/test/, etc.)
			// without hardcoding directory names.
			testFiles, err := idx.GetTestFilesByName("", basenames)
			if err == nil {
				for _, tf := range testFiles {
					if _, exists := candidates[tf]; !exists {
						candidates[tf] = &candidate{Path: tf, Distance: 2}
					}
				}
			}
		}
	}

	// C# test discovery: C# test files live in separate .Tests project directories
	// (e.g., tests/Jellyfin.Server.Implementations.Tests/SessionManager/SessionManagerTests.cs)
	// with no import edges back to the source. We search the index for test files
	// whose stem matches the subject stem, plus stems with common suffixes stripped
	// (e.g., AutomaticLineEnderCommandHandler → AutomaticLineEnderTests.cs).
	if strings.HasSuffix(subject, ".cs") && !classify.IsTestFile(subject) {
		subjectStem := strings.TrimSuffix(path.Base(subject), ".cs")
		if subjectStem != "" {
			seen := map[string]bool{}
			var testBasenames []string
			addTestName := func(stem string) {
				for _, name := range []string{
					stem + "Tests.cs",
					stem + "Test.cs",
					"Test" + stem + ".cs",
					stem + "Spec.cs",
				} {
					if !seen[name] {
						seen[name] = true
						testBasenames = append(testBasenames, name)
					}
				}
			}

			// Exact stem match.
			addTestName(subjectStem)

			// Strip common C# class suffixes and search for shortened stems.
			// E.g., AutomaticLineEnderCommandHandler → AutomaticLineEnder
			// MakeLocalFunctionStaticHelper → MakeLocalFunctionStatic
			csharpClassSuffixes := []string{
				"CommandHandler", "Handler", "Helpers", "Helper",
				"Service", "Provider", "Manager", "Factory",
				"Builder", "Adapter", "Wrapper", "Extensions",
				"Repository", "Controller", "Options", "Configuration",
				"Validator", "Converter", "Processor", "Client",
			}
			for _, suffix := range csharpClassSuffixes {
				if stripped := strings.TrimSuffix(subjectStem, suffix); stripped != subjectStem && len(stripped) > 2 {
					addTestName(stripped)
				}
			}

			// CamelCase prefix search: for multi-word stems, try progressively
			// shorter prefixes. E.g., MakeLocalFunctionStaticHelper →
			// MakeLocalFunctionStatic, MakeLocalFunction, MakeLocal.
			parts := splitCamelCase(subjectStem)
			if len(parts) >= 3 {
				// Try dropping 1 and 2 trailing words.
				for drop := 1; drop <= 2 && drop < len(parts)-1; drop++ {
					prefix := strings.Join(parts[:len(parts)-drop], "")
					if len(prefix) > 3 {
						addTestName(prefix)
					}
				}
			}

			testFiles, err := idx.GetTestFilesByName("", testBasenames)
			if err == nil {
				for _, tf := range testFiles {
					if _, exists := candidates[tf]; !exists {
						candidates[tf] = &candidate{Path: tf, Distance: 2}
					}
				}
			}
		}
	}

	// C# test project mirror discovery: C# test projects (e.g., tests/MyLib.Tests/)
	// mirror source projects (e.g., src/MyLib/) with test files in corresponding
	// subdirectories. Like Java's src/main/java ↔ src/test/java pattern, but using
	// .NET project naming conventions. We discover the source project name, find
	// matching test project directories, and add test files as Distance=1 candidates.
	if strings.HasSuffix(subject, ".cs") && !classify.IsTestFile(subject) {
		if projectName, relPath := csharpProjectInfo(subject); projectName != "" {
			testDirs, err := idx.FindCSharpTestDirs(projectName)
			if err == nil {
				const maxMirrorTests = 10
				for _, testDir := range testDirs {
					// Look for test files in the mirrored subdirectory path.
					var searchDir string
					if relPath != "" {
						searchDir = testDir + "/" + relPath
					} else {
						searchDir = testDir
					}
					testFiles, err := idx.GetTestFilesUnderDir(searchDir, ".cs")
					if err != nil {
						continue
					}
					count := 0
					for _, tf := range testFiles {
						if classify.IsTestFile(tf) {
							if _, exists := candidates[tf]; !exists {
								candidates[tf] = &candidate{Path: tf, Distance: 1, IsImporter: true}
								count++
								if count >= maxMirrorTests {
									break
								}
							}
						}
					}
				}
			}
		}
	}

	// Fetch signals for all candidates.
	allPaths := make([]string, 0, len(candidates))
	for p := range candidates {
		allPaths = append(allPaths, p)
	}
	signals, err := idx.GetSignals(allPaths)
	if err != nil {
		return nil, err
	}
	for path, sig := range signals {
		if c, ok := candidates[path]; ok {
			c.Signals = sig
		}
	}

	// Fetch git history signals (non-fatal if unavailable).
	enrichWithGitSignals(idx, candidates, allPaths, subject)

	// Evidence gate: filter same-package siblings without structural evidence.
	// Runs after enrichWithGitSignals so CoChangeFreq is populated.
	filterSamePackageSiblings(candidates, subject)

	// Convert to slice (exclude subject — it's handled separately).
	result := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		result = append(result, *c)
	}
	return result, nil
}

// prioritizeByPrefix reorders siblings so that those sharing a common
// underscore-delimited prefix with the subject come first.
// For example, if subject is "pkg/node_resource_apply.go", siblings like
// "pkg/node_resource_abstract.go" (sharing "node_resource") rank before "pkg/archive.go".
func prioritizeByPrefix(siblings []string, subject string) []string {
	subjectBase := path.Base(subject)
	subjectStem := strings.TrimSuffix(subjectBase, ".go")

	var withPrefix, without []string
	for _, s := range siblings {
		if shareUnderscorePrefix(subjectStem, strings.TrimSuffix(path.Base(s), ".go")) {
			withPrefix = append(withPrefix, s)
		} else {
			without = append(without, s)
		}
	}
	return append(withPrefix, without...)
}

// shareUnderscorePrefix returns true if two filename stems (Go or Rust snake_case)
// share a common underscore-delimited prefix of at least one segment.
// "node_resource_apply" and "node_resource_abstract" share "node_resource".
// "context" and "context_apply" share "context" (single-segment prefix).
// "create" and "controller" share nothing.
func shareUnderscorePrefix(a, b string) bool {
	if a == b {
		return false
	}
	// Single-segment prefix: one stem is a complete prefix of the other at an
	// underscore boundary. E.g. "context" ↔ "context_apply", "daemon" ↔ "daemon_unix".
	if len(a) < len(b) && strings.HasPrefix(b, a+"_") {
		return true
	}
	if len(b) < len(a) && strings.HasPrefix(a, b+"_") {
		return true
	}
	// Multi-segment matching: find the longest common prefix of underscore segments.
	partsA := strings.Split(a, "_")
	partsB := strings.Split(b, "_")
	if len(partsA) < 2 || len(partsB) < 2 {
		return false
	}
	n := len(partsA)
	if len(partsB) < n {
		n = len(partsB)
	}
	shared := 0
	for i := 0; i < n; i++ {
		if partsA[i] != partsB[i] {
			break
		}
		shared++
	}
	// Require at least 1 shared segment, and at least one must have more
	// segments beyond the shared prefix (prevents identical names matching).
	return shared >= 1 && (shared < len(partsA) || shared < len(partsB)) && !(shared == len(partsA) && shared == len(partsB))
}

// javaTestMirrorDir maps a Java source directory to its corresponding test directory
// under the Maven/Gradle parallel-tree layout: src/main/java/pkg/... → src/test/java/pkg/...
// Returns "" if the directory doesn't follow the Maven/Gradle layout.
func javaTestMirrorDir(sourceDir string) string {
	mainIdx := strings.Index(sourceDir, "/src/main/java/")
	if mainIdx < 0 && strings.HasPrefix(sourceDir, "src/main/java/") {
		mainIdx = 0
	}
	if mainIdx < 0 {
		return ""
	}

	var prefix, suffix string
	if mainIdx == 0 {
		prefix = ""
		suffix = sourceDir[len("src/main/java/"):]
	} else {
		prefix = sourceDir[:mainIdx+1]
		suffix = sourceDir[mainIdx+len("/src/main/java/"):]
	}

	return prefix + "src/test/java/" + suffix
}

// splitCamelCase splits a CamelCase identifier into its constituent words.
// "KafkaProducer" → ["Kafka", "Producer"]
// "HTTPSConnection" → ["HTTPS", "Connection"]
// "URLParser" → ["URL", "Parser"]
// "AbstractWorker" → ["Abstract", "Worker"]
func splitCamelCase(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	runes := []rune(s)
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) {
			// Check if this starts a new word or continues an acronym.
			if unicode.IsLower(runes[i-1]) {
				// lowerUpper boundary: "Producer|K"
				parts = append(parts, string(runes[start:i]))
				start = i
			} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				// End of acronym: "HTT|Ps" → split before the last uppercase
				if i > start {
					parts = append(parts, string(runes[start:i]))
					start = i
				}
			}
		}
	}
	parts = append(parts, string(runes[start:]))
	return parts
}

// shareJavaClassPrefix returns true if two Java class names share a common
// CamelCase prefix of at least one word.
// "KafkaProducer" and "KafkaConsumer" share "Kafka".
// "AbstractWorker" and "AbstractHerder" share "Abstract".
// "KafkaProducer" and "KafkaProducer" returns false (identical).
func shareJavaClassPrefix(a, b string) bool {
	if a == b {
		return false
	}
	partsA := splitCamelCase(a)
	partsB := splitCamelCase(b)
	if len(partsA) < 2 || len(partsB) < 2 {
		return false
	}

	n := len(partsA)
	if len(partsB) < n {
		n = len(partsB)
	}
	shared := 0
	for i := 0; i < n; i++ {
		if partsA[i] != partsB[i] {
			break
		}
		shared++
	}
	// Require at least 1 shared word, and names must differ beyond the shared prefix.
	return shared >= 1 && (shared < len(partsA) || shared < len(partsB)) && !(shared == len(partsA) && shared == len(partsB))
}

// prioritizeByJavaClassPrefix reorders siblings so those sharing a CamelCase
// prefix with the subject come first.
func prioritizeByJavaClassPrefix(siblings []string, subject string) []string {
	subjectStem := strings.TrimSuffix(path.Base(subject), ".java")

	var withPrefix, without []string
	for _, s := range siblings {
		sStem := strings.TrimSuffix(path.Base(s), ".java")
		if shareJavaClassPrefix(subjectStem, sStem) {
			withPrefix = append(withPrefix, s)
		} else {
			without = append(without, s)
		}
	}
	return append(withPrefix, without...)
}

// rustCrateRoot extracts the crate root by finding "src/" in the path.
// "crates/bevy_ecs/src/system/mod.rs" → "crates/bevy_ecs"
// "src/sync/mutex.rs" → "."
// Returns "" if no "src/" found.
func rustCrateRoot(filePath string) string {
	// Look for /src/ in path.
	idx := strings.LastIndex(filePath, "/src/")
	if idx >= 0 {
		return filePath[:idx]
	}
	// Check if path starts with src/.
	if strings.HasPrefix(filePath, "src/") {
		return "."
	}
	return ""
}

// rustSourceModPath extracts the module path from a Rust source file within its crate.
// Strips the crate root + "/src/" prefix, the .rs extension, and trailing /mod, /lib, /main.
// "crates/bevy_ecs/src/system/function_system.rs" with root "crates/bevy_ecs" → "system/function_system"
// "crates/bevy_ecs/src/system/mod.rs" with root "crates/bevy_ecs" → "system"
// "src/sync/mutex.rs" with root "." → "sync/mutex"
func rustSourceModPath(filePath, crateRoot string) string {
	var rel string
	if crateRoot == "." {
		rel = strings.TrimPrefix(filePath, "src/")
	} else {
		rel = strings.TrimPrefix(filePath, crateRoot+"/src/")
	}
	// Strip .rs extension.
	rel = strings.TrimSuffix(rel, ".rs")
	// Strip trailing /mod, /lib, /main.
	for _, suffix := range []string{"/mod", "/lib", "/main"} {
		rel = strings.TrimSuffix(rel, suffix)
	}
	// Handle root-level lib.rs, main.rs, mod.rs → empty module path.
	if rel == "lib" || rel == "main" || rel == "mod" {
		return ""
	}
	return rel
}

// rustTestModuleMatch checks if a test file stem matches a source module path.
// The test filename flattens the module path by replacing "/" with "_".
// Returns (matches bool, allSegments bool).
// "sync_barrier" vs "sync/barrier" → (true, true) — exact match
// "sync" vs "sync/mutex" → (true, false) — prefix match, broad
// "io_read" vs "sync/mutex" → (false, false) — no match
func rustTestModuleMatch(testStem, modPath string) (bool, bool) {
	if testStem == "" || modPath == "" {
		return false, false
	}

	// Flatten module path: "system/function_system" → "system_function_system"
	flatMod := strings.ReplaceAll(modPath, "/", "_")

	// Exact match: testStem == flattened module path.
	if testStem == flatMod {
		// allSegments only true when the module path has multiple segments (≥2 path components).
		allSegs := strings.Contains(modPath, "/")
		return true, allSegs
	}

	// Suffix match: flattened module ends with "_" + testStem.
	// Handles partial-path test naming: tcp_stream → net/tcp/stream.
	if strings.HasSuffix(flatMod, "_"+testStem) {
		// Direct when test covers ≥2 segments of a ≥3 segment path.
		testParts := strings.Count(testStem, "_") + 1
		modParts := strings.Count(modPath, "/") + 1
		allSegs := testParts >= 2 && modParts >= 3
		return true, allSegs
	}

	// Prefix match: flattened module starts with testStem + "_" (broad match).
	if strings.HasPrefix(flatMod, testStem+"_") {
		return true, false
	}

	// Last-segment match: test stem starts with the last segment of the module path.
	// "pipe_subprocess" starts with "pipe" (last segment of "sys/pipe") → broad match.
	if strings.Contains(modPath, "/") {
		lastSeg := modPath[strings.LastIndex(modPath, "/")+1:]
		if testStem == lastSeg || strings.HasPrefix(testStem, lastSeg+"_") {
			return true, false
		}
	}

	return false, false
}

// rustIntegrationTestMatch checks if a test file in a crate's tests/ directory
// matches the given subject file. Returns (matches, directMatch).
func rustIntegrationTestMatch(subject, testPath string) (bool, bool) {
	crateRoot := rustCrateRoot(subject)
	if crateRoot == "" {
		return false, false
	}

	// Check test is in the crate's tests/ directory.
	var testsDir string
	if crateRoot == "." {
		testsDir = "tests/"
	} else {
		testsDir = crateRoot + "/tests/"
	}
	if !strings.HasPrefix(testPath, testsDir) {
		return false, false
	}

	// Extract source module path, with crate-name fallback for lib.rs/main.rs.
	modPath := rustSourceModPath(subject, crateRoot)
	if modPath == "" {
		modPath = rustCrateNameFromPath(crateRoot)
	}
	if modPath == "" {
		return false, false
	}

	// Extract test stem (filename without .rs).
	testBase := path.Base(testPath)
	testStem := strings.TrimSuffix(testBase, ".rs")
	if testStem == "mod" || testStem == "main" {
		return false, false
	}

	return rustTestModuleMatch(testStem, modPath)
}

// isSameCrateIntegrationTest checks if testPath is in the same crate's tests/ directory as subject.
func isSameCrateIntegrationTest(subject, testPath string) bool {
	crateRoot := rustCrateRoot(subject)
	if crateRoot == "" {
		return false
	}
	var testsDir string
	if crateRoot == "." {
		testsDir = "tests/"
	} else {
		testsDir = crateRoot + "/tests/"
	}
	return strings.HasPrefix(testPath, testsDir)
}

// rustCrateNameFromPath derives the crate name from the crate root directory path.
// Rust convention: directory name = crate name, with hyphens replaced by underscores.
// "crates/bevy_ecs" → "bevy_ecs", "src/tools/miri" → "miri", "." → ""
func rustCrateNameFromPath(crateRoot string) string {
	if crateRoot == "" || crateRoot == "." {
		return ""
	}
	return strings.ReplaceAll(path.Base(crateRoot), "-", "_")
}

// prioritizeByRustPrefix reorders siblings so those sharing an underscore-delimited
// prefix with the subject come first. Reuses shareUnderscorePrefix since Rust uses snake_case.
func prioritizeByRustPrefix(siblings []string, subject string) []string {
	subjectStem := strings.TrimSuffix(path.Base(subject), ".rs")

	var withPrefix, without []string
	for _, s := range siblings {
		if shareUnderscorePrefix(subjectStem, strings.TrimSuffix(path.Base(s), ".rs")) {
			withPrefix = append(withPrefix, s)
		} else {
			without = append(without, s)
		}
	}
	return append(withPrefix, without...)
}

// filterSamePackageSiblings removes same-package siblings that lack structural evidence.
// A sibling is kept if it has: a direct import/call edge, co-change frequency >= 2,
// or a prefix match. Siblings without evidence are removed from the candidate map.
// This runs after enrichWithGitSignals so CoChangeFreq is populated.
func filterSamePackageSiblings(candidates map[string]*candidate, subject string) {
	for p, c := range candidates {
		if p == subject || c.Distance == 0 {
			continue
		}
		// Only filter same-package siblings (Go, Java, Rust, C#).
		isSamePackage := c.IsSamePackageSibling || c.IsGoSamePackage || c.IsJavaSamePackage || c.IsRustSameModule || c.IsCSharpSameNamespace
		if !isSamePackage {
			continue
		}
		// Keep if has direct import/call edge.
		if c.IsImport || c.IsImporter {
			continue
		}
		// Keep if co-change frequency >= 2.
		if c.CoChangeFreq >= 2 {
			continue
		}
		// Keep if has prefix match.
		if c.HasPrefixMatch || c.HasJavaClassPrefix || c.HasRustModulePrefix || c.HasCSharpClassPrefix {
			continue
		}
		// No evidence: remove from candidates.
		delete(candidates, p)
	}
}

// csharpProjectInfo extracts the C# project name and the file's relative path
// within that project from a repo-relative file path. It identifies the project
// root as the deepest directory component that looks like a .NET project name
// (contains dots, e.g., "MediaBrowser.Controller" or "Orleans.Runtime").
// For simple projects without dots, uses the first meaningful directory component.
//
// Examples:
//
//	"src/EFCore.Design/Design/Internal/Foo.cs" → ("EFCore.Design", "Design/Internal")
//	"Emby.Server.Implementations/Session/SessionManager.cs" → ("Emby.Server.Implementations", "Session")
//	"src/Compilers/CSharp/Portable/Syntax/SyntaxFacts.cs" → ("CSharp", "Portable/Syntax")
//	"tests/Jellyfin.Api.Tests/Auth/HandlerTests.cs" → ("Jellyfin.Api.Tests", "Auth")
func csharpProjectInfo(filePath string) (projectName, relPath string) {
	dir := path.Dir(filePath)
	parts := strings.Split(dir, "/")

	// Find the deepest directory component that looks like a project name (contains a dot).
	bestIdx := -1
	for i, part := range parts {
		if strings.Contains(part, ".") && part != "." {
			bestIdx = i
		}
	}

	if bestIdx >= 0 {
		projectName = parts[bestIdx]
		if bestIdx+1 < len(parts) {
			relPath = strings.Join(parts[bestIdx+1:], "/")
		}
		return projectName, relPath
	}

	// Fallback: no dotted directory. Use heuristics for repos like Roslyn
	// where projects are at paths like src/Compilers/CSharp/Portable/.
	// Skip common root dirs and use the first "interesting" component.
	for i, part := range parts {
		if part == "src" || part == "test" || part == "tests" || part == "." {
			continue
		}
		// Use this component as the project name.
		projectName = part
		if i+1 < len(parts) {
			relPath = strings.Join(parts[i+1:], "/")
		}
		return projectName, relPath
	}

	return "", ""
}

// enrichWithGitSignals populates churn and co-change data on candidates.
// Failures are silently ignored (graceful degradation to structural-only).
func enrichWithGitSignals(idx *db.Index, candidates map[string]*candidate, allPaths []string, subject string) {
	const windowDays = 90

	// Get max churn for normalization.
	maxChurn, err := idx.GetMaxChurn(windowDays)
	if err != nil || maxChurn == 0 {
		return
	}

	// Get churn values for all candidate paths.
	churnMap, err := idx.GetChurnForFiles(allPaths, windowDays)
	if err != nil {
		return
	}
	for path, value := range churnMap {
		if c, ok := candidates[path]; ok {
			c.Churn = value / maxChurn
		}
	}

	// Get co-change partners for the subject file (no limit — data is already
	// bounded by MaxCommitSize * WindowDays, and we only apply freq to existing candidates).
	partners, err := idx.GetCoChangePartners(subject, windowDays, 0)
	if err != nil {
		return
	}
	for _, p := range partners {
		if c, ok := candidates[p.Path]; ok {
			c.CoChangeFreq = p.Frequency
		}
	}
}
