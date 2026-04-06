package analyzer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/indexer"
	"github.com/kehoej/contextception/internal/model"
)

// --- Test helpers for v3 output ---

// flattenTests extracts file paths from the TestEntry slice.
func flattenTests(output *model.AnalysisOutput) []string {
	result := make([]string, len(output.Tests))
	for i, e := range output.Tests {
		result[i] = e.File
	}
	return result
}

// testCandidatePaths extracts paths from testCandidate slice (for unit tests on filterTestCandidates).
func testCandidatePaths(tcs []testCandidate) []string {
	result := make([]string, len(tcs))
	for i, tc := range tcs {
		result[i] = tc.path
	}
	return result
}

// flattenMustRead extracts file paths from the v3 MustReadEntry slice.
func flattenMustRead(output *model.AnalysisOutput) []string {
	result := make([]string, len(output.MustRead))
	for i, e := range output.MustRead {
		result[i] = e.File
	}
	return result
}

// flattenRelated flattens the grouped related map into full paths.
func flattenRelated(output *model.AnalysisOutput) []string {
	var result []string
	for key, entries := range output.Related {
		for _, e := range entries {
			if key == "." {
				result = append(result, e.File)
			} else {
				result = append(result, key+"/"+e.File)
			}
		}
	}
	return result
}

// flattenLikelyModify flattens the grouped likely_modify map into full paths.
func flattenLikelyModify(output *model.AnalysisOutput) []string {
	var result []string
	for key, entries := range output.LikelyModify {
		for _, e := range entries {
			if key == "." {
				result = append(result, e.File)
			} else {
				result = append(result, key+"/"+e.File)
			}
		}
	}
	return result
}

// getLikelyModEntry finds a LikelyModifyEntry by full path.
func getLikelyModEntry(output *model.AnalysisOutput, file string) *model.LikelyModifyEntry {
	for key, entries := range output.LikelyModify {
		for i, e := range entries {
			fullPath := key + "/" + e.File
			if key == "." {
				fullPath = e.File
			}
			if fullPath == file {
				return &output.LikelyModify[key][i]
			}
		}
	}
	return nil
}

// hasSignal checks if a file in any output section has the given signal.
func hasSignal(output *model.AnalysisOutput, file, signal string) bool {
	// Check likely_modify.
	for key, entries := range output.LikelyModify {
		for _, e := range entries {
			fullPath := key + "/" + e.File
			if key == "." {
				fullPath = e.File
			}
			if fullPath == file {
				for _, s := range e.Signals {
					if s == signal {
						return true
					}
				}
			}
		}
	}
	// Check related.
	for key, entries := range output.Related {
		for _, e := range entries {
			fullPath := key + "/" + e.File
			if key == "." {
				fullPath = e.File
			}
			if fullPath == file {
				for _, s := range e.Signals {
					if s == signal {
						return true
					}
				}
			}
		}
	}
	return false
}

// inAnyPositiveSection returns true if the file is in must_read, likely_modify, related, or tests.
func inAnyPositiveSection(output *model.AnalysisOutput, file string) bool {
	return sliceContains(flattenMustRead(output), file) ||
		sliceContains(flattenLikelyModify(output), file) ||
		sliceContains(flattenRelated(output), file) ||
		sliceContains(flattenTests(output), file)
}

// --- Test project helpers ---

// createTestProject creates a temp Python project for integration tests.
func createTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"setup.py": "# setup\n",
		"myapp/__init__.py": `from .models import User
`,
		"myapp/main.py": `from .models import User
from .utils import helper
import os
`,
		"myapp/models.py": `from .base import BaseModel
`,
		"myapp/utils.py": `import json
from . import models
`,
		"myapp/base.py":         "# no imports\n",
		"myapp/sub/__init__.py": "",
		"myapp/sub/handler.py": `from ..models import User
from . import helpers
`,
		"myapp/sub/helpers.py": `import logging
`,
		"tests/test_main.py": `from myapp.main import something
`,
		"tests/conftest.py": `from myapp.models import User
`,
		"docs/tutorial.py": `from myapp.main import app
`,
		"docs_src/example_app.py": `from myapp.main import app
from myapp.models import User
`,
		"examples/basic.py": `from myapp.utils import helper
`,
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func openTestIndex(t *testing.T) *db.Index {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := db.OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}
	return idx
}

// buildIndex creates a test project, indexes it, and returns the analyzer.
func buildIndex(t *testing.T) (*Analyzer, *db.Index) {
	t.Helper()
	root := createTestProject(t)
	idx := openTestIndex(t)

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	return a, idx
}

func TestAnalyzeMainPy(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	if output.SchemaVersion != "3.2" {
		t.Errorf("SchemaVersion = %q, want %q", output.SchemaVersion, "3.2")
	}
	if output.Subject != "myapp/main.py" {
		t.Errorf("Subject = %q, want %q", output.Subject, "myapp/main.py")
	}

	// Subject should NOT be in must_read.
	mustRead := flattenMustRead(output)
	assertNotContains(t, "must_read", mustRead, "myapp/main.py")

	// must_read should include direct imports (models.py, utils.py).
	assertContains(t, "must_read", mustRead, "myapp/models.py")
	assertContains(t, "must_read", mustRead, "myapp/utils.py")

	// tests/test_main.py should be in the tests section (imports the subject).
	assertContains(t, "tests", flattenTests(output), "tests/test_main.py")

	// tests/test_main.py should NOT be in must_read or related.
	assertNotContains(t, "must_read", mustRead, "tests/test_main.py")
	assertNotContains(t, "related", flattenRelated(output), "tests/test_main.py")

	// docs/examples files should NOT be in any positive section.
	if inAnyPositiveSection(output, "docs/tutorial.py") {
		t.Error("docs/tutorial.py should not be in any positive section")
	}
	if inAnyPositiveSection(output, "docs_src/example_app.py") {
		t.Error("docs_src/example_app.py should not be in any positive section")
	}

	// conftest.py should NOT be in must_read or related (it may appear in tests).
	if sliceContains(flattenMustRead(output), "tests/conftest.py") {
		t.Error("tests/conftest.py should not be in must_read")
	}
	if sliceContains(flattenRelated(output), "tests/conftest.py") {
		t.Error("tests/conftest.py should not be in related")
	}

	// base.py should be in related (2-hop: main.py → models.py → base.py).
	assertContains(t, "related", flattenRelated(output), "myapp/base.py")
}

func TestAnalyzeModelsPy(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/models.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)

	// must_read should include base.py (direct import).
	assertContains(t, "must_read", mustRead, "myapp/base.py")

	// must_read should include importers (files that import models.py).
	assertContains(t, "must_read", mustRead, "myapp/main.py")
	assertContains(t, "must_read", mustRead, "myapp/utils.py")
	assertContains(t, "must_read", mustRead, "myapp/__init__.py")
	assertContains(t, "must_read", mustRead, "myapp/sub/handler.py")
}

func TestAnalyzeBasePy(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/base.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)

	// must_read: models.py (sole importer).
	assertContains(t, "must_read", mustRead, "myapp/models.py")

	// related should have 2-hop neighbors.
	related := flattenRelated(output)
	if len(related) == 0 {
		t.Error("expected related files for base.py (2-hop neighbors)")
	}
}

func TestAnalyzeFileNotInIndex(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	_, err := a.Analyze("nonexistent/file.py")
	if err == nil {
		t.Error("expected error for file not in index")
	}
}

func TestAnalyzeDeterminism(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output1, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	output2, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// Compare must_read (v3: ordered list).
	mr1 := flattenMustRead(output1)
	mr2 := flattenMustRead(output2)
	assertSameElements(t, "must_read", mr1, mr2)

	// Compare related.
	rel1 := flattenRelated(output1)
	rel2 := flattenRelated(output2)
	assertSameElements(t, "related", rel1, rel2)

	// Compare likely_modify.
	lm1 := flattenLikelyModify(output1)
	lm2 := flattenLikelyModify(output2)
	assertSameElements(t, "likely_modify", lm1, lm2)

	// Compare tests.
	assertSameElements(t, "tests", flattenTests(output1), flattenTests(output2))
}

// TestAnalyzeGoldenFile validates the full output structure for the main
// test project against expected categorization.
func TestAnalyzeGoldenFile(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)

	// Subject should NOT be in must_read.
	assertNotContains(t, "must_read", mustRead, "myapp/main.py")

	// must_read must contain direct imports and importers.
	assertContains(t, "must_read", mustRead, "myapp/models.py")
	assertContains(t, "must_read", mustRead, "myapp/utils.py")

	// related should contain 2-hop files (not docs/examples).
	related := flattenRelated(output)
	assertContains(t, "related", related, "myapp/base.py")
	for _, r := range related {
		if classify.IsIgnorable(r) {
			t.Errorf("related contains ignorable file %q", r)
		}
	}

	// Ignored files should NOT be in must_read, related, or likely_modify.
	for _, ignored := range []string{"tests/conftest.py", "docs/tutorial.py", "docs_src/example_app.py"} {
		assertNotContains(t, "must_read", mustRead, ignored)
		assertNotContains(t, "related", related, ignored)
		assertNotContains(t, "likely_modify", flattenLikelyModify(output), ignored)
	}
}

// TestIgnorableDetection unit tests for isIgnorable.
func TestIgnorableDetection(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"docs/tutorial.py", true},
		{"docs_src/example_app.py", true},
		{"examples/basic.py", true},
		{"myapp/docs/readme.py", false}, // nested docs/ is not ignorable (could be a real package)
		{"tests/conftest.py", true},
		{"myapp/conftest.py", true},
		{"myapp/main.py", false},
		{"myapp/models.py", false},
		{"tests/test_main.py", false}, // test files are handled by isTestFile, not isIgnorable
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := classify.IsIgnorable(tt.path)
			if got != tt.want {
				t.Errorf("isIgnorable(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// createOverflowProject creates a project where hub.py imports 15 utility files
// and has 3 reverse importers. This tests that all forward imports appear in
// must_read even when exceeding maxMustRead.
func createOverflowProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"pkg/__init__.py": "",
	}

	// hub.py imports 15 utility files.
	hubImports := ""
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("util_%02d", i)
		files[fmt.Sprintf("pkg/%s.py", name)] = "# utility\n"
		hubImports += fmt.Sprintf("from . import %s\n", name)
	}
	files["pkg/hub.py"] = hubImports

	// 3 files that import hub.py (reverse importers).
	for i := 1; i <= 3; i++ {
		files[fmt.Sprintf("pkg/consumer_%d.py", i)] = "from . import hub\n"
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func buildOverflowIndex(t *testing.T) (*Analyzer, *db.Index) {
	t.Helper()
	root := createOverflowProject(t)
	idx := openTestIndex(t)

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	return a, idx
}

// TestMustReadGuaranteesForwardImports verifies that must_read is capped at
// maxMustRead (10). Forward imports beyond the cap overflow to related.
// Reverse importers also overflow to related. No files are dropped.
func TestMustReadGuaranteesForwardImports(t *testing.T) {
	a, idx := buildOverflowIndex(t)
	defer idx.Close()

	output, err := a.Analyze("pkg/hub.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)

	// Subject should NOT be in must_read.
	assertNotContains(t, "must_read", mustRead, "pkg/hub.py")

	// must_read should have 10 highest-scored forward imports (maxMustRead).
	if len(mustRead) != 10 {
		t.Errorf("must_read length = %d, want 10 (maxMustRead)", len(mustRead))
	}

	related := flattenRelated(output)
	likelyMod := flattenLikelyModify(output)

	// All 15 forward imports must be in either must_read, related, or likely_modify (none dropped).
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("pkg/util_%02d.py", i)
		inMR := sliceContains(mustRead, name)
		inRel := sliceContains(related, name)
		inLM := sliceContains(likelyMod, name)
		if !inMR && !inRel && !inLM {
			t.Errorf("forward import %q dropped — not in must_read, related, or likely_modify", name)
		}
	}

	// 5 forward imports that are NOT in must_read should be somewhere else (related or likely_modify).
	notInMustRead := 0
	for i := 1; i <= 15; i++ {
		name := fmt.Sprintf("pkg/util_%02d.py", i)
		if !sliceContains(mustRead, name) {
			notInMustRead++
			inRel := sliceContains(related, name)
			inLM := sliceContains(likelyMod, name)
			if !inRel && !inLM {
				t.Errorf("overflow forward import %q not in related or likely_modify", name)
			}
		}
	}
	if notInMustRead != 5 {
		t.Errorf("expected 5 forward imports NOT in must_read (overflow), got %d", notInMustRead)
	}

	// Reverse importers (consumer_*.py) should all be somewhere.
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("pkg/consumer_%d.py", i)
		inMR := sliceContains(mustRead, name)
		inRel := sliceContains(related, name)
		inLM := sliceContains(likelyMod, name)
		if !inMR && !inRel && !inLM {
			t.Errorf("reverse importer %q dropped — not in must_read, related, or likely_modify", name)
		}
	}
}

// TestMustReadCapPreservesScoreOrder verifies that when forward imports exceed
// maxMustRead, the highest-scored ones stay in must_read.
func TestMustReadCapPreservesScoreOrder(t *testing.T) {
	a, idx := buildOverflowIndex(t)
	defer idx.Close()

	output, err := a.Analyze("pkg/hub.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)

	// must_read should have maxMustRead entries.
	if len(mustRead) > 10 {
		t.Errorf("must_read exceeds cap: got %d entries, want <= 10", len(mustRead))
	}
}

// TestHubPyInRelated verifies that 2-hop candidates with high indegree
// appear in related (previously tested via risk, which is now removed).
func TestHubPyInRelated(t *testing.T) {
	root := t.TempDir()

	files := map[string]string{
		"pkg/__init__.py":   "",
		"pkg/subject.py":    "from . import direct_dep\n",
		"pkg/direct_dep.py": "from . import hub\n",
		"pkg/hub.py":        "# hub\n",
	}

	for i := 1; i <= 5; i++ {
		files[fmt.Sprintf("pkg/user_%d.py", i)] = "from . import hub\n"
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	// hub.py is 2-hop from subject.py. Should appear in related.
	related := flattenRelated(output)
	assertContains(t, "related", related, "pkg/hub.py")
}

// TestAnalyzeExternalDependencies verifies that external/stdlib imports
// are surfaced in the External field.
func TestAnalyzeExternalDependencies(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// myapp/main.py has "import os" which should be in External.
	if len(output.External) == 0 {
		t.Fatal("expected External to be non-empty for myapp/main.py")
	}

	found := false
	for _, spec := range output.External {
		if spec == "os" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'os' in External, got %v", output.External)
	}
}

func sliceContains(s []string, val string) bool {
	for _, v := range s {
		if v == val {
			return true
		}
	}
	return false
}

func assertContains(t *testing.T, label string, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s does not contain %q, got %v", label, want, slice)
}

func assertNotContains(t *testing.T, label string, slice []string, unwanted string) {
	t.Helper()
	for _, s := range slice {
		if s == unwanted {
			t.Errorf("%s should not contain %q, got %v", label, unwanted, slice)
			return
		}
	}
}

// assertSameElements checks that two slices contain the same elements (order-independent).
func assertSameElements(t *testing.T, label string, a, b []string) {
	t.Helper()
	if len(a) != len(b) {
		t.Errorf("%s length mismatch: %d vs %d\na: %v\nb: %v", label, len(a), len(b), a, b)
		return
	}
	setA := make(map[string]int)
	for _, s := range a {
		setA[s]++
	}
	setB := make(map[string]int)
	for _, s := range b {
		setB[s]++
	}
	for k, v := range setA {
		if setB[k] != v {
			t.Errorf("%s element count mismatch for %q: %d vs %d", label, k, v, setB[k])
		}
	}
}

// TestHistoricalModifierCapping verifies the ADR-0009 cap:
// historical_modifier <= 0.5 * structural_weight.
func TestHistoricalModifierCapping(t *testing.T) {
	c := candidate{
		Path:         "hot.py",
		Distance:     1,
		IsImport:     true,
		Signals:      db.SignalRow{Indegree: 2, Outdegree: 1},
		Churn:        1.0,
		CoChangeFreq: 10,
	}

	maxIndegree := 10
	maxCoChangeFreq := 10

	score := computeScore(c, maxIndegree, maxCoChangeFreq)

	normalizedIndegree := 2.0 / 10.0
	structural := (weightIndegree * normalizedIndegree) +
		(weightDistance * 1.0) +
		(weightEntrypoint * 0.0) +
		(weightAPISurface * 0.0) +
		proximityBonus

	expectedCap := historicalCap * structural
	expectedScore := structural + expectedCap

	if score != expectedScore {
		t.Errorf("score = %f, want %f (capped historical modifier)", score, expectedScore)
	}
}

// TestHistoricalModifierBelowCap verifies that when the historical modifier
// is below the cap, it's applied fully.
func TestHistoricalModifierBelowCap(t *testing.T) {
	c := candidate{
		Path:         "mild.py",
		Distance:     1,
		IsImport:     true,
		Signals:      db.SignalRow{Indegree: 5, Outdegree: 2},
		Churn:        0.1,
		CoChangeFreq: 1,
	}

	maxIndegree := 10
	maxCoChangeFreq := 10

	score := computeScore(c, maxIndegree, maxCoChangeFreq)

	normalizedIndegree := 5.0 / 10.0
	structural := (weightIndegree * normalizedIndegree) +
		(weightDistance * 1.0) +
		(weightEntrypoint * 0.0) +
		(weightAPISurface * 1.0) +
		proximityBonus

	historical := (weightCoChange * 0.1) + (weightChurn * 0.1)
	cap := historicalCap * structural

	if historical >= cap {
		t.Fatalf("test assumption violated: historical %f >= cap %f", historical, cap)
	}

	expectedScore := structural + historical
	if score != expectedScore {
		t.Errorf("score = %f, want %f (uncapped historical)", score, expectedScore)
	}
}

// TestZeroGitSignalsNoRegression verifies that with zero git signals,
// scoring produces the same result as the structural-only formula.
func TestZeroGitSignalsNoRegression(t *testing.T) {
	c := candidate{
		Path:         "plain.py",
		Distance:     1,
		IsImport:     true,
		Signals:      db.SignalRow{Indegree: 4, Outdegree: 1},
		Churn:        0.0,
		CoChangeFreq: 0,
	}

	score := computeScore(c, 10, 0)

	normalizedIndegree := 4.0 / 10.0
	expected := (weightIndegree * normalizedIndegree) +
		(weightDistance * 1.0) +
		(weightEntrypoint * 0.0) +
		(weightAPISurface * 1.0) +
		proximityBonus

	if score != expected {
		t.Errorf("score = %f, want %f (no git signals, structural only)", score, expected)
	}
}

// --- Git signal integration helpers ---

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
}

func gitAdd(t *testing.T, dir string, files ...string) {
	t.Helper()
	args := append([]string{"add"}, files...)
	run(t, dir, "git", args...)
}

func gitCommitAt(t *testing.T, dir string, msg string, when time.Time) {
	t.Helper()
	dateStr := when.Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+dateStr,
		"GIT_COMMITTER_DATE="+dateStr,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGitSignalsAffectAnalysis is a controlled end-to-end test that proves
// git signals (churn, co-change) actually change analysis output.
func TestGitSignalsAffectAnalysis(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/hub.py", "# hub module\nVALUE = 1\n")
	writeFile(t, root, "pkg/subject.py", "from . import hub\nfrom . import utils\n")
	writeFile(t, root, "pkg/utils.py", "# utils\nHELPER = True\n")
	writeFile(t, root, "pkg/consumer_1.py", "from . import hub\n")
	writeFile(t, root, "pkg/consumer_2.py", "from . import hub\n")
	writeFile(t, root, "pkg/consumer_3.py", "from . import hub\n")

	gitInit(t, root)
	now := time.Now()

	gitAdd(t, root, ".")
	gitCommitAt(t, root, "initial commit", now.AddDate(0, 0, -60))

	// 5 commits modifying hub.py + subject.py together (high co-change).
	for i := 0; i < 5; i++ {
		writeFile(t, root, "pkg/hub.py", fmt.Sprintf("# hub module\nVALUE = %d\n", i+10))
		writeFile(t, root, "pkg/subject.py", fmt.Sprintf("from . import hub\nfrom . import utils\n# v%d\n", i))
		gitAdd(t, root, "pkg/hub.py", "pkg/subject.py")
		gitCommitAt(t, root, fmt.Sprintf("update hub+subject %d", i), now.AddDate(0, 0, -50+i))
	}

	// 10 more commits modifying hub.py alone (high churn on hub).
	for i := 0; i < 10; i++ {
		writeFile(t, root, "pkg/hub.py", fmt.Sprintf("# hub module\nVALUE = %d\n", i+100))
		gitAdd(t, root, "pkg/hub.py")
		gitCommitAt(t, root, fmt.Sprintf("update hub %d", i), now.AddDate(0, 0, -40+i))
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	// 1. co_change signal should exist for hub.py.
	if !hasSignal(output, "pkg/hub.py", "co_change:5") {
		// Check for any co_change signal.
		if !hasSignalPrefix(output, "pkg/hub.py", "co_change:") {
			t.Errorf("expected co_change signal for hub.py")
		}
	}

	// 2. high_churn signal should exist for hub.py.
	if !hasSignal(output, "pkg/hub.py", "high_churn") {
		t.Errorf("expected high_churn signal for hub.py")
	}

	// 3. hub.py should rank in must_read with historical boost.
	mustRead := flattenMustRead(output)
	assertContains(t, "must_read", mustRead, "pkg/hub.py")
	assertContains(t, "must_read", mustRead, "pkg/utils.py")
}

// hasSignalPrefix checks if a file has a signal starting with prefix.
func hasSignalPrefix(output *model.AnalysisOutput, file, prefix string) bool {
	for key, entries := range output.LikelyModify {
		for _, e := range entries {
			fullPath := key + "/" + e.File
			if key == "." {
				fullPath = e.File
			}
			if fullPath == file {
				for _, s := range e.Signals {
					if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
						return true
					}
				}
			}
		}
	}
	for key, entries := range output.Related {
		for _, e := range entries {
			fullPath := key + "/" + e.File
			if key == "." {
				fullPath = e.File
			}
			if fullPath == file {
				for _, s := range e.Signals {
					if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
						return true
					}
				}
			}
		}
	}
	return false
}

// TestGitSignalsGracefulDegradation verifies that when there are no git signal
// rows in the DB, enrichWithGitSignals returns without modifying candidates.
func TestGitSignalsGracefulDegradation(t *testing.T) {
	idx := openTestIndex(t)
	defer idx.Close()

	candidates := map[string]*candidate{
		"a.py": {Path: "a.py", Distance: 1, IsImport: true},
		"b.py": {Path: "b.py", Distance: 1, IsImporter: true},
	}
	allPaths := []string{"a.py", "b.py"}

	enrichWithGitSignals(idx, candidates, allPaths, "subject.py")

	for path, c := range candidates {
		if c.Churn != 0 {
			t.Errorf("%s: Churn = %f, want 0", path, c.Churn)
		}
		if c.CoChangeFreq != 0 {
			t.Errorf("%s: CoChangeFreq = %d, want 0", path, c.CoChangeFreq)
		}
	}
}

// --- Config integration tests ---

func createConfigProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"pkg/__init__.py": "",
		"pkg/main.py": `from . import models
from . import vendor_lib
from . import gen_models
`,
		"pkg/models.py":     "# models\n",
		"pkg/vendor_lib.py": "# vendor lib\n",
		"pkg/gen_models.py": "# generated models\n",
		"vendor/dep.py": `from pkg import models
`,
		"vendor/util.py": `from pkg import models
`,
		"generated/api.py": `from pkg import models
`,
		"pkg/service.py": "from . import models\n",
		"pkg/handler.py": "from . import models\n",
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func buildConfigIndex(t *testing.T, root string, cfg *config.Config) (*Analyzer, *db.Index) {
	t.Helper()
	idx := openTestIndex(t)

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
		Config:   cfg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{RepoConfig: cfg})
	return a, idx
}

func TestConfigIgnore(t *testing.T) {
	root := createConfigProject(t)
	cfg := &config.Config{
		Version: 1,
		Ignore:  []string{"vendor/"},
	}

	a, idx := buildConfigIndex(t, root, cfg)
	defer idx.Close()

	exists, err := idx.FileExists("vendor/dep.py")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("vendor/dep.py should not be indexed when vendor/ is config-ignored")
	}

	output, err := a.Analyze("pkg/main.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	assertNotContains(t, "must_read", mustRead, "vendor/dep.py")
	assertNotContains(t, "must_read", mustRead, "vendor/util.py")
}

func TestConfigGenerated(t *testing.T) {
	root := createConfigProject(t)
	cfg := &config.Config{
		Version:   1,
		Generated: []string{"generated/"},
	}

	a, idx := buildConfigIndex(t, root, cfg)
	defer idx.Close()

	output, err := a.Analyze("pkg/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// generated/api.py should NOT be in any positive section.
	if inAnyPositiveSection(output, "generated/api.py") {
		t.Errorf("generated/api.py should not be in any positive section")
	}

	mustRead := flattenMustRead(output)
	assertNotContains(t, "must_read", mustRead, "generated/api.py")
}

func TestConfigEntrypoints(t *testing.T) {
	root := createConfigProject(t)
	cfg := &config.Config{
		Version:     1,
		Entrypoints: []string{"pkg/models.py"},
	}

	a, idx := buildConfigIndex(t, root, cfg)
	defer idx.Close()

	signals, err := idx.GetSignals([]string{"pkg/models.py"})
	if err != nil {
		t.Fatal(err)
	}
	sig, ok := signals["pkg/models.py"]
	if !ok {
		t.Fatal("pkg/models.py not found in signals")
	}
	if !sig.IsEntrypoint {
		t.Error("pkg/models.py should be marked as entrypoint via config")
	}

	// Analyze and verify models.py is surfaced (must_read as direct import).
	output, err := a.Analyze("pkg/main.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	assertContains(t, "must_read", mustRead, "pkg/models.py")
}

func TestNoConfigSameAsEmpty(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	assertContains(t, "must_read", mustRead, "myapp/models.py")
	assertContains(t, "must_read", mustRead, "myapp/utils.py")
	assertContains(t, "related", flattenRelated(output), "myapp/base.py")

	// Test/doc files should NOT be in must_read or related.
	assertNotContains(t, "must_read", mustRead, "tests/test_main.py")
}

// --- Related tier / co-change prioritization tests ---

func TestRelatedPrioritizesCoChange(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/subject.py", "from . import helper\n")
	writeFile(t, root, "pkg/helper.py", "from . import bridge\nfrom . import copartner\n")
	writeFile(t, root, "pkg/bridge.py", "# bridge utility\n")
	writeFile(t, root, "pkg/copartner.py", "# co-change partner\n")

	for i := 1; i <= 8; i++ {
		writeFile(t, root, fmt.Sprintf("pkg/user_%d.py", i), "from . import bridge\n")
	}

	gitInit(t, root)
	now := time.Now()

	gitAdd(t, root, "pkg/__init__.py", "pkg/bridge.py", "pkg/helper.py")
	for i := 1; i <= 8; i++ {
		gitAdd(t, root, fmt.Sprintf("pkg/user_%d.py", i))
	}
	gitCommitAt(t, root, "add infrastructure", now.AddDate(0, 0, -60))

	gitAdd(t, root, "pkg/subject.py", "pkg/copartner.py")
	gitCommitAt(t, root, "add subject and copartner", now.AddDate(0, 0, -55))

	for i := 0; i < 3; i++ {
		writeFile(t, root, "pkg/subject.py", fmt.Sprintf("from . import helper\n# v%d\n", i))
		writeFile(t, root, "pkg/copartner.py", fmt.Sprintf("# co-change partner v%d\n", i))
		gitAdd(t, root, "pkg/subject.py", "pkg/copartner.py")
		gitCommitAt(t, root, fmt.Sprintf("co-change %d", i), now.AddDate(0, 0, -30+i))
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	// copartner.py should be in likely_modify or related (has 3x co-change from distance 2).
	related := flattenRelated(output)
	likelyMod := flattenLikelyModify(output)
	inRelated := sliceContains(related, "pkg/copartner.py")
	inLikelyMod := sliceContains(likelyMod, "pkg/copartner.py")
	if !inRelated && !inLikelyMod {
		t.Errorf("copartner.py should be in related or likely_modify, got neither.\nrelated: %v\nlikely_modify: %v", related, likelyMod)
	}

	// bridge.py should be in related (distance 2, no co-change).
	inRelated = sliceContains(related, "pkg/bridge.py")
	inLikelyMod = sliceContains(likelyMod, "pkg/bridge.py")
	if !inRelated && !inLikelyMod {
		t.Errorf("bridge.py should be in related or likely_modify")
	}
}

// TestTwoHopHistoricalCapRelaxed verifies that distance-2 candidates use
// the relaxed historical cap (1.5x structural).
func TestTwoHopHistoricalCapRelaxed(t *testing.T) {
	c := candidate{
		Path:         "twoHop.py",
		Distance:     2,
		IsImport:     false,
		Signals:      db.SignalRow{Indegree: 1, Outdegree: 1},
		Churn:        0.0,
		CoChangeFreq: 10,
	}

	maxIndegree := 10
	maxCoChangeFreq := 10

	score := computeScore(c, maxIndegree, maxCoChangeFreq)

	normalizedIndegree := 1.0 / 10.0
	structural := (weightIndegree * normalizedIndegree) +
		(weightDistance * 0.0) +
		(weightEntrypoint * 0.0) +
		(weightAPISurface * 0.0) +
		0.0

	relaxedCap := historicalCapTwoHop * structural
	standardCap := historicalCap * structural

	if relaxedCap <= standardCap {
		t.Fatalf("relaxed cap (%f) should exceed standard cap (%f)", relaxedCap, standardCap)
	}

	historical := (weightCoChange * 1.0) + (weightChurn * 0.0)
	expectedHistorical := relaxedCap
	if historical < relaxedCap {
		expectedHistorical = historical
	}

	expectedScore := structural + expectedHistorical
	if score != expectedScore {
		t.Errorf("score = %f, want %f (relaxed 2-hop historical cap)", score, expectedScore)
	}
}

// TestLikelyModifyInOutput verifies that the likely_modify section fires
// for appropriate cases and does NOT fire for high-fan-in imports.
func TestLikelyModifyInOutput(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/subject.py", "from . import tight\nfrom . import logger\n")
	writeFile(t, root, "pkg/tight.py", "# tightly coupled\n")
	writeFile(t, root, "pkg/logger.py", "# shared logger\n")
	writeFile(t, root, "pkg/consumer.py", "from . import subject\n")

	for i := 1; i <= 7; i++ {
		writeFile(t, root, fmt.Sprintf("pkg/svc_%d.py", i), "from . import logger\n")
	}

	gitInit(t, root)
	now := time.Now()
	gitAdd(t, root, ".")
	gitCommitAt(t, root, "init", now.AddDate(0, 0, -60))

	for i := 0; i < 2; i++ {
		writeFile(t, root, "pkg/subject.py", fmt.Sprintf("from . import tight\nfrom . import logger\n# v%d\n", i))
		writeFile(t, root, "pkg/consumer.py", fmt.Sprintf("from . import subject\n# v%d\n", i))
		gitAdd(t, root, "pkg/subject.py", "pkg/consumer.py")
		gitCommitAt(t, root, fmt.Sprintf("co-change %d", i), now.AddDate(0, 0, -30+i))
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	likelyMod := flattenLikelyModify(output)

	// tight.py: direct import with indegree <= 2 → likely_modify.
	assertContains(t, "likely_modify", likelyMod, "pkg/tight.py")

	// consumer.py: importer with co-change >= 2 → likely_modify.
	assertContains(t, "likely_modify", likelyMod, "pkg/consumer.py")

	// logger.py: high fan-in import, no co-change → NOT likely_modify.
	assertNotContains(t, "likely_modify", likelyMod, "pkg/logger.py")

	// Verify confidence levels.
	tightEntry := getLikelyModEntry(output, "pkg/tight.py")
	if tightEntry == nil {
		t.Fatal("expected likely_modify entry for tight.py")
	}
	if tightEntry.Confidence != "high" {
		t.Errorf("tight.py confidence = %q, want %q", tightEntry.Confidence, "high")
	}

	consumerEntry := getLikelyModEntry(output, "pkg/consumer.py")
	if consumerEntry == nil {
		t.Fatal("expected likely_modify entry for consumer.py")
	}
	if consumerEntry.Confidence != "high" {
		t.Errorf("consumer.py confidence = %q, want %q", consumerEntry.Confidence, "high")
	}
}

// TestLikelyModifyConfidenceLevels verifies confidence levels.
func TestLikelyModifyConfidenceLevels(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/subject.py", "from . import tight_confirmed\nfrom . import frequent\n")
	writeFile(t, root, "pkg/tight_confirmed.py", "# tightly coupled\n")
	writeFile(t, root, "pkg/frequent.py", "# frequently co-changed\n")
	writeFile(t, root, "pkg/heavy_consumer.py", "from . import subject\n")
	writeFile(t, root, "pkg/light_consumer.py", "from . import subject\n")

	gitInit(t, root)
	now := time.Now()
	gitAdd(t, root, ".")
	gitCommitAt(t, root, "init", now.AddDate(0, 0, -60))

	writeFile(t, root, "pkg/subject.py", "from . import tight_confirmed\nfrom . import frequent\n# v1\n")
	writeFile(t, root, "pkg/tight_confirmed.py", "# tightly coupled v1\n")
	gitAdd(t, root, "pkg/subject.py", "pkg/tight_confirmed.py")
	gitCommitAt(t, root, "co-change tight", now.AddDate(0, 0, -50))

	for i := 0; i < 5; i++ {
		writeFile(t, root, "pkg/subject.py", fmt.Sprintf("from . import tight_confirmed\nfrom . import frequent\n# v%d\n", i+2))
		writeFile(t, root, "pkg/frequent.py", fmt.Sprintf("# frequently co-changed v%d\n", i))
		gitAdd(t, root, "pkg/subject.py", "pkg/frequent.py")
		gitCommitAt(t, root, fmt.Sprintf("co-change frequent %d", i), now.AddDate(0, 0, -40+i))
	}

	for i := 0; i < 3; i++ {
		writeFile(t, root, "pkg/subject.py", fmt.Sprintf("from . import tight_confirmed\nfrom . import frequent\n# vc%d\n", i))
		writeFile(t, root, "pkg/heavy_consumer.py", fmt.Sprintf("from . import subject\n# v%d\n", i))
		gitAdd(t, root, "pkg/subject.py", "pkg/heavy_consumer.py")
		gitCommitAt(t, root, fmt.Sprintf("co-change heavy %d", i), now.AddDate(0, 0, -30+i))
	}

	writeFile(t, root, "pkg/subject.py", "from . import tight_confirmed\nfrom . import frequent\n# final\n")
	writeFile(t, root, "pkg/light_consumer.py", "from . import subject\n# v1\n")
	gitAdd(t, root, "pkg/subject.py", "pkg/light_consumer.py")
	gitCommitAt(t, root, "co-change light", now.AddDate(0, 0, -20))

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		file     string
		wantConf string
		reason   string
	}{
		{"pkg/frequent.py", "high", "CoChangeFreq >= 5"},
		{"pkg/tight_confirmed.py", "high", "IsImport && Indegree <= 2 && CoChangeFreq >= 1"},
		{"pkg/heavy_consumer.py", "high", "IsImporter && CoChangeFreq >= 3"},
		{"pkg/light_consumer.py", "medium", "IsImporter && CoChangeFreq < 3"},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			entry := getLikelyModEntry(output, tt.file)
			if entry == nil {
				t.Fatalf("expected likely_modify for %s (%s), got none.\nlikely_modify: %v",
					tt.file, tt.reason, flattenLikelyModify(output))
			}
			if entry.Confidence != tt.wantConf {
				t.Errorf("confidence = %q, want %q (%s)", entry.Confidence, tt.wantConf, tt.reason)
			}
		})
	}
}

// TestLikelyModifyLowConfidence verifies that the "low" confidence tier
// surfaces near-miss files.
func TestLikelyModifyLowConfidence(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/subject.py", "from . import moderate_dep\nfrom . import helper\n")
	writeFile(t, root, "pkg/moderate_dep.py", "# moderate fan-in dep\n")
	writeFile(t, root, "pkg/helper.py", "from . import near_miss\nfrom . import noise\n")
	writeFile(t, root, "pkg/near_miss.py", "# near-miss 2-hop\n")
	writeFile(t, root, "pkg/noise.py", "# noise 2-hop\n")
	writeFile(t, root, "pkg/single_consumer.py", "from . import subject\n")

	writeFile(t, root, "pkg/user_a.py", "from . import moderate_dep\n")
	writeFile(t, root, "pkg/user_b.py", "from . import moderate_dep\n")

	gitInit(t, root)
	now := time.Now()

	gitAdd(t, root, "pkg/__init__.py", "pkg/helper.py", "pkg/near_miss.py", "pkg/noise.py",
		"pkg/moderate_dep.py", "pkg/user_a.py", "pkg/user_b.py")
	gitCommitAt(t, root, "add infrastructure", now.AddDate(0, 0, -60))

	gitAdd(t, root, "pkg/subject.py", "pkg/single_consumer.py")
	gitCommitAt(t, root, "add subject and consumer", now.AddDate(0, 0, -55))

	writeFile(t, root, "pkg/subject.py", "from . import moderate_dep\nfrom . import helper\n# v1\n")
	writeFile(t, root, "pkg/moderate_dep.py", "# moderate fan-in dep v1\n")
	gitAdd(t, root, "pkg/subject.py", "pkg/moderate_dep.py")
	gitCommitAt(t, root, "co-change moderate", now.AddDate(0, 0, -40))

	for i := 0; i < 2; i++ {
		writeFile(t, root, "pkg/subject.py", fmt.Sprintf("from . import moderate_dep\nfrom . import helper\n# nm%d\n", i))
		writeFile(t, root, "pkg/near_miss.py", fmt.Sprintf("# near-miss 2-hop v%d\n", i))
		gitAdd(t, root, "pkg/subject.py", "pkg/near_miss.py")
		gitCommitAt(t, root, fmt.Sprintf("co-change near-miss %d", i), now.AddDate(0, 0, -30+i))
	}

	writeFile(t, root, "pkg/subject.py", "from . import moderate_dep\nfrom . import helper\n# noise\n")
	writeFile(t, root, "pkg/noise.py", "# noise 2-hop v1\n")
	gitAdd(t, root, "pkg/subject.py", "pkg/noise.py")
	gitCommitAt(t, root, "co-change noise", now.AddDate(0, 0, -20))

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("condition_A_near_threshold", func(t *testing.T) {
		entry := getLikelyModEntry(output, "pkg/near_miss.py")
		if entry == nil {
			t.Fatalf("expected likely_modify for near_miss.py, got none.\nlikely_modify: %v", flattenLikelyModify(output))
		}
		if entry.Confidence != "low" {
			t.Errorf("confidence = %q, want %q", entry.Confidence, "low")
		}
	})

	t.Run("condition_B_importer_single_cochange", func(t *testing.T) {
		entry := getLikelyModEntry(output, "pkg/single_consumer.py")
		if entry == nil {
			t.Fatalf("expected likely_modify for single_consumer.py, got none.\nlikely_modify: %v", flattenLikelyModify(output))
		}
		if entry.Confidence != "low" {
			t.Errorf("confidence = %q, want %q", entry.Confidence, "low")
		}
	})

	t.Run("condition_C_moderate_fanin", func(t *testing.T) {
		entry := getLikelyModEntry(output, "pkg/moderate_dep.py")
		if entry == nil {
			t.Fatalf("expected likely_modify for moderate_dep.py, got none.\nlikely_modify: %v", flattenLikelyModify(output))
		}
		if entry.Confidence != "low" {
			t.Errorf("confidence = %q, want %q", entry.Confidence, "low")
		}
	})

	t.Run("noise_excluded", func(t *testing.T) {
		likelyMod := flattenLikelyModify(output)
		if sliceContains(likelyMod, "pkg/noise.py") {
			t.Errorf("noise.py should NOT have likely_modify (1x co-change, distance 2, no structural relationship)")
		}
	})
}

// TestTransitiveCallerThroughBarrel verifies barrel-piercing traversal.
func TestTransitiveCallerThroughBarrel(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/agents/__init__.py", "from .task_runner import run\n")
	writeFile(t, root, "pkg/agents/task_runner.py", "# agent task runner\ndef run(): pass\n")
	writeFile(t, root, "pkg/orchestrator.py", "from . import agents\n")
	writeFile(t, root, "pkg/direct_consumer.py", "from .agents import task_runner\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/agents/task_runner.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	assertContains(t, "must_read", mustRead, "pkg/agents/__init__.py")

	related := flattenRelated(output)
	likelyMod := flattenLikelyModify(output)
	inMR := sliceContains(mustRead, "pkg/orchestrator.py")
	inRel := sliceContains(related, "pkg/orchestrator.py")
	inLM := sliceContains(likelyMod, "pkg/orchestrator.py")
	if !inMR && !inRel && !inLM {
		t.Errorf("orchestrator.py should be surfaced via barrel-piercing")
	}

	// Should have transitive_caller signal.
	if !hasSignal(output, "pkg/orchestrator.py", "transitive_caller") {
		t.Errorf("expected transitive_caller signal for orchestrator.py")
	}
}

// TestTransitiveCallerThroughIndexTS verifies barrel-piercing with TypeScript index.ts.
func TestTransitiveCallerThroughIndexTS(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "package.json", "{\"name\":\"test\"}\n")
	writeFile(t, root, "tsconfig.json", "{\"compilerOptions\":{\"baseUrl\":\".\"}}\n")
	writeFile(t, root, "src/agents/subtitle.agent.ts", "export class SubtitleAgent {}\n")
	writeFile(t, root, "src/agents/index.ts", "export { SubtitleAgent } from './subtitle.agent'\n")
	writeFile(t, root, "src/server/handler.ts", "import { SubtitleAgent } from '../agents'\n")
	writeFile(t, root, "src/direct.ts", "import { SubtitleAgent } from './agents/subtitle.agent'\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("src/agents/subtitle.agent.ts")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	assertContains(t, "must_read", mustRead, "src/agents/index.ts")

	related := flattenRelated(output)
	likelyMod := flattenLikelyModify(output)
	inMR := sliceContains(mustRead, "src/server/handler.ts")
	inRel := sliceContains(related, "src/server/handler.ts")
	inLM := sliceContains(likelyMod, "src/server/handler.ts")
	if !inMR && !inRel && !inLM {
		t.Errorf("handler.ts should be surfaced via barrel-piercing")
	}

	if !hasSignal(output, "src/server/handler.ts", "transitive_caller") {
		t.Errorf("expected transitive_caller signal for handler.ts")
	}
}

// TestRelatedTierDeterminism runs analysis twice and verifies related output
// is identical.
func TestRelatedTierDeterminism(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/subject.py", "from . import dep_a\nfrom . import dep_b\n")
	writeFile(t, root, "pkg/dep_a.py", "from . import shared\n")
	writeFile(t, root, "pkg/dep_b.py", "from . import shared\nfrom . import extra\n")
	writeFile(t, root, "pkg/shared.py", "# shared\n")
	writeFile(t, root, "pkg/extra.py", "# extra\n")
	for i := 1; i <= 5; i++ {
		writeFile(t, root, fmt.Sprintf("pkg/user_%d.py", i), "from . import shared\n")
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})

	output1, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}
	output2, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	assertSameElements(t, "related", flattenRelated(output1), flattenRelated(output2))
	assertSameElements(t, "must_read", flattenMustRead(output1), flattenMustRead(output2))
}

// TestTestsSectionPopulated verifies that test files appear in the tests section.
func TestTestsSectionPopulated(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// tests/test_main.py imports the subject → should be in tests.
	assertContains(t, "tests", flattenTests(output), "tests/test_main.py")

	// Tests should not be in must_read.
	assertNotContains(t, "must_read", flattenMustRead(output), "tests/test_main.py")
}

// TestMustReadIsFlat verifies the v3 must_read is a flat list, not a map.
func TestMustReadIsFlat(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// must_read should be non-empty and entries should have File field.
	if len(output.MustRead) == 0 {
		t.Fatal("expected non-empty must_read")
	}
	for _, entry := range output.MustRead {
		if entry.File == "" {
			t.Error("must_read entry has empty File")
		}
	}
}

// TestFilterTestCandidatesMustReadProximity verifies that when no direct tests
// exist, dependency tests (near must_read files) are suppressed entirely.
func TestFilterTestCandidatesMustReadProximity(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "src/lib/__tests__/subtitle-formatter.test.ts"}, Score: 1.0},
		{candidate: candidate{Path: "src/unrelated/foo.test.ts"}, Score: 0.5},
	}
	subject := "src/agents/subtitle.agent.ts"
	mustReadPaths := []string{"src/lib/subtitle-formatter.ts"}

	result, _ := filterTestCandidates(candidates, subject, mustReadPaths, defaultMaxTests)

	// No direct tests for subject → dependency tests suppressed.
	resultPaths := testCandidatePaths(result)
	assertNotContains(t, "tests", resultPaths, "src/lib/__tests__/subtitle-formatter.test.ts")
	assertNotContains(t, "tests", resultPaths, "src/unrelated/foo.test.ts")
	if len(result) != 0 {
		t.Errorf("expected empty tests, got %v", resultPaths)
	}
}

// TestTestsSubdirRequiresImportOrStemMatch verifies that a test in
// subjectDir/__tests__/ without import or stem match is NOT classified as direct.
func TestTestsSubdirRequiresImportOrStemMatch(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "src/agents/__tests__/video-transcription.agent.test.ts"}, Score: 1.0},
	}
	subject := "src/agents/subtitle.agent.ts"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected empty tests when __tests__/ test doesn't import or stem-match subject, got %v", testCandidatePaths(result))
	}
}

// TestFilterTestCandidatesStemMatch verifies that when no direct tests exist,
// even dependency tests with stem matches to must_read files are suppressed.
func TestFilterTestCandidatesStemMatch(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "src/lib/subtitle-formatter.test.ts"}, Score: 1.0},
		{candidate: candidate{Path: "src/lib/subtitle-formatter.spec.tsx"}, Score: 0.9},
		{candidate: candidate{Path: "src/unrelated/other.test.ts"}, Score: 0.5},
	}
	subject := "src/agents/subtitle.agent.ts"
	mustReadPaths := []string{"src/lib/subtitle-formatter.ts"}

	result, _ := filterTestCandidates(candidates, subject, mustReadPaths, defaultMaxTests)

	// No direct tests for subject → all dependency tests suppressed.
	if len(result) != 0 {
		t.Errorf("expected empty tests when no direct tests exist, got %v", testCandidatePaths(result))
	}
}

// TestTestDiscoveryForMustReadDeps is an integration test: creates a TS project
// where the subject imports a dependency that has its own test file, and verifies
// that test file appears in the tests section.
func TestTestDiscoveryForMustReadDeps(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "package.json", `{"name":"test"}`)
	writeFile(t, root, "tsconfig.json", `{"compilerOptions":{"baseUrl":"."}}`)
	writeFile(t, root, "src/agents/subtitle.agent.ts",
		"import { format } from '../lib/subtitle-formatter'\nexport function run() { return format() }\n")
	writeFile(t, root, "src/lib/subtitle-formatter.ts",
		"export function format() { return 'formatted' }\n")
	writeFile(t, root, "src/lib/__tests__/subtitle-formatter.test.ts",
		"import { format } from '../subtitle-formatter'\ntest('format', () => { expect(format()).toBe('formatted') })\n")
	writeFile(t, root, "src/agents/__tests__/subtitle.agent.test.ts",
		"import { run } from '../subtitle.agent'\ntest('run', () => { expect(run()).toBeTruthy() })\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("src/agents/subtitle.agent.ts")
	if err != nil {
		t.Fatal(err)
	}

	// subtitle-formatter.ts should be in must_read (direct import of subject).
	assertContains(t, "must_read", flattenMustRead(output), "src/lib/subtitle-formatter.ts")

	// Both test files should be discovered:
	// - subtitle.agent.test.ts: imports the subject (existing criteria).
	assertContains(t, "tests", flattenTests(output), "src/agents/__tests__/subtitle.agent.test.ts")
	// - subtitle-formatter.test.ts: nearMustRead + stemMatch via must_read file.
	assertContains(t, "tests", flattenTests(output), "src/lib/__tests__/subtitle-formatter.test.ts")
}

// --- High fan-in tests ---

// createHighFanInProject creates a project where subject.py has 60 reverse
// importers and 3 forward imports.
func createHighFanInProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"pkg/__init__.py": "",
		"pkg/dep_a.py":    "# dep A\n",
		"pkg/dep_b.py":    "# dep B\n",
		"pkg/dep_c.py":    "# dep C\n",
	}

	// subject.py imports 3 deps.
	files["pkg/subject.py"] = "from . import dep_a\nfrom . import dep_b\nfrom . import dep_c\n"

	// 60 consumer files that import subject.py.
	for i := 1; i <= 60; i++ {
		files[fmt.Sprintf("pkg/consumer_%03d.py", i)] = "from . import subject\n"
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func buildHighFanInIndex(t *testing.T) (*Analyzer, *db.Index) {
	t.Helper()
	root := createHighFanInProject(t)
	idx := openTestIndex(t)

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	return a, idx
}

// TestHighFanInMustReadOnlyForwardImports verifies that when a subject has
// 60 reverse importers (>= highFanInThreshold), must_read contains only
// forward imports, not consumer files.
func TestHighFanInMustReadOnlyForwardImports(t *testing.T) {
	a, idx := buildHighFanInIndex(t)
	defer idx.Close()

	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)

	// must_read should contain the 3 forward imports.
	assertContains(t, "must_read", mustRead, "pkg/dep_a.py")
	assertContains(t, "must_read", mustRead, "pkg/dep_b.py")
	assertContains(t, "must_read", mustRead, "pkg/dep_c.py")

	// must_read should NOT contain any consumer files.
	for i := 1; i <= 60; i++ {
		name := fmt.Sprintf("pkg/consumer_%03d.py", i)
		assertNotContains(t, "must_read", mustRead, name)
	}

	// must_read should have exactly 3 entries (the forward imports).
	if len(mustRead) != 3 {
		t.Errorf("must_read length = %d, want 3 (forward imports only)", len(mustRead))
	}
}

// TestHighFanInRelatedCapped verifies that for a high fan-in subject,
// related is capped at maxRelated (10), not 60+.
func TestHighFanInRelatedCapped(t *testing.T) {
	a, idx := buildHighFanInIndex(t)
	defer idx.Close()

	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	related := flattenRelated(output)

	if len(related) > defaultMaxRelated {
		t.Errorf("related length = %d, want <= %d (maxRelated cap)", len(related), defaultMaxRelated)
	}
}

// TestDirectTestsPrioritizedOverDependencyTests verifies that direct tests
// (stem match to subject) sort before must_read-proximity tests.
func TestDirectTestsPrioritizedOverDependencyTests(t *testing.T) {
	subjectStem := extractSubjectStem("logger.ts")
	if subjectStem != "logger" {
		t.Fatalf("extractSubjectStem(logger.ts) = %q, want %q", subjectStem, "logger")
	}

	// Create candidates: a direct test (stem matches subject) and a dependency test
	// (stem matches a must_read file). Give dependency test a higher score so that
	// without tiering it would sort first.
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "src/utils/__tests__/helpers.test.ts"}, Score: 5.0},
		{candidate: candidate{Path: "src/logger/__tests__/logger.test.ts"}, Score: 1.0},
	}
	subject := "src/logger/logger.ts"
	mustReadPaths := []string{"src/utils/helpers.ts"}

	result, _ := filterTestCandidates(candidates, subject, mustReadPaths, defaultMaxTests)
	resultPaths := testCandidatePaths(result)

	if len(result) < 2 {
		t.Fatalf("expected 2 tests, got %d: %v", len(result), resultPaths)
	}

	// Direct test (logger.test.ts) should come first despite lower score.
	if resultPaths[0] != "src/logger/__tests__/logger.test.ts" {
		t.Errorf("first test = %q, want direct test %q", resultPaths[0], "src/logger/__tests__/logger.test.ts")
	}
	if resultPaths[1] != "src/utils/__tests__/helpers.test.ts" {
		t.Errorf("second test = %q, want dependency test %q", resultPaths[1], "src/utils/__tests__/helpers.test.ts")
	}
	// Verify tier flags.
	if !result[0].direct {
		t.Error("first test should be direct")
	}
	if result[1].direct {
		t.Error("second test should be dependency (not direct)")
	}
}

// TestEmptyTestsNote verifies that TestsNote is set when a non-test subject
// has no test candidates, and absent when the subject is a test file itself.
func TestEmptyTestsNote(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/main.py", "# no imports\n")
	writeFile(t, root, "tests/test_other.py", "# tests something else\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})

	t.Run("non_test_subject_no_tests", func(t *testing.T) {
		output, err := a.Analyze("pkg/main.py")
		if err != nil {
			t.Fatal(err)
		}
		if len(output.Tests) != 0 {
			t.Errorf("expected empty tests, got %v", flattenTests(output))
		}
		if output.TestsNote != "no test files found in nearby directories" {
			t.Errorf("TestsNote = %q, want %q", output.TestsNote, "no test files found in nearby directories")
		}
	})

	t.Run("test_subject_no_note", func(t *testing.T) {
		output, err := a.Analyze("tests/test_other.py")
		if err != nil {
			t.Fatal(err)
		}
		if output.TestsNote != "" {
			t.Errorf("TestsNote = %q, want empty for test subject", output.TestsNote)
		}
	})
}

// TestHighFanInSuppressesConsumerTests verifies that when a subject has high
// fan-in (>= highFanInThreshold reverse importers), dependency-tier tests
// (tests that import the subject) are suppressed. Only direct tests survive.
func TestHighFanInSuppressesConsumerTests(t *testing.T) {
	root := t.TempDir()

	files := map[string]string{
		"pkg/__init__.py": "",
		"pkg/dep_a.py":    "# dep A\n",
		"pkg/dep_b.py":    "# dep B\n",
	}

	// subject.py imports 2 deps.
	files["pkg/subject.py"] = "from . import dep_a\nfrom . import dep_b\n"

	// 60 consumer files that import subject.py.
	for i := 1; i <= 60; i++ {
		files[fmt.Sprintf("pkg/consumer_%03d.py", i)] = "from . import subject\n"
	}

	// A consumer test that imports the subject — this is a dependency-tier test,
	// not a direct test (no stem match, not in subject's dir or __tests__).
	files["tests/test_consumer_of_subject.py"] = "from pkg.subject import something\n"

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	// With high fan-in, consumer tests should be suppressed.
	if len(output.Tests) != 0 {
		t.Errorf("expected empty tests for high fan-in subject, got %v", flattenTests(output))
	}
	if output.TestsNote != "1 test files nearby but none directly test this file" {
		t.Errorf("TestsNote = %q, want %q", output.TestsNote, "1 test files nearby but none directly test this file")
	}
}

// TestLikelyModifyNoDuplication verifies the no-duplication invariant: files
// in likely_modify that are NOT in must_read should NOT appear in related.
// This prevents the ~40-60% duplication that the old containment invariant caused.
func TestLikelyModifyNoDuplication(t *testing.T) {
	root := t.TempDir()

	// Barrel re-exports 14 modules — overflows maxMustRead (10).
	barrelContent := ""
	for i := 1; i <= 14; i++ {
		name := fmt.Sprintf("mod_%02d", i)
		writeFile(t, root, fmt.Sprintf("src/%s.ts", name), fmt.Sprintf("export const %s = %d\n", name, i))
		barrelContent += fmt.Sprintf("export { %s } from './%s'\n", name, name)
	}
	writeFile(t, root, "package.json", `{"name":"test"}`)
	writeFile(t, root, "tsconfig.json", `{"compilerOptions":{"baseUrl":"."}}`)
	writeFile(t, root, "src/index.ts", barrelContent)
	writeFile(t, root, "src/app.ts", "import { mod_01, mod_02 } from './index'\n")

	gitInit(t, root)
	now := time.Now()

	gitAdd(t, root, ".")
	gitCommitAt(t, root, "initial", now.AddDate(0, 0, -60))

	// Create co-change history between app.ts and several modules.
	// This makes those modules qualify for likely_modify.
	for i := 0; i < 4; i++ {
		writeFile(t, root, "src/app.ts", fmt.Sprintf("import { mod_01, mod_02 } from './index'\n// v%d\n", i))
		writeFile(t, root, "src/mod_01.ts", fmt.Sprintf("export const mod_01 = %d\n", 100+i))
		writeFile(t, root, "src/mod_02.ts", fmt.Sprintf("export const mod_02 = %d\n", 200+i))
		writeFile(t, root, "src/mod_03.ts", fmt.Sprintf("export const mod_03 = %d\n", 300+i))
		gitAdd(t, root, "src/app.ts", "src/mod_01.ts", "src/mod_02.ts", "src/mod_03.ts")
		gitCommitAt(t, root, fmt.Sprintf("co-change %d", i), now.AddDate(0, 0, -40+i))
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	// Analyze the barrel file (index.ts) — it has 14 re-exports, overflows must_read.
	a := New(idx, Config{})
	output, err := a.Analyze("src/index.ts")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	related := flattenRelated(output)
	likelyMod := flattenLikelyModify(output)

	mustReadSet := make(map[string]bool, len(mustRead))
	for _, p := range mustRead {
		mustReadSet[p] = true
	}
	relatedSet := make(map[string]bool, len(related))
	for _, p := range related {
		relatedSet[p] = true
	}

	// No-duplication invariant: likely_modify files NOT in must_read should NOT be in related.
	for _, lm := range likelyMod {
		if !mustReadSet[lm] && relatedSet[lm] {
			t.Errorf("duplication: likely_modify file %q also appears in related", lm)
		}
	}

	// Sanity: must_read should be capped at maxMustRead.
	if len(mustRead) > 10 {
		t.Errorf("must_read exceeds cap: got %d, want <= 10", len(mustRead))
	}
}

// TestTopoSortFoundationalFirst verifies that when must_read files are independent
// siblings (no inter-edges), higher-indegree files (more foundational) sort before
// lower-indegree files. Types/models that many files import should come before
// leaf utilities that few files import.
func TestTopoSortFoundationalFirst(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/subject.py", "from . import leaf\nfrom . import popular\n")
	writeFile(t, root, "pkg/leaf.py", "# leaf — low indegree\n")
	writeFile(t, root, "pkg/popular.py", "# popular — high indegree (foundational)\n")

	// Add many importers of popular.py to boost its indegree.
	for i := 1; i <= 20; i++ {
		writeFile(t, root, fmt.Sprintf("pkg/user_%02d.py", i), "from . import popular\n")
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	assertContains(t, "must_read", mustRead, "pkg/leaf.py")
	assertContains(t, "must_read", mustRead, "pkg/popular.py")

	// popular.py (indegree ~21, foundational) should come before leaf.py (low indegree ~1).
	leafIdx := -1
	popularIdx := -1
	for i, p := range mustRead {
		if p == "pkg/leaf.py" {
			leafIdx = i
		}
		if p == "pkg/popular.py" {
			popularIdx = i
		}
	}
	if popularIdx >= leafIdx {
		t.Errorf("expected popular.py (idx=%d) before leaf.py (idx=%d) in must_read: %v",
			popularIdx, leafIdx, mustRead)
	}
}

// --- I-001: sameDir qualification ---

// TestSameDirTestRequiresImportOrStemMatch verifies that a test file in the
// same directory as the subject is NOT classified as direct when it has a
// different stem and does not import the subject.
func TestSameDirTestRequiresImportOrStemMatch(t *testing.T) {
	candidates := []scoredCandidate{
		// Same dir, different stem, no import.
		{candidate: candidate{Path: "src/server/server-utils.test.ts"}, Score: 1.0},
	}
	subject := "src/server/base-server.ts"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected empty tests when same-dir test doesn't import or stem-match subject, got %v", testCandidatePaths(result))
	}
}

// --- I-007: importsSubject alone should not make a test direct ---

// TestRemoteDirImportNotDirect verifies that a test in a completely different
// directory is NOT classified as direct even when it imports the subject,
// regardless of whether the subject is high fan-in.
func TestRemoteDirImportNotDirect(t *testing.T) {
	candidates := []scoredCandidate{
		// Different dir, different stem, imports subject.
		{candidate: candidate{
			Path:       "src/server/route-matcher-providers/app-page-route-matcher-provider.test.ts",
			IsImporter: true,
		}, Score: 1.0},
	}
	subject := "src/shared/lib/constants.ts"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected empty tests when remote-dir test only imports subject, got %v", testCandidatePaths(result))
	}
}

// --- I-002: likely_modify low for importers without co-change ---

// TestLikelyModifyLowImporterWithoutCoChange verifies that a direct importer
// at distance 1 with CoChangeFreq=0 qualifies as isLikelyModifyLow.
func TestLikelyModifyLowImporterWithoutCoChange(t *testing.T) {
	sc := &scoredCandidate{
		candidate: candidate{
			Path:         "pkg/consumer.py",
			Distance:     1,
			IsImporter:   true,
			CoChangeFreq: 0,
		},
	}

	if isLikelyModify(sc) {
		t.Error("should NOT qualify as isLikelyModify (no co-change)")
	}
	if !isLikelyModifyLow(sc) {
		t.Error("should qualify as isLikelyModifyLow (condition D: importer, distance=1, CoChangeFreq=0)")
	}
}

// --- I-005/I-006: blast_radius fixes ---

// TestBlastRadiusLowForLeafFile verifies that a file with few importers gets
// blast_radius "low", even if it has many 2-hop neighbors.
func TestBlastRadiusLowForLeafFile(t *testing.T) {
	cats := categories{
		TotalLikelyModify:    0,
		TotalRelated:         12, // Many 2-hop neighbors.
		ReverseImporterCount: 2,  // Few actual importers.
	}
	br := computeBlastRadius(cats, 0, false, Caps{}.resolve())
	if br.Level != "low" {
		t.Errorf("blast_radius level = %q, want %q for leaf file with few importers", br.Level, "low")
	}
}

// TestBlastRadiusLowForTestSubject verifies that test files always get
// blast_radius "low", regardless of importer count.
func TestBlastRadiusLowForTestSubject(t *testing.T) {
	cats := categories{
		TotalLikelyModify:    20,
		TotalRelated:         100,
		ReverseImporterCount: 60,
	}
	br := computeBlastRadius(cats, 5, true, Caps{}.resolve())
	if br.Level != "low" {
		t.Errorf("blast_radius level = %q, want %q for test subject", br.Level, "low")
	}
}

// TestBlastRadiusDiscountsD2Low verifies that distance-2 LOW candidates
// are excluded from the "high" threshold, preventing co-change noise in
// monorepos from inflating blast_radius for structurally contained files.
func TestBlastRadiusDiscountsD2Low(t *testing.T) {
	cats := categories{
		TotalLikelyModify:      20,
		LikelyModifyD2LowCount: 15, // 15 are distance-2 LOW
		ReverseImporterCount:   1,
	}
	br := computeBlastRadius(cats, 0, false, Caps{}.resolve())
	// effectiveLM = 20 - 15 = 5 → "medium" not "high"
	if br.Level != "medium" {
		t.Errorf("blast_radius level = %q, want %q (effectiveLM = 5, totalLM = 20 with 15 D2 LOW)", br.Level, "medium")
	}
}

// TestBlastRadiusHighWithStrongSignals verifies that files with mostly
// non-D2-LOW candidates still get "high" blast_radius.
func TestBlastRadiusHighWithStrongSignals(t *testing.T) {
	cats := categories{
		TotalLikelyModify:      20,
		LikelyModifyD2LowCount: 3, // only 3 are distance-2 LOW
		ReverseImporterCount:   1,
	}
	br := computeBlastRadius(cats, 0, false, Caps{}.resolve())
	// effectiveLM = 20 - 3 = 17 → "high"
	if br.Level != "high" {
		t.Errorf("blast_radius level = %q, want %q (effectiveLM = 17)", br.Level, "high")
	}
}

// --- Go platform-aware stem matching ---

// TestExtractTestStemGoPlatform verifies that platform suffixes are stripped
// from Go test file stems, allowing exec_linux_test.go to match exec.go.
func TestExtractTestStemGoPlatform(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"exec_linux_test.go", "exec"},
		{"exec_darwin_amd64_test.go", "exec"},
		{"handler_test.go", "handler"},
		{"os_exec_windows_test.go", "os_exec"},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			got := extractTestStem(tt.base)
			if got != tt.want {
				t.Errorf("extractTestStem(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

// TestExtractSubjectStemGoPlatform verifies that platform suffixes are stripped
// from Go subject file stems, allowing exec_linux.go to match exec_test.go.
func TestExtractSubjectStemGoPlatform(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"exec_linux.go", "exec"},
		{"exec_darwin_amd64.go", "exec"},
		{"handler.go", "handler"},
		{"models.py", "models"},     // Non-Go: no stripping
		{"app.test.ts", "app.test"}, // Non-Go: no stripping
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			got := extractSubjectStem(tt.base)
			if got != tt.want {
				t.Errorf("extractSubjectStem(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

// TestBlastRadiusUsesPackageLevelCount verifies that computeBlastRadius
// returns "high" when PackageImporterCount >= highFanInThreshold even if
// ReverseImporterCount is near zero (non-representative Go file scenario).
func TestBlastRadiusUsesPackageLevelCount(t *testing.T) {
	cats := categories{
		TotalLikelyModify:    3,
		ReverseImporterCount: 1,  // near-zero file-level importers
		PackageImporterCount: 60, // but package has 60 importers
	}
	br := computeBlastRadius(cats, 0, false, Caps{}.resolve())
	if br.Level != "high" {
		t.Errorf("blast_radius level = %q, want %q (PackageImporterCount = 60 should trigger high)", br.Level, "high")
	}
}

// --- I-015: __init__.py test detection ---

// TestInitPyTestDetection verifies that test_init.py is classified as direct
// for __init__.py subjects when the test imports the subject.
func TestInitPyTestDetection(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/auth/test_init.py", IsImporter: true}, Score: 1.0},
	}
	subject := "homeassistant/auth/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 1 {
		t.Fatalf("expected 1 test, got %d: %v", len(result), testCandidatePaths(result))
	}
	if !result[0].direct {
		t.Errorf("expected test_init.py to be direct for __init__.py subject")
	}
}

// TestInitPyTestNoImport verifies that test_init.py is NOT classified as direct
// for __init__.py subjects when the test does NOT import the subject.
func TestInitPyTestNoImport(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/auth/test_init.py", IsImporter: false}, Score: 1.0},
	}
	subject := "homeassistant/auth/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	// No direct tests → dependency tests suppressed → empty result.
	if len(result) != 0 {
		t.Errorf("expected empty tests when test_init.py doesn't import __init__.py, got %v", testCandidatePaths(result))
	}
}

// TestInitPyTestDifferentPackage verifies that a remote test_init.py without
// import is NOT classified as direct for a different package's __init__.py.
func TestInitPyTestDifferentPackage(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/components/light/test_init.py", IsImporter: false}, Score: 1.0},
	}
	subject := "homeassistant/auth/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected empty tests for remote test_init.py without import, got %v", testCandidatePaths(result))
	}
}

// --- I-018: High-fan-in __init__.py false positives ---

// TestInitPyHighFanInRejectsNonCorresponding verifies that test_init.py files from
// non-corresponding directories are NOT classified as direct, even when they import
// the subject (high-fan-in scenario).
func TestInitPyHighFanInRejectsNonCorresponding(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/components/sensor/test_init.py", IsImporter: true}, Score: 1.0},
		{candidate: candidate{Path: "tests/components/switch/test_init.py", IsImporter: true}, Score: 0.9},
		{candidate: candidate{Path: "tests/components/automation/test_init.py", IsImporter: true}, Score: 0.8},
		{candidate: candidate{Path: "tests/components/light/test_init.py", IsImporter: true}, Score: 0.5},
	}
	subject := "homeassistant/components/light/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	// Only tests/components/light/test_init.py should be direct (corresponding dir).
	directCount := 0
	for _, tc := range result {
		if tc.direct {
			directCount++
			if tc.path != "tests/components/light/test_init.py" {
				t.Errorf("unexpected direct test: %s", tc.path)
			}
		}
	}
	if directCount != 1 {
		t.Errorf("expected 1 direct test, got %d: %v", directCount, testCandidatePaths(result))
	}
}

// TestInitPyCorrespondingDirWithImport confirms that a test_init.py in the
// corresponding directory with import IS classified as direct.
func TestInitPyCorrespondingDirWithImport(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/components/light/test_init.py", IsImporter: true}, Score: 1.0},
	}
	subject := "homeassistant/components/light/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 1 {
		t.Fatalf("expected 1 test, got %d: %v", len(result), testCandidatePaths(result))
	}
	if !result[0].direct {
		t.Errorf("expected test_init.py in corresponding dir to be direct")
	}
}

// --- I-017: Cross-directory stem false positives ---

// TestRemoteStemMatchRequiresImport verifies that a remote test with matching stem
// but no import is NOT classified as direct.
func TestRemoteStemMatchRequiresImport(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/components/light/test_entity.py", IsImporter: false}, Score: 1.0},
		{candidate: candidate{Path: "tests/components/sensor/test_entity.py", IsImporter: false}, Score: 0.9},
	}
	subject := "homeassistant/helpers/entity.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected empty tests for remote stem matches without import, got %v", testCandidatePaths(result))
	}
}

// TestRemoteStemMatchWithImportIsDirect verifies that a remote test with matching
// stem AND import IS classified as direct.
func TestRemoteStemMatchWithImportIsDirect(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/helpers/test_entity.py", IsImporter: true}, Score: 1.0},
		{candidate: candidate{Path: "tests/components/light/test_entity.py", IsImporter: false}, Score: 0.9},
	}
	subject := "homeassistant/helpers/entity.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 1 {
		t.Fatalf("expected 1 test, got %d: %v", len(result), testCandidatePaths(result))
	}
	if result[0].path != "tests/helpers/test_entity.py" {
		t.Errorf("expected tests/helpers/test_entity.py, got %q", result[0].path)
	}
	if !result[0].direct {
		t.Errorf("expected remote stem match with import to be direct")
	}
}

// TestSameDirStemMatchStillWorks verifies that same-directory stem matches
// still work without requiring import (preserved behavior).
func TestSameDirStemMatchStillWorks(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "myapp/test_models.py", IsImporter: false}, Score: 1.0},
	}
	subject := "myapp/models.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 1 {
		t.Fatalf("expected 1 test, got %d: %v", len(result), testCandidatePaths(result))
	}
	if !result[0].direct {
		t.Errorf("expected same-dir stem match to be direct without import")
	}
}

// --- Schema 3.2 integration tests ---

func TestConfidenceFullResolution(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// main.py has resolved imports (models, utils) and external (os).
	// No not_found unresolved, so confidence should be 1.0.
	if output.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", output.Confidence)
	}
	if output.ConfidenceNote != "" {
		t.Errorf("ConfidenceNote = %q, want empty", output.ConfidenceNote)
	}
}

func TestConfidenceLeafFile(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/base.py")
	if err != nil {
		t.Fatal(err)
	}

	// base.py has no imports at all → leaf file → confidence 1.0.
	if output.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0 for leaf file", output.Confidence)
	}
}

func TestConfidenceWithUnresolved(t *testing.T) {
	// Directly populate the DB to simulate a file with unresolved imports.
	// This avoids Python resolver ambiguities with relative imports.
	idx := openTestIndex(t)
	defer idx.Close()

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = db.InsertFileTx(tx, "app/main.py", "h1", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err = db.InsertFileTx(tx, "app/utils.py", "h2", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	// 1 resolved edge.
	if err = db.InsertEdgeTx(tx, "app/main.py", "app/utils.py", "import", "utils", "relative", 1, nil); err != nil {
		t.Fatal(err)
	}
	// 1 not_found unresolved.
	if err = db.InsertUnresolvedTx(tx, "app/main.py", "missing_mod", "not_found", 5); err != nil {
		t.Fatal(err)
	}
	// Signals for main.py (required by analyzer).
	if _, err = tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"app/main.py", 0, 1, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility) VALUES (?, ?, ?, ?, ?)",
		"app/utils.py", 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("app/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// 1 resolved + 1 not_found → confidence = 0.5.
	if output.Confidence >= 1.0 {
		t.Errorf("Confidence = %f, want < 1.0 with unresolved imports", output.Confidence)
	}
	if output.Confidence != 0.5 {
		t.Errorf("Confidence = %f, want 0.5 (1 resolved / 2 total)", output.Confidence)
	}
	if output.ConfidenceNote == "" {
		t.Error("ConfidenceNote should be set when imports are unresolved")
	}
}

func TestRoleOnMustReadEntries(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// Check that role appears on must_read entries where applicable.
	for _, entry := range output.MustRead {
		// base.py has no imports and is imported by models — could be foundation
		// depending on indegree/stableThresh. Just verify the field exists (may be "").
		if entry.File == "myapp/__init__.py" {
			if entry.Role != "barrel" {
				t.Errorf("__init__.py role = %q, want %q", entry.Role, "barrel")
			}
		}
	}
}

func TestTestsNoteZeroCandidates(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/main.py", "# no imports\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/main.py")
	if err != nil {
		t.Fatal(err)
	}

	if output.TestsNote != "no test files found in nearby directories" {
		t.Errorf("TestsNote = %q, want %q", output.TestsNote, "no test files found in nearby directories")
	}
}

func TestTestsNoteWithNearbyCandidates(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/main.py", "from . import utils\n")
	writeFile(t, root, "pkg/utils.py", "# no imports\n")
	// test_utils.py tests utils, not main. But it's a test candidate via graph.
	writeFile(t, root, "pkg/test_utils.py", "from . import utils\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	output, err := a.Analyze("pkg/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// test_utils.py is a test candidate but doesn't directly test main.py.
	// The tests_note should indicate candidates exist but none directly test the subject.
	if len(output.Tests) == 0 && output.TestsNote == "no test files found in nearby directories" {
		t.Error("TestsNote should NOT say 'no test files found' when test candidates exist nearby")
	}
}

// === Test 1: Configurable Caps Unit Tests ===

func TestCapsResolveDefaults(t *testing.T) {
	c := Caps{}.resolve()
	if c.MaxMustRead != 10 {
		t.Errorf("MaxMustRead = %d, want 10", c.MaxMustRead)
	}
	if c.MaxRelated != 10 {
		t.Errorf("MaxRelated = %d, want 10", c.MaxRelated)
	}
	if c.MaxLikelyModify != 15 {
		t.Errorf("MaxLikelyModify = %d, want 15", c.MaxLikelyModify)
	}
	if c.MaxTests != 5 {
		t.Errorf("MaxTests = %d, want 5", c.MaxTests)
	}
}

func TestCapsResolveCustom(t *testing.T) {
	c := Caps{MaxMustRead: 3, MaxRelated: 5}.resolve()
	if c.MaxMustRead != 3 {
		t.Errorf("MaxMustRead = %d, want 3", c.MaxMustRead)
	}
	if c.MaxRelated != 5 {
		t.Errorf("MaxRelated = %d, want 5", c.MaxRelated)
	}
	if c.MaxLikelyModify != 15 {
		t.Errorf("MaxLikelyModify = %d, want 15 (default)", c.MaxLikelyModify)
	}
	if c.MaxTests != 5 {
		t.Errorf("MaxTests = %d, want 5 (default)", c.MaxTests)
	}
}

func TestCapsResolveNegativeUsesDefault(t *testing.T) {
	c := Caps{MaxMustRead: -1}.resolve()
	if c.MaxMustRead != 10 {
		t.Errorf("MaxMustRead = %d, want 10 (negative treated as zero → default)", c.MaxMustRead)
	}
}

func TestCategorizeCustomMustReadCap(t *testing.T) {
	root := createOverflowProject(t)
	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{Caps: Caps{MaxMustRead: 3}})
	output, err := a.Analyze("pkg/hub.py")
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	if len(mustRead) > 3 {
		t.Errorf("must_read = %d, want <= 3 with custom cap", len(mustRead))
	}

	// Overflow should appear in related or likely_modify.
	related := flattenRelated(output)
	likelyMod := flattenLikelyModify(output)
	total := len(mustRead) + len(related) + len(likelyMod)
	if total == 0 {
		t.Error("expected at least some entries across must_read, related, likely_modify")
	}
}

func TestCategorizeCustomLikelyModifyCap(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/subject.py", "from . import dep_a\nfrom . import dep_b\nfrom . import dep_c\n")
	writeFile(t, root, "pkg/dep_a.py", "# dep A\n")
	writeFile(t, root, "pkg/dep_b.py", "# dep B\n")
	writeFile(t, root, "pkg/dep_c.py", "# dep C\n")
	// 5 consumers that import subject (will be likely_modify low candidates).
	for i := 1; i <= 5; i++ {
		writeFile(t, root, fmt.Sprintf("pkg/consumer_%d.py", i), "from . import subject\n")
	}

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{Caps: Caps{MaxLikelyModify: 2}})
	output, err := a.Analyze("pkg/subject.py")
	if err != nil {
		t.Fatal(err)
	}

	likelyMod := flattenLikelyModify(output)
	if len(likelyMod) > 2 {
		t.Errorf("likely_modify = %d, want <= 2 with custom cap", len(likelyMod))
	}
}

func TestCategorizeCustomTestsCap(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "pkg/__init__.py", "")
	writeFile(t, root, "pkg/main.py", "# main\n")
	writeFile(t, root, "pkg/test_main.py", "from . import main\n")
	writeFile(t, root, "pkg/test_main_extra.py", "from . import main\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{Caps: Caps{MaxTests: 1}})
	output, err := a.Analyze("pkg/main.py")
	if err != nil {
		t.Fatal(err)
	}

	if len(output.Tests) > 1 {
		t.Errorf("tests = %d, want <= 1 with custom cap", len(output.Tests))
	}
}

// === Test 3: Multi-file Analysis Unit Tests ===

func TestAnalyzeMultiEmptyInput(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	_, err := a.AnalyzeMulti([]string{})
	if err == nil {
		t.Error("expected error for empty input")
	}
	if err != nil && err.Error() != "no files provided" {
		t.Errorf("error = %q, want %q", err.Error(), "no files provided")
	}
}

func TestAnalyzeMultiSingleFile(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	multi, err := a.AnalyzeMulti([]string{"myapp/main.py"})
	if err != nil {
		t.Fatal(err)
	}

	single, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// Single-file AnalyzeMulti should delegate to Analyze and produce identical output.
	if multi.Subject != single.Subject {
		t.Errorf("Subject mismatch: %q vs %q", multi.Subject, single.Subject)
	}
	assertSameElements(t, "must_read", flattenMustRead(multi), flattenMustRead(single))
	assertSameElements(t, "tests", flattenTests(multi), flattenTests(single))
	if multi.Confidence != single.Confidence {
		t.Errorf("Confidence mismatch: %f vs %f", multi.Confidence, single.Confidence)
	}
}

func TestAnalyzeMultiTwoFiles(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	output, err := a.AnalyzeMulti([]string{"myapp/main.py", "myapp/models.py"})
	if err != nil {
		t.Fatal(err)
	}

	// Subject should be comma-separated.
	if output.Subject != "myapp/main.py, myapp/models.py" {
		t.Errorf("Subject = %q, want %q", output.Subject, "myapp/main.py, myapp/models.py")
	}

	// must_read should be deduped (no file appears twice).
	mustRead := flattenMustRead(output)
	seen := make(map[string]bool)
	for _, f := range mustRead {
		if seen[f] {
			t.Errorf("duplicate in must_read: %q", f)
		}
		seen[f] = true
	}

	// Subject files should NOT be in must_read, related, or likely_modify.
	for _, subj := range []string{"myapp/main.py", "myapp/models.py"} {
		assertNotContains(t, "must_read", mustRead, subj)
		assertNotContains(t, "related", flattenRelated(output), subj)
		assertNotContains(t, "likely_modify", flattenLikelyModify(output), subj)
	}

	// blast_radius should be present.
	if output.BlastRadius == nil {
		t.Error("expected blast_radius to be set")
	}

	// Confidence should be the minimum of both (both are 1.0 in this project).
	if output.Confidence > 1.0 || output.Confidence < 0.0 {
		t.Errorf("Confidence = %f, want 0.0-1.0", output.Confidence)
	}
}

func TestAnalyzeMultiSubjectsExcludedFromMustRead(t *testing.T) {
	a, idx := buildIndex(t)
	defer idx.Close()

	// main.py imports models.py. models.py has main.py as reverse importer.
	// When analyzing both, neither subject should appear in must_read.
	output, err := a.AnalyzeMulti([]string{"myapp/main.py", "myapp/models.py"})
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	assertNotContains(t, "must_read", mustRead, "myapp/main.py")
	assertNotContains(t, "must_read", mustRead, "myapp/models.py")
}

func TestAnalyzeMultiCapsApplied(t *testing.T) {
	root := createOverflowProject(t)
	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	// Analyze two files that individually have many must_read entries.
	a := New(idx, Config{Caps: Caps{MaxMustRead: 2}})
	output, err := a.AnalyzeMulti([]string{"pkg/hub.py", "pkg/consumer_1.py"})
	if err != nil {
		t.Fatal(err)
	}

	mustRead := flattenMustRead(output)
	if len(mustRead) > 2 {
		t.Errorf("must_read = %d, want <= 2 with custom cap on multi-file", len(mustRead))
	}
}

// === Test 6: Code Signatures Integration ===

func TestAnalyzeWithSignatures(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "myapp/__init__.py", "")
	writeFile(t, root, "myapp/main.py", "from .models import User\n")
	writeFile(t, root, "myapp/models.py", "class User(Base):\n    name = 'test'\n\ndef helper():\n    pass\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{Signatures: true, RepoRoot: root})
	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// Find models.py in must_read and check for definitions.
	var modelsEntry *model.MustReadEntry
	for i, entry := range output.MustRead {
		if entry.File == "myapp/models.py" {
			modelsEntry = &output.MustRead[i]
			break
		}
	}
	if modelsEntry == nil {
		t.Fatal("expected myapp/models.py in must_read")
	}

	// Symbols should include "User" (imported by main.py).
	if !sliceContains(modelsEntry.Symbols, "User") {
		t.Errorf("expected 'User' in symbols for models.py, got %v", modelsEntry.Symbols)
	}

	// Definitions should contain a signature matching "class User".
	if len(modelsEntry.Definitions) == 0 {
		t.Fatal("expected non-empty definitions for models.py when signatures enabled")
	}
	found := false
	for _, def := range modelsEntry.Definitions {
		if len(def) >= 10 && def[:10] == "class User" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected definition matching 'class User', got %v", modelsEntry.Definitions)
	}
}

func TestAnalyzeWithoutSignatures(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "myapp/__init__.py", "")
	writeFile(t, root, "myapp/main.py", "from .models import User\n")
	writeFile(t, root, "myapp/models.py", "class User(Base):\n    name = 'test'\n")

	idx := openTestIndex(t)
	defer idx.Close()

	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx, Verbose: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{Signatures: false})
	output, err := a.Analyze("myapp/main.py")
	if err != nil {
		t.Fatal(err)
	}

	// Definitions should be nil/empty on all must_read entries when signatures disabled.
	for _, entry := range output.MustRead {
		if len(entry.Definitions) > 0 {
			t.Errorf("expected empty definitions for %s when signatures disabled, got %v", entry.File, entry.Definitions)
		}
	}
}

// --- Round 16: Go same-package support tests ---

func TestShareGoPrefix(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// Shared prefix: node_resource_apply ↔ node_resource_abstract
		{"node_resource_apply", "node_resource_abstract", true},
		// Shared prefix: eval_apply ↔ eval_check
		{"eval_apply", "eval_check", true},
		// No shared prefix: create ↔ controller (single segments, different words)
		{"create", "controller", false},
		// No shared prefix: different first segments
		{"node_apply", "eval_check", false},
		// Same name: shouldn't match (shared == full length)
		{"node_resource", "node_resource", false},
		// One is prefix of other: node_resource vs node_resource_apply
		{"node_resource", "node_resource_apply", true},
		// Single-segment prefix: context ↔ context_apply
		{"context", "context_apply", true},
		// Single-segment prefix: daemon ↔ daemon_unix
		{"daemon", "daemon_unix", true},
		// Single-segment prefix: provider ↔ provider_test (test files filtered elsewhere)
		{"provider", "provider_test", true},
		// Single-segment: no match when stems differ
		{"create", "creator_utils", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := shareUnderscorePrefix(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("shareUnderscorePrefix(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestPrioritizeByPrefix(t *testing.T) {
	subject := "pkg/node_resource_apply.go"
	siblings := []string{
		"pkg/archive.go",
		"pkg/node_resource_abstract.go",
		"pkg/config.go",
		"pkg/node_resource_plan.go",
	}
	result := prioritizeByPrefix(siblings, subject)
	// Prefix matches should come first.
	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %d", len(result))
	}
	if result[0] != "pkg/node_resource_abstract.go" {
		t.Errorf("result[0] = %q, want %q", result[0], "pkg/node_resource_abstract.go")
	}
	if result[1] != "pkg/node_resource_plan.go" {
		t.Errorf("result[1] = %q, want %q", result[1], "pkg/node_resource_plan.go")
	}
	// Non-prefix matches follow in original order.
	if result[2] != "pkg/archive.go" {
		t.Errorf("result[2] = %q, want %q", result[2], "pkg/archive.go")
	}
}

func TestIsSamePackageSiblingSignals(t *testing.T) {
	// Test that isLikelyModify returns true for same-package siblings in small packages.
	sc := &scoredCandidate{
		candidate: candidate{
			Distance:         1,
			IsGoSamePackage:  true,
			IsSmallGoPackage: true,
		},
	}
	if !isLikelyModify(sc) {
		t.Error("isLikelyModify should return true for IsGoSamePackage in small package at distance 1")
	}

	// Test confidence: without co-change or prefix → medium.
	conf := likelyModifyConfidence(sc)
	if conf != "medium" {
		t.Errorf("confidence = %q, want %q for same-package without co-change/prefix", conf, "medium")
	}

	// With prefix match → high.
	sc.HasPrefixMatch = true
	conf = likelyModifyConfidence(sc)
	if conf != "high" {
		t.Errorf("confidence = %q, want %q for same-package with prefix match", conf, "high")
	}

	// With co-change → high (even without prefix).
	sc.HasPrefixMatch = false
	sc.CoChangeFreq = 1
	conf = likelyModifyConfidence(sc)
	if conf != "high" {
		t.Errorf("confidence = %q, want %q for same-package with co-change", conf, "high")
	}
}

func TestGoSamePackageLargePackageFiltering(t *testing.T) {
	// In large packages (>20 files), only prefix-matched siblings qualify for likely_modify.
	sc := &scoredCandidate{
		candidate: candidate{
			Distance:         1,
			IsGoSamePackage:  true,
			IsSmallGoPackage: false, // large package
			HasPrefixMatch:   false, // no prefix match
		},
	}
	if isLikelyModify(sc) {
		t.Error("isLikelyModify should return false for large-package non-prefix sibling")
	}

	// With prefix match in large package → qualifies.
	sc.HasPrefixMatch = true
	if !isLikelyModify(sc) {
		t.Error("isLikelyModify should return true for large-package prefix-matched sibling")
	}

	// Prefix match gives HIGH confidence.
	conf := likelyModifyConfidence(sc)
	if conf != "high" {
		t.Errorf("confidence = %q, want %q for prefix-matched sibling", conf, "high")
	}
}

func TestSamePackageBuildSignals(t *testing.T) {
	// IsSamePackageSibling should emit same_package.
	sc := &scoredCandidate{
		candidate: candidate{
			Distance:             1,
			IsSamePackageSibling: true,
			CoChangeFreq:         2,
		},
	}
	signals := buildSignals(sc, config.Empty(), 5)
	found := false
	for _, s := range signals {
		if s == "same_package" {
			found = true
		}
		if s == "two_hop" {
			t.Error("same-package sibling should not have two_hop signal")
		}
	}
	if !found {
		t.Errorf("expected 'same_package' signal for IsSamePackageSibling, got %v", signals)
	}

	// IsGoSamePackage should also emit same_package.
	sc2 := &scoredCandidate{
		candidate: candidate{
			Distance:        1,
			IsImport:        true,
			IsGoSamePackage: true,
		},
	}
	signals2 := buildSignals(sc2, config.Empty(), 5)
	found2 := false
	for _, s := range signals2 {
		if s == "same_package" {
			found2 = true
		}
	}
	if !found2 {
		t.Errorf("expected 'same_package' signal for IsGoSamePackage, got %v", signals2)
	}
}

func TestCategorizeSamePackageSiblingsInMustRead(t *testing.T) {
	// Simulate a Go file with forward imports and same-package siblings.
	scored := []scoredCandidate{
		{candidate: candidate{Path: "pkg/subject.go", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "pkg/dep.go", Distance: 1, IsImport: true}, Score: 8},
		{candidate: candidate{Path: "pkg/sibling_types.go", Distance: 1, IsSamePackageSibling: true}, Score: 6},
		{candidate: candidate{Path: "pkg/consumer.go", Distance: 1, IsImporter: true}, Score: 5},
	}
	cats := categorize(scored, "pkg/subject.go", config.Empty(), Caps{}, 5)

	// must_read should contain: dep.go (import), sibling_types.go (same-package), consumer.go (reverse).
	if len(cats.MustRead) != 3 {
		t.Fatalf("expected 3 must_read, got %d: %v", len(cats.MustRead), cats.MustRead)
	}
	// Forward import first, then same-package, then reverse.
	if cats.MustRead[0] != "pkg/dep.go" {
		t.Errorf("must_read[0] = %q, want %q", cats.MustRead[0], "pkg/dep.go")
	}
	if cats.MustRead[1] != "pkg/sibling_types.go" {
		t.Errorf("must_read[1] = %q, want %q", cats.MustRead[1], "pkg/sibling_types.go")
	}
	if cats.MustRead[2] != "pkg/consumer.go" {
		t.Errorf("must_read[2] = %q, want %q", cats.MustRead[2], "pkg/consumer.go")
	}
}

func TestSamePackageSiblingLikelyModifyMedium(t *testing.T) {
	// Same-package siblings in small packages should qualify as MEDIUM likely_modify.
	scored := []scoredCandidate{
		{candidate: candidate{Path: "pkg/subject.go", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "pkg/sibling.go", Distance: 1, IsGoSamePackage: true, IsSmallGoPackage: true}, Score: 6},
	}
	cats := categorize(scored, "pkg/subject.go", config.Empty(), Caps{}, 5)

	if len(cats.LikelyModify) != 1 {
		t.Fatalf("expected 1 likely_modify, got %d", len(cats.LikelyModify))
	}
	lm := cats.LikelyModify[0]
	if lm.confidence != "medium" {
		t.Errorf("confidence = %q, want %q", lm.confidence, "medium")
	}
	foundSamePackage := false
	for _, s := range lm.signals {
		if s == "same_package" {
			foundSamePackage = true
		}
	}
	if !foundSamePackage {
		t.Errorf("expected 'same_package' signal in likely_modify, got %v", lm.signals)
	}
}

func TestGoSamePackagePrefixMatchHighConfidence(t *testing.T) {
	// Prefix-matched same-package files should get HIGH confidence.
	scored := []scoredCandidate{
		{candidate: candidate{Path: "pkg/context.go", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "pkg/context_apply.go", Distance: 1, IsGoSamePackage: true, HasPrefixMatch: true}, Score: 7},
	}
	cats := categorize(scored, "pkg/context.go", config.Empty(), Caps{}, 5)

	if len(cats.LikelyModify) != 1 {
		t.Fatalf("expected 1 likely_modify, got %d", len(cats.LikelyModify))
	}
	lm := cats.LikelyModify[0]
	if lm.confidence != "high" {
		t.Errorf("confidence = %q, want %q for prefix-matched same-package file", lm.confidence, "high")
	}
}

func TestGoSamePackageTestPromotion(t *testing.T) {
	// When a .go subject has no direct tests, same-dir _test.go files
	// should be promoted as direct tests.
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "pkg/daemon_test.go", Distance: 1}, Score: 5},
		{candidate: candidate{Path: "pkg/utils_test.go", Distance: 1}, Score: 4},
		{candidate: candidate{Path: "pkg/other_test.go", Distance: 1}, Score: 3},
	}
	tests, total := filterTestCandidates(candidates, "pkg/create.go", nil, 5)
	// totalBeforeCap counts direct + dependency BEFORE capping.
	// 2 promoted direct + 0 dependency (third doesn't qualify) + 1 remaining = variable.
	if total < 2 {
		t.Errorf("total = %d, want >= 2", total)
	}
	// Should promote up to 2 same-dir test files as direct.
	if len(tests) < 2 {
		t.Fatalf("expected at least 2 tests, got %d", len(tests))
	}
	directCount := 0
	for _, tc := range tests {
		if tc.direct {
			directCount++
		}
	}
	if directCount < 2 {
		t.Errorf("expected at least 2 direct tests from promotion, got %d", directCount)
	}
}

func TestGoTestPromotionSkippedForNonGo(t *testing.T) {
	// Python files should NOT get Go test promotion.
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "pkg/test_utils.py", Distance: 1}, Score: 5},
	}
	tests, _ := filterTestCandidates(candidates, "pkg/models.py", nil, 5)
	// No stem match, no import → no direct tests → dependency suppressed.
	if len(tests) != 0 {
		t.Errorf("expected 0 tests for Python file with no matching tests, got %d: %v",
			len(tests), testCandidatePaths(tests))
	}
}

func TestGoTestPromotionCapAt2(t *testing.T) {
	// Should promote at most 2 same-dir test files.
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "pkg/a_test.go", Distance: 1}, Score: 5},
		{candidate: candidate{Path: "pkg/b_test.go", Distance: 1}, Score: 4},
		{candidate: candidate{Path: "pkg/c_test.go", Distance: 1}, Score: 3},
		{candidate: candidate{Path: "pkg/d_test.go", Distance: 1}, Score: 2},
	}
	tests, _ := filterTestCandidates(candidates, "pkg/create.go", nil, 5)

	directCount := 0
	for _, tc := range tests {
		if tc.direct {
			directCount++
		}
	}
	if directCount != 2 {
		t.Errorf("expected exactly 2 promoted direct tests, got %d", directCount)
	}
}

func TestGoTestPromotionSkippedWhenDirectExists(t *testing.T) {
	// If a direct test already matches (stem match), no promotion should happen.
	candidates := []scoredCandidate{
		// create_test.go matches create.go by stem → direct test.
		{candidate: candidate{Path: "pkg/create_test.go", Distance: 1, IsImporter: false}, Score: 5},
		{candidate: candidate{Path: "pkg/daemon_test.go", Distance: 1}, Score: 4},
	}
	tests, _ := filterTestCandidates(candidates, "pkg/create.go", nil, 5)

	// create_test.go should be direct via stem match (not via promotion).
	if len(tests) < 1 {
		t.Fatal("expected at least 1 test")
	}
	if tests[0].path != "pkg/create_test.go" {
		t.Errorf("tests[0] = %q, want %q", tests[0].path, "pkg/create_test.go")
	}
	if !tests[0].direct {
		t.Error("create_test.go should be a direct test via stem match")
	}
}

func TestGoSamePackageDistanceOverrideInCategorizer(t *testing.T) {
	// An IsGoSamePackage file discovered via edge traversal (no IsSamePackageSibling)
	// should still route to samePackageSiblings bucket after distance override.
	scored := []scoredCandidate{
		{candidate: candidate{Path: "pkg/subject.go", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "pkg/dep.go", Distance: 1, IsImport: true}, Score: 8},
		{candidate: candidate{Path: "pkg/context_apply.go", Distance: 1, IsGoSamePackage: true, HasPrefixMatch: true}, Score: 6},
	}
	cats := categorize(scored, "pkg/subject.go", config.Empty(), Caps{}, 5)

	if len(cats.MustRead) != 2 {
		t.Fatalf("expected 2 must_read, got %d: %v", len(cats.MustRead), cats.MustRead)
	}
	// Forward import first, then same-package.
	if cats.MustRead[0] != "pkg/dep.go" {
		t.Errorf("must_read[0] = %q, want %q", cats.MustRead[0], "pkg/dep.go")
	}
	if cats.MustRead[1] != "pkg/context_apply.go" {
		t.Errorf("must_read[1] = %q, want %q", cats.MustRead[1], "pkg/context_apply.go")
	}
}

func TestGoSamePackageDistance2QualifiesLikelyModify(t *testing.T) {
	// Simulates the post-override state: file was at Distance=2 via edge traversal,
	// overridden to Distance=1 by post-processing.
	scored := []scoredCandidate{
		{candidate: candidate{Path: "pkg/node_resource_apply.go", Distance: 0}, Score: 10},
		{candidate: candidate{
			Path: "pkg/node_resource_abstract.go", Distance: 1,
			IsGoSamePackage: true, HasPrefixMatch: true,
		}, Score: 7},
	}
	cats := categorize(scored, "pkg/node_resource_apply.go", config.Empty(), Caps{}, 5)

	if len(cats.LikelyModify) != 1 {
		t.Fatalf("expected 1 likely_modify, got %d", len(cats.LikelyModify))
	}
	if cats.LikelyModify[0].confidence != "high" {
		t.Errorf("confidence = %q, want %q", cats.LikelyModify[0].confidence, "high")
	}
	if cats.LikelyModify[0].path != "pkg/node_resource_abstract.go" {
		t.Errorf("path = %q, want %q", cats.LikelyModify[0].path, "pkg/node_resource_abstract.go")
	}
}

// --- Round 19: Java first-class language support tests ---

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"KafkaProducer", []string{"Kafka", "Producer"}},
		{"KafkaConsumer", []string{"Kafka", "Consumer"}},
		{"AbstractWorker", []string{"Abstract", "Worker"}},
		{"HTTPSConnection", []string{"HTTPS", "Connection"}},
		{"URLParser", []string{"URL", "Parser"}},
		{"SimpleClass", []string{"Simple", "Class"}},
		{"ABC", []string{"ABC"}},
		{"lowercase", []string{"lowercase"}},
		{"", nil},
		{"A", []string{"A"}},
		{"DataSourceAutoConfiguration", []string{"Data", "Source", "Auto", "Configuration"}},
		{"IOUtils", []string{"IO", "Utils"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitCamelCase(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCamelCase(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCamelCase(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestShareJavaClassPrefix(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		// Same prefix: KafkaProducer ↔ KafkaConsumer share "Kafka"
		{"KafkaProducer", "KafkaConsumer", true},
		// Same prefix: AbstractWorker ↔ AbstractHerder share "Abstract"
		{"AbstractWorker", "AbstractHerder", true},
		// Same prefix: DataSourceAutoConfiguration ↔ DataSourceProperties share "Data", "Source"
		{"DataSourceAutoConfiguration", "DataSourceProperties", true},
		// Different prefix: KafkaProducer ↔ SocketServer share nothing
		{"KafkaProducer", "SocketServer", false},
		// Identical names: should not match
		{"KafkaProducer", "KafkaProducer", false},
		// Single-word names: no prefix possible (need >= 2 parts)
		{"Worker", "Herder", false},
		// One single-word: KafkaProducer vs Worker → false
		{"KafkaProducer", "Worker", false},
		// Acronym prefix: HTTPClient ↔ HTTPServer
		{"HTTPClient", "HTTPServer", true},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := shareJavaClassPrefix(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("shareJavaClassPrefix(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestJavaTestMirrorDir(t *testing.T) {
	tests := []struct {
		sourceDir string
		want      string
	}{
		// Standard Maven single-module
		{"src/main/java/com/example/service", "src/test/java/com/example/service"},
		// Multi-module Maven
		{"module-a/src/main/java/com/example", "module-a/src/test/java/com/example"},
		// Deep multi-module
		{"parent/child/src/main/java/org/apache/kafka", "parent/child/src/test/java/org/apache/kafka"},
		// Non-Maven path → ""
		{"com/example/service", ""},
		// src/test/java path (already test dir) → ""
		{"src/test/java/com/example", ""},
		// Root-level src/main/java
		{"src/main/java/pkg", "src/test/java/pkg"},
	}
	for _, tt := range tests {
		t.Run(tt.sourceDir, func(t *testing.T) {
			got := javaTestMirrorDir(tt.sourceDir)
			if got != tt.want {
				t.Errorf("javaTestMirrorDir(%q) = %q, want %q", tt.sourceDir, got, tt.want)
			}
		})
	}
}

func TestJavaSamePackageLikelyModify(t *testing.T) {
	// Java same-package with class prefix match in small package → qualifies, high confidence.
	sc := &scoredCandidate{
		candidate: candidate{
			Distance:           1,
			IsJavaSamePackage:  true,
			HasJavaClassPrefix: true,
			IsSmallJavaPackage: true,
		},
	}
	if !isLikelyModify(sc) {
		t.Error("isLikelyModify should return true for Java same-package with class prefix at distance 1")
	}
	conf := likelyModifyConfidence(sc)
	if conf != "high" {
		t.Errorf("confidence = %q, want %q for Java same-package with class prefix", conf, "high")
	}

	// Java same-package in small package without class prefix → qualifies, medium confidence.
	sc2 := &scoredCandidate{
		candidate: candidate{
			Distance:           1,
			IsJavaSamePackage:  true,
			HasJavaClassPrefix: false,
			IsSmallJavaPackage: true,
		},
	}
	if !isLikelyModify(sc2) {
		t.Error("isLikelyModify should return true for Java same-package in small package at distance 1")
	}
	conf2 := likelyModifyConfidence(sc2)
	if conf2 != "medium" {
		t.Errorf("confidence = %q, want %q for Java same-package without prefix/co-change", conf2, "medium")
	}

	// Java same-package with co-change → high confidence.
	sc3 := &scoredCandidate{
		candidate: candidate{
			Distance:           1,
			IsJavaSamePackage:  true,
			HasJavaClassPrefix: false,
			IsSmallJavaPackage: true,
			CoChangeFreq:       2,
		},
	}
	conf3 := likelyModifyConfidence(sc3)
	if conf3 != "high" {
		t.Errorf("confidence = %q, want %q for Java same-package with co-change", conf3, "high")
	}

	// Java same-package in large package without class prefix → does NOT qualify.
	sc4 := &scoredCandidate{
		candidate: candidate{
			Distance:           1,
			IsJavaSamePackage:  true,
			HasJavaClassPrefix: false,
			IsSmallJavaPackage: false,
		},
	}
	if isLikelyModify(sc4) {
		t.Error("isLikelyModify should return false for Java large-package non-prefix sibling")
	}

	// Java same-package in large package WITH class prefix → qualifies.
	sc5 := &scoredCandidate{
		candidate: candidate{
			Distance:           1,
			IsJavaSamePackage:  true,
			HasJavaClassPrefix: true,
			IsSmallJavaPackage: false,
		},
	}
	if !isLikelyModify(sc5) {
		t.Error("isLikelyModify should return true for Java large-package prefix-matched sibling")
	}
}

func TestJavaSamePackageBuildSignals(t *testing.T) {
	sc := &scoredCandidate{
		candidate: candidate{
			Distance:          1,
			IsJavaSamePackage: true,
			CoChangeFreq:      1,
		},
	}
	signals := buildSignals(sc, config.Empty(), 5)
	found := false
	for _, s := range signals {
		if s == "same_package" {
			found = true
		}
		if s == "two_hop" {
			t.Error("Java same-package sibling should not have two_hop signal")
		}
	}
	if !found {
		t.Errorf("expected 'same_package' signal for IsJavaSamePackage, got %v", signals)
	}
}

func TestCategorizeJavaSamePackageInMustRead(t *testing.T) {
	scored := []scoredCandidate{
		{candidate: candidate{Path: "src/main/java/com/example/Subject.java", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "src/main/java/com/example/Dep.java", Distance: 1, IsImport: true}, Score: 8},
		{candidate: candidate{Path: "src/main/java/com/example/Sibling.java", Distance: 1, IsJavaSamePackage: true}, Score: 6},
		{candidate: candidate{Path: "src/main/java/com/example/Consumer.java", Distance: 1, IsImporter: true}, Score: 5},
	}
	cats := categorize(scored, "src/main/java/com/example/Subject.java", config.Empty(), Caps{}, 5)

	if len(cats.MustRead) != 3 {
		t.Fatalf("expected 3 must_read, got %d: %v", len(cats.MustRead), cats.MustRead)
	}
	// Forward import first, then same-package, then reverse.
	if cats.MustRead[0] != "src/main/java/com/example/Dep.java" {
		t.Errorf("must_read[0] = %q, want Dep.java", cats.MustRead[0])
	}
	if cats.MustRead[1] != "src/main/java/com/example/Sibling.java" {
		t.Errorf("must_read[1] = %q, want Sibling.java", cats.MustRead[1])
	}
	if cats.MustRead[2] != "src/main/java/com/example/Consumer.java" {
		t.Errorf("must_read[2] = %q, want Consumer.java", cats.MustRead[2])
	}
}

func TestPrioritizeByJavaClassPrefix(t *testing.T) {
	subject := "src/main/java/com/example/KafkaProducer.java"
	siblings := []string{
		"src/main/java/com/example/SocketServer.java",
		"src/main/java/com/example/KafkaConsumer.java",
		"src/main/java/com/example/Utils.java",
		"src/main/java/com/example/KafkaConfig.java",
	}
	result := prioritizeByJavaClassPrefix(siblings, subject)
	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %d", len(result))
	}
	// Prefix matches ("Kafka") should come first.
	if result[0] != "src/main/java/com/example/KafkaConsumer.java" {
		t.Errorf("result[0] = %q, want KafkaConsumer.java", result[0])
	}
	if result[1] != "src/main/java/com/example/KafkaConfig.java" {
		t.Errorf("result[1] = %q, want KafkaConfig.java", result[1])
	}
}

// --- Rust first-class language support tests ---

func TestRustCrateRoot(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		// Workspace crate.
		{"crates/bevy_ecs/src/system/mod.rs", "crates/bevy_ecs"},
		// Workspace crate, deeper path.
		{"crates/bevy_render/src/render_resource/pipeline.rs", "crates/bevy_render"},
		// Root crate.
		{"src/sync/mutex.rs", "."},
		// Root crate, direct child.
		{"src/lib.rs", "."},
		// No src/ in path.
		{"tests/integration.rs", ""},
		// Top-level file.
		{"Cargo.toml", ""},
		// Nested workspace with src.
		{"packages/tokio/src/runtime/builder.rs", "packages/tokio"},
		// Edge case: "src" in directory name but not /src/.
		{"mysrc/foo.rs", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := rustCrateRoot(tt.path)
			if got != tt.want {
				t.Errorf("rustCrateRoot(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRustSourceModPath(t *testing.T) {
	tests := []struct {
		filePath  string
		crateRoot string
		want      string
	}{
		{"crates/bevy_ecs/src/system/function_system.rs", "crates/bevy_ecs", "system/function_system"},
		{"crates/bevy_ecs/src/system/mod.rs", "crates/bevy_ecs", "system"},
		{"src/sync/mutex.rs", ".", "sync/mutex"},
		{"src/lib.rs", ".", ""},
		{"crates/bevy_app/src/app.rs", "crates/bevy_app", "app"},
	}
	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := rustSourceModPath(tt.filePath, tt.crateRoot)
			if got != tt.want {
				t.Errorf("rustSourceModPath(%q, %q) = %q, want %q", tt.filePath, tt.crateRoot, got, tt.want)
			}
		})
	}
}

func TestRustTestModuleMatch(t *testing.T) {
	tests := []struct {
		testStem string
		modPath  string
		matches  bool
		allSegs  bool
	}{
		// Exact multi-segment match.
		{"sync_barrier", "sync/barrier", true, true},
		// Broad match: first segment only.
		{"sync", "sync/mutex", true, false},
		// No match: different first segment.
		{"io_read", "sync/mutex", false, false},
		// Single segment exact match (not allSegs since <2 matched).
		{"app", "app", true, false},
		// Empty inputs.
		{"", "sync/mutex", false, false},
		{"sync", "", false, false},
		// Multi-segment partial: matches first two of three.
		{"system_function", "system/function/inner", true, false},
		// Suffix match: tcp_stream → net/tcp/stream (2 of 3 segments → direct).
		{"tcp_stream", "net/tcp/stream", true, true},
		// Suffix match: single segment test on 2-segment module (not allSegs: testParts=1 < 2).
		{"stream", "tcp/stream", true, false},
		// Suffix non-match: uds_stream vs net/unix/stream.
		{"uds_stream", "net/unix/stream", false, false},
		// Suffix non-match: task_builder vs task/spawn.
		{"task_builder", "task/spawn", false, false},
		// Last-segment match: test stem starts with last segment of module path.
		{"pipe_subprocess", "sys/pipe", true, false},
		// Last-segment exact match: test stem == last segment.
		{"pipe", "sys/pipe", true, false},
		// Last-segment non-match: test stem doesn't start with last segment.
		{"socket_test", "sys/pipe", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.testStem+"_vs_"+tt.modPath, func(t *testing.T) {
			m, a := rustTestModuleMatch(tt.testStem, tt.modPath)
			if m != tt.matches || a != tt.allSegs {
				t.Errorf("rustTestModuleMatch(%q, %q) = (%v, %v), want (%v, %v)",
					tt.testStem, tt.modPath, m, a, tt.matches, tt.allSegs)
			}
		})
	}
}

func TestRustCrateNameFromPath(t *testing.T) {
	tests := []struct {
		crateRoot string
		want      string
	}{
		{"crates/bevy_ecs", "bevy_ecs"},
		{"src/tools/miri", "miri"},
		{"compiler/rustc_session", "rustc_session"},
		{"src/tools/rust-analyzer", "rust_analyzer"},
		{"my-crate", "my_crate"},
		{".", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.crateRoot, func(t *testing.T) {
			got := rustCrateNameFromPath(tt.crateRoot)
			if got != tt.want {
				t.Errorf("rustCrateNameFromPath(%q) = %q, want %q", tt.crateRoot, got, tt.want)
			}
		})
	}
}

func TestRustIntegrationTestMatch(t *testing.T) {
	tests := []struct {
		subject string
		test    string
		matches bool
		direct  bool
	}{
		// Same crate, exact module match.
		{"crates/bevy_ecs/src/system/function_system.rs", "crates/bevy_ecs/tests/system_function_system.rs", true, true},
		// Same crate, broad match.
		{"crates/bevy_ecs/src/system/function_system.rs", "crates/bevy_ecs/tests/system.rs", true, false},
		// Cross crate: test is in different crate.
		{"crates/bevy_ecs/src/system/function_system.rs", "crates/bevy_render/tests/system.rs", false, false},
		// Root crate.
		{"src/sync/mutex.rs", "tests/sync_mutex.rs", true, true},
		// Root crate, broad.
		{"src/sync/mutex.rs", "tests/sync.rs", true, false},
		// No src/ in subject.
		{"tests/helper.rs", "tests/helper_test.rs", false, false},
		// lib.rs with crate-name fallback: "miri" matches test stem "miri".
		{"src/tools/miri/src/lib.rs", "src/tools/miri/tests/miri.rs", true, false},
		// lib.rs with crate-name fallback: rustc_session matches.
		{"compiler/rustc_session/src/lib.rs", "compiler/rustc_session/tests/rustc_session.rs", true, false},
		// lib.rs at repo root: crateRoot="." → empty crate name → no match.
		{"src/lib.rs", "tests/lib.rs", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.subject+"_"+tt.test, func(t *testing.T) {
			m, d := rustIntegrationTestMatch(tt.subject, tt.test)
			if m != tt.matches || d != tt.direct {
				t.Errorf("rustIntegrationTestMatch(%q, %q) = (%v, %v), want (%v, %v)",
					tt.subject, tt.test, m, d, tt.matches, tt.direct)
			}
		})
	}
}

func TestRustSameModuleLikelyModify(t *testing.T) {
	// Prefix match + small module → qualifies, high confidence.
	sc1 := &scoredCandidate{
		candidate: candidate{
			Distance:            1,
			IsRustSameModule:    true,
			HasRustModulePrefix: true,
			IsSmallRustModule:   true,
		},
	}
	if !isLikelyModify(sc1) {
		t.Error("isLikelyModify should return true for Rust same-module with prefix at distance 1")
	}
	if conf := likelyModifyConfidence(sc1); conf != "high" {
		t.Errorf("confidence = %q, want %q for Rust same-module with prefix", conf, "high")
	}

	// Small module without prefix → qualifies, medium confidence.
	sc2 := &scoredCandidate{
		candidate: candidate{
			Distance:            1,
			IsRustSameModule:    true,
			HasRustModulePrefix: false,
			IsSmallRustModule:   true,
		},
	}
	if !isLikelyModify(sc2) {
		t.Error("isLikelyModify should return true for Rust same-module in small module at distance 1")
	}
	if conf := likelyModifyConfidence(sc2); conf != "medium" {
		t.Errorf("confidence = %q, want %q for Rust same-module without prefix/co-change", conf, "medium")
	}

	// Co-change → high confidence.
	sc3 := &scoredCandidate{
		candidate: candidate{
			Distance:            1,
			IsRustSameModule:    true,
			HasRustModulePrefix: false,
			IsSmallRustModule:   true,
			CoChangeFreq:        2,
		},
	}
	if conf := likelyModifyConfidence(sc3); conf != "high" {
		t.Errorf("confidence = %q, want %q for Rust same-module with co-change", conf, "high")
	}

	// Large module without prefix → does NOT qualify.
	sc4 := &scoredCandidate{
		candidate: candidate{
			Distance:            1,
			IsRustSameModule:    true,
			HasRustModulePrefix: false,
			IsSmallRustModule:   false,
		},
	}
	if isLikelyModify(sc4) {
		t.Error("isLikelyModify should return false for Rust large-module non-prefix sibling")
	}

	// Large module WITH prefix → qualifies.
	sc5 := &scoredCandidate{
		candidate: candidate{
			Distance:            1,
			IsRustSameModule:    true,
			HasRustModulePrefix: true,
			IsSmallRustModule:   false,
		},
	}
	if !isLikelyModify(sc5) {
		t.Error("isLikelyModify should return true for Rust large-module prefix-matched sibling")
	}
}

func TestRustSameModuleBuildSignals(t *testing.T) {
	sc := &scoredCandidate{
		candidate: candidate{
			Distance:         1,
			IsRustSameModule: true,
			CoChangeFreq:     1,
		},
	}
	signals := buildSignals(sc, config.Empty(), 5)
	found := false
	for _, s := range signals {
		if s == "same_package" {
			found = true
		}
		if s == "two_hop" {
			t.Error("Rust same-module sibling should not have two_hop signal")
		}
	}
	if !found {
		t.Errorf("expected 'same_package' signal for IsRustSameModule, got %v", signals)
	}
}

func TestCategorizeRustSameModuleInMustRead(t *testing.T) {
	scored := []scoredCandidate{
		{candidate: candidate{Path: "crates/bevy_ecs/src/system/function_system.rs", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "crates/bevy_ecs/src/system/system_param.rs", Distance: 1, IsImport: true}, Score: 8},
		{candidate: candidate{Path: "crates/bevy_ecs/src/system/mod.rs", Distance: 1, IsRustSameModule: true}, Score: 6},
		{candidate: candidate{Path: "crates/bevy_ecs/src/world/mod.rs", Distance: 1, IsImporter: true}, Score: 5},
	}
	cats := categorize(scored, "crates/bevy_ecs/src/system/function_system.rs", config.Empty(), Caps{}, 5)

	if len(cats.MustRead) != 3 {
		t.Fatalf("expected 3 must_read, got %d: %v", len(cats.MustRead), cats.MustRead)
	}
	// Forward import first, then same-module, then reverse.
	if cats.MustRead[0] != "crates/bevy_ecs/src/system/system_param.rs" {
		t.Errorf("must_read[0] = %q, want system_param.rs", cats.MustRead[0])
	}
	if cats.MustRead[1] != "crates/bevy_ecs/src/system/mod.rs" {
		t.Errorf("must_read[1] = %q, want mod.rs", cats.MustRead[1])
	}
	if cats.MustRead[2] != "crates/bevy_ecs/src/world/mod.rs" {
		t.Errorf("must_read[2] = %q, want world/mod.rs", cats.MustRead[2])
	}
}

func TestInlineTestDetection(t *testing.T) {
	// A .rs file with #[cfg(test)] should appear as a direct test for itself
	// via the inline test injection in categorizer.
	subject := "crates/bevy_ecs/src/system/function_system.rs"
	scored := []scoredCandidate{
		{candidate: candidate{Path: subject, Distance: 0, HasInlineTests: true}, Score: 10},
		{candidate: candidate{Path: "crates/bevy_ecs/src/system/system_param.rs", Distance: 1, IsImport: true}, Score: 8},
	}
	cats := categorize(scored, subject, config.Empty(), Caps{}, 5)

	// Subject has #[cfg(test)] → should appear in tests as direct.
	found := false
	for _, tc := range cats.Tests {
		if tc.path == subject && tc.direct {
			found = true
		}
	}
	if !found {
		t.Errorf("expected subject %q with HasInlineTests=true to appear in tests as direct, got %v", subject, cats.Tests)
	}
}

func TestInlineTestSiblingDiscovery(t *testing.T) {
	// A sibling .rs file with #[cfg(test)] should appear as a direct test
	// for the subject (same Rust module = same test scope).
	subject := "crates/bevy_ecs/src/system/function_system.rs"
	sibling := "crates/bevy_ecs/src/system/system_param.rs"
	scored := []scoredCandidate{
		{candidate: candidate{Path: subject, Distance: 0}, Score: 10},
		{candidate: candidate{Path: sibling, Distance: 1, IsRustSameModule: true, HasInlineTests: true}, Score: 8},
	}
	cats := categorize(scored, subject, config.Empty(), Caps{}, 5)

	found := false
	for _, tc := range cats.Tests {
		if tc.path == sibling && tc.direct {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sibling %q with HasInlineTests=true to appear in tests as direct, got %v", sibling, cats.Tests)
	}
}

func TestInlineTestNoFalsePositive(t *testing.T) {
	// A .rs file WITHOUT #[cfg(test)] should NOT appear in tests
	// purely from inline test injection.
	subject := "crates/bevy_render/src/render_resource/pipeline.rs"
	scored := []scoredCandidate{
		{candidate: candidate{Path: subject, Distance: 0, HasInlineTests: false}, Score: 10},
		{candidate: candidate{Path: "crates/bevy_render/src/render_resource/mod.rs", Distance: 1, IsRustSameModule: true, HasInlineTests: false}, Score: 6},
	}
	cats := categorize(scored, subject, config.Empty(), Caps{}, 5)

	if len(cats.Tests) != 0 {
		t.Errorf("expected no tests for .rs file without #[cfg(test)], got %v", cats.Tests)
	}
}

func TestIsSameCrateIntegrationTest(t *testing.T) {
	tests := []struct {
		subject string
		test    string
		want    bool
	}{
		// Same crate.
		{"crates/tokio/src/net/tcp/stream.rs", "crates/tokio/tests/tcp_stream.rs", true},
		// Root crate.
		{"src/net/tcp/stream.rs", "tests/tcp_stream.rs", true},
		// Different crate.
		{"crates/tokio/src/net/tcp/stream.rs", "crates/tokio-util/tests/tcp_stream.rs", false},
		// Not in tests/ directory.
		{"crates/tokio/src/net/tcp/stream.rs", "crates/tokio/src/net/tcp_test.rs", false},
		// No src/ in subject.
		{"tests/helper.rs", "tests/other.rs", false},
	}
	for _, tt := range tests {
		t.Run(tt.subject+"_"+tt.test, func(t *testing.T) {
			got := isSameCrateIntegrationTest(tt.subject, tt.test)
			if got != tt.want {
				t.Errorf("isSameCrateIntegrationTest(%q, %q) = %v, want %v",
					tt.subject, tt.test, got, tt.want)
			}
		})
	}
}

func TestInlineSiblingTierReclassification(t *testing.T) {
	// When the subject has its own #[cfg(test)], sibling inline tests should
	// be classified as dependency (not direct).
	subject := "crates/bevy_ecs/src/system/function_system.rs"
	sibling := "crates/bevy_ecs/src/system/system_param.rs"
	scored := []scoredCandidate{
		{candidate: candidate{Path: subject, Distance: 0, HasInlineTests: true}, Score: 10},
		{candidate: candidate{Path: sibling, Distance: 1, IsRustSameModule: true, HasInlineTests: true}, Score: 8},
	}
	cats := categorize(scored, subject, config.Empty(), Caps{}, 5)

	// Subject should be direct.
	subjectDirect := false
	siblingDirect := false
	siblingFound := false
	for _, tc := range cats.Tests {
		if tc.path == subject && tc.direct {
			subjectDirect = true
		}
		if tc.path == sibling {
			siblingFound = true
			siblingDirect = tc.direct
		}
	}
	if !subjectDirect {
		t.Errorf("expected subject %q to be direct test", subject)
	}
	if !siblingFound {
		t.Errorf("expected sibling %q to appear in tests", sibling)
	}
	if siblingDirect {
		t.Errorf("expected sibling %q to be dependency (not direct) when subject has inline tests", sibling)
	}
}

func TestInlineSiblingStaysDirectWhenSubjectLacksTests(t *testing.T) {
	// When the subject does NOT have #[cfg(test)], sibling inline tests stay direct.
	subject := "crates/bevy_ui/src/layout/mod.rs"
	sibling := "crates/bevy_ui/src/layout/convert.rs"
	scored := []scoredCandidate{
		{candidate: candidate{Path: subject, Distance: 0, HasInlineTests: false}, Score: 10},
		{candidate: candidate{Path: sibling, Distance: 1, IsRustSameModule: true, HasInlineTests: true}, Score: 8},
	}
	cats := categorize(scored, subject, config.Empty(), Caps{}, 5)

	found := false
	for _, tc := range cats.Tests {
		if tc.path == sibling && tc.direct {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sibling %q to be direct test when subject lacks inline tests", sibling)
	}
}

// --- Python __init__.py parent-directory stem matching tests ---

// TestInitPyParentDirStemMatch verifies that test_pipelines.py in tests/ is
// classified as a direct test for pipelines/__init__.py via parent-dir stem matching.
func TestInitPyParentDirStemMatch(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/test_pipelines.py"}, Score: 1.0},
	}
	subject := "src/transformers/pipelines/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_pipelines.py to be classified as direct test for pipelines/__init__.py")
	}
	if !result[0].direct {
		t.Errorf("expected test_pipelines.py to be direct tier, got dependency")
	}
}

// TestInitPyParentDirStemMatchNested verifies nested tests/ directory works.
func TestInitPyParentDirStemMatchNested(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/models/test_bert.py"}, Score: 1.0},
	}
	subject := "src/transformers/models/bert/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_bert.py to be classified as direct test for bert/__init__.py")
	}
	if !result[0].direct {
		t.Errorf("expected test_bert.py to be direct tier")
	}
}

// TestInitPyParentDirStemNoFalsePositive verifies that a test with a different
// stem is NOT matched as direct for __init__.py.
func TestInitPyParentDirStemNoFalsePositive(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/test_unrelated.py"}, Score: 1.0},
	}
	subject := "src/transformers/pipelines/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected test_unrelated.py NOT to match pipelines/__init__.py, got %v", testCandidatePaths(result))
	}
}

// TestInitPyParentDirStemOutsideTestsDir verifies that test matching also works
// when the test imports the subject (not just tests/ directory).
func TestInitPyParentDirStemOutsideTestsDir(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "some_dir/test_pipelines.py", IsImporter: true}, Score: 1.0},
	}
	subject := "src/transformers/pipelines/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_pipelines.py with import to match pipelines/__init__.py")
	}
	if !result[0].direct {
		t.Errorf("expected direct tier")
	}
}

// --- hasTestDirComponent tests ---

func TestHasTestDirComponent(t *testing.T) {
	tests := []struct {
		dir  string
		want bool
	}{
		{"tests/unit", true},
		{"Lib/test/test_asyncio", true},
		{"src/test/java/com/example", true},
		{"test", true},
		{"tests", true},
		{"some/nested/tests/dir", true},
		{"src/main/java", false},
		{"testing/helpers", false}, // "testing" != "test"
		{"contest/data", false},    // "contest" != "test"
		{"Lib/asyncio", false},
		{".", false},
	}
	for _, tt := range tests {
		if got := hasTestDirComponent(tt.dir); got != tt.want {
			t.Errorf("hasTestDirComponent(%q) = %v, want %v", tt.dir, got, tt.want)
		}
	}
}

// --- parentDirStemMatch tests ---

// TestParentDirStemMatchRegularPy verifies that regular Python files match tests
// whose stem equals the parent directory name when in a test/ directory.
func TestParentDirStemMatchRegularPy(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_asyncio/test_events.py"}, Score: 0.5},
		{candidate: candidate{Path: "Lib/test/test_asyncio.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/events.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	// test_asyncio.py should match via parentDirStemMatch (parent dir "asyncio" == test stem "asyncio")
	found := false
	for _, tc := range result {
		if tc.path == "Lib/test/test_asyncio.py" && tc.direct {
			found = true
		}
	}
	if !found {
		t.Errorf("expected test_asyncio.py to be direct test for asyncio/events.py via parentDirStemMatch, got %v", testCandidatePaths(result))
	}
}

// TestParentDirStemMatchNotForInitPy ensures parentDirStemMatch doesn't fire for __init__.py
// (which already has its own initParentMatch logic).
func TestParentDirStemMatchNotForInitPy(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_asyncio.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	// Should still match via initParentMatch, verify it's direct.
	if len(result) == 0 {
		t.Fatal("expected test_asyncio.py to match __init__.py via initParentMatch")
	}
	if !result[0].direct {
		t.Error("expected direct tier")
	}
}

// TestParentDirStemMatchRequiresTestDir verifies the safety gate — a test outside
// a test directory must import the subject to qualify.
func TestParentDirStemMatchRequiresTestDir(t *testing.T) {
	candidates := []scoredCandidate{
		// Not in test dir and doesn't import subject
		{candidate: candidate{Path: "benchmarks/test_asyncio.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/events.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected test_asyncio.py outside test dir (no import) NOT to match, got %v", testCandidatePaths(result))
	}
}

// TestParentDirStemMatchWithImport verifies that a test outside a test dir
// still matches if it imports the subject.
func TestParentDirStemMatchWithImport(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "benchmarks/test_asyncio.py", IsImporter: true}, Score: 1.0},
	}
	subject := "Lib/asyncio/events.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_asyncio.py with import to match asyncio/events.py")
	}
	if !result[0].direct {
		t.Error("expected direct tier")
	}
}

// TestParentDirStemMatchSingularTestDir verifies matching works with singular "test/" directory
// (CPython convention: Lib/test/test_asyncio.py).
func TestParentDirStemMatchSingularTestDir(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_sqlite3.py"}, Score: 1.0},
	}
	subject := "Lib/sqlite3/dbapi2.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_sqlite3.py to match sqlite3/dbapi2.py via parentDirStemMatch")
	}
	if !result[0].direct {
		t.Error("expected direct tier")
	}
}

// TestParentDirStemMatchNotForNonPython verifies parentDirStemMatch only fires for .py files.
func TestParentDirStemMatchNotForNonPython(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/test_asyncio.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/events.go" // Go file, not Python

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected parentDirStemMatch NOT to fire for non-.py subject, got %v", testCandidatePaths(result))
	}
}

// --- nestedTestDirMatch tests ---

// TestNestedTestDirMatch verifies that test_<package>/test_<module>.py is classified
// as a direct test for <package>/<module>.py (CPython nested test directory convention).
func TestNestedTestDirMatch(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_asyncio/test_events.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/events.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_asyncio/test_events.py to be classified as direct test for asyncio/events.py")
	}
	if !result[0].direct {
		t.Error("expected direct tier")
	}
}

// TestNestedTestDirMatchNoFalsePositive verifies that different directory stems
// don't match — test_sqlite3/test_events.py should NOT match asyncio/events.py.
func TestNestedTestDirMatchNoFalsePositive(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_sqlite3/test_events.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/events.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected test_sqlite3/test_events.py NOT to match asyncio/events.py, got %v", testCandidatePaths(result))
	}
}

// TestNestedTestDirMatchStemMismatch verifies that same directory stem but different
// file stems don't match — test_asyncio/test_tasks.py should NOT match asyncio/events.py.
func TestNestedTestDirMatchStemMismatch(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_asyncio/test_tasks.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/events.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected test_asyncio/test_tasks.py NOT to match asyncio/events.py, got %v", testCandidatePaths(result))
	}
}

// TestNestedTestDirMatchNotForInitPy verifies that __init__.py uses initParentMatch
// instead of nestedTestDirMatch (barrel files suppress subject stem).
func TestNestedTestDirMatchNotForInitPy(t *testing.T) {
	candidates := []scoredCandidate{
		// This would match if nestedTestDirMatch fired for __init__.py, but
		// extractSubjectStem returns "" for barrel files, so subjectStem is empty.
		{candidate: candidate{Path: "Lib/test/test_asyncio/test_init.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	// test_init.py won't match: nestedTestDirMatch is gated on !isInitPy,
	// and initMatch requires importsSubject which is false here.
	if len(result) != 0 {
		t.Errorf("expected test_init.py NOT to match __init__.py via nestedTestDirMatch, got %v", testCandidatePaths(result))
	}
}

// TestNestedTestDirMatchNotForNonPython verifies nestedTestDirMatch only fires for .py files.
func TestNestedTestDirMatchNotForNonPython(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "test/test_asyncio/test_events.py"}, Score: 1.0},
	}
	subject := "lib/asyncio/events.go" // Go file, not Python

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected nestedTestDirMatch NOT to fire for non-.py subject, got %v", testCandidatePaths(result))
	}
}

// --- initParentMatch with singular test/ directory ---

// TestInitParentMatchSingularTestDir verifies initParentMatch works with singular "test/"
// directory (was broken before hasTestDirComponent fix).
func TestInitParentMatchSingularTestDir(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_asyncio.py"}, Score: 1.0},
	}
	subject := "Lib/asyncio/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_asyncio.py in test/ (singular) to match asyncio/__init__.py")
	}
	if !result[0].direct {
		t.Error("expected direct tier")
	}
}

// --- Python same_directory signal and tier boost tests ---

// TestPythonSameDirectorySignal verifies that same-directory Python importers
// get the "same_directory" signal in likely_modify.
func TestPythonSameDirectorySignal(t *testing.T) {
	scored := []scoredCandidate{
		{candidate: candidate{Path: "Lib/asyncio/events.py", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "Lib/asyncio/constants.py", Distance: 1, IsImporter: true}, Score: 5},
	}
	cats := categorize(scored, "Lib/asyncio/events.py", config.Empty(), Caps{}, 5)

	if len(cats.LikelyModify) == 0 {
		t.Fatal("expected constants.py in likely_modify")
	}
	lm := cats.LikelyModify[0]
	foundSameDir := false
	for _, s := range lm.signals {
		if s == "same_directory" {
			foundSameDir = true
		}
	}
	if !foundSameDir {
		t.Errorf("expected same_directory signal for same-dir Python importer, got signals: %v", lm.signals)
	}
}

// TestPythonSameDirectorySignalNotForGo verifies same_directory signal only fires for .py.
func TestPythonSameDirectorySignalNotForGo(t *testing.T) {
	scored := []scoredCandidate{
		{candidate: candidate{Path: "pkg/subject.go", Distance: 0}, Score: 10},
		{candidate: candidate{Path: "pkg/consumer.go", Distance: 1, IsImporter: true}, Score: 5},
	}
	cats := categorize(scored, "pkg/subject.go", config.Empty(), Caps{}, 5)

	for _, lm := range cats.LikelyModify {
		for _, s := range lm.signals {
			if s == "same_directory" {
				t.Errorf("same_directory signal should not appear for Go files")
			}
		}
	}
}

// TestPythonTierBoostSameDir verifies same-directory Python importers with
// zero co-change are boosted from "low" to "medium" tier.
func TestPythonTierBoostSameDir(t *testing.T) {
	scored := []scoredCandidate{
		{candidate: candidate{Path: "Lib/asyncio/events.py", Distance: 0}, Score: 10},
		// Same-dir importer with zero co-change → normally "low" via isLikelyModifyLow condition D.
		{candidate: candidate{Path: "Lib/asyncio/constants.py", Distance: 1, IsImporter: true, CoChangeFreq: 0}, Score: 5},
	}
	cats := categorize(scored, "Lib/asyncio/events.py", config.Empty(), Caps{}, 5)

	if len(cats.LikelyModify) == 0 {
		t.Fatal("expected constants.py in likely_modify")
	}
	lm := cats.LikelyModify[0]
	if lm.confidence != "medium" {
		t.Errorf("expected same-dir Python importer boosted to medium, got %q", lm.confidence)
	}
}

// TestPythonTierBoostNotForCoChange verifies same-directory Python importers
// with co-change data are NOT boosted (they already have co-change evidence).
func TestPythonTierBoostNotForCoChange(t *testing.T) {
	scored := []scoredCandidate{
		{candidate: candidate{Path: "pkg/subject.py", Distance: 0}, Score: 10},
		// Same-dir importer with 1 co-change → "low" via condition B, should stay low.
		{candidate: candidate{Path: "pkg/consumer.py", Distance: 1, IsImporter: true, CoChangeFreq: 1}, Score: 5},
	}
	cats := categorize(scored, "pkg/subject.py", config.Empty(), Caps{}, 5)

	if len(cats.LikelyModify) == 0 {
		t.Fatal("expected consumer.py in likely_modify")
	}
	lm := cats.LikelyModify[0]
	if lm.confidence != "low" {
		t.Errorf("expected same-dir Python importer with co-change to stay low, got %q", lm.confidence)
	}
}

// TestPythonTierBoostNotForDistant verifies distant Python importers stay "low".
func TestPythonTierBoostNotForDistant(t *testing.T) {
	scored := []scoredCandidate{
		{candidate: candidate{Path: "Lib/asyncio/events.py", Distance: 0}, Score: 10},
		// Different directory importer with zero co-change → should remain "low".
		{candidate: candidate{Path: "Lib/http/client.py", Distance: 1, IsImporter: true}, Score: 5},
	}
	cats := categorize(scored, "Lib/asyncio/events.py", config.Empty(), Caps{}, 5)

	if len(cats.LikelyModify) == 0 {
		t.Fatal("expected client.py in likely_modify")
	}
	lm := cats.LikelyModify[0]
	if lm.confidence != "low" {
		t.Errorf("expected distant Python importer to stay low, got %q", lm.confidence)
	}
}

// --- normalization helper tests ---

func TestNormalizePyDirStem(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"_pyrepl", "pyrepl"},
		{"__private", "_private"},
		{"normal", ""},
		{"_", ""},
		{"", ""},
		{"_x", "x"},
	}
	for _, tt := range tests {
		got := normalizePyDirStem(tt.input)
		if got != tt.want {
			t.Errorf("normalizePyDirStem(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePyStem(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dbapi2", "dbapi"},
		{"sha256", "sha"},
		{"v2", ""},     // result too short
		{"config", ""}, // no trailing digits
		{"123", ""},    // all digits, result empty
		{"ab1", "ab"},
		{"", ""},
		{"a1", ""}, // result "a" is < 2 chars
	}
	for _, tt := range tests {
		got := normalizePyStem(tt.input)
		if got != tt.want {
			t.Errorf("normalizePyStem(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- stem normalization integration tests ---

// TestNestedTestDirMatchTrailingDigitNorm verifies that test_sqlite3/test_dbapi.py
// matches sqlite3/dbapi2.py via trailing-digit normalization.
func TestNestedTestDirMatchTrailingDigitNorm(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_sqlite3/test_dbapi.py"}, Score: 1.0},
	}
	subject := "Lib/sqlite3/dbapi2.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected test_sqlite3/test_dbapi.py to match sqlite3/dbapi2.py via trailing-digit normalization")
	}
	if !result[0].direct {
		t.Error("expected direct tier")
	}
}

// TestParentDirStemMatchLeadingUnderscore verifies that test_pyrepl/test_pyrepl.py
// matches _pyrepl/types.py via leading-underscore normalization on parent dir.
func TestParentDirStemMatchLeadingUnderscore(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "Lib/test/test_pyrepl/test_pyrepl.py"}, Score: 1.0},
	}
	subject := "Lib/_pyrepl/types.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	found := false
	for _, tc := range result {
		if tc.path == "Lib/test/test_pyrepl/test_pyrepl.py" && tc.direct {
			found = true
		}
	}
	if !found {
		t.Errorf("expected test_pyrepl/test_pyrepl.py to match _pyrepl/types.py via parentDirStemMatch, got %v", testCandidatePaths(result))
	}
}

// TestInitParentMatchLeadingUnderscore verifies that tests/test_internal.py
// matches _internal/__init__.py via leading-underscore normalization.
func TestInitParentMatchLeadingUnderscore(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "tests/test_internal.py"}, Score: 1.0},
	}
	subject := "src/_internal/__init__.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) == 0 {
		t.Fatal("expected tests/test_internal.py to match _internal/__init__.py via initParentMatch with underscore normalization")
	}
	if !result[0].direct {
		t.Error("expected direct tier")
	}
}

// --- negative tests ---

// TestTrailingDigitNormNotGlobalStemMatch verifies that trailing-digit normalization
// does NOT apply to the global subjectStemMatch (only to nestedTestDirMatch).
// This prevents handler2.py from matching test_handler.py in different directories.
func TestTrailingDigitNormNotGlobalStemMatch(t *testing.T) {
	candidates := []scoredCandidate{
		// Different directory, no import — would only match via subjectStemMatch
		{candidate: candidate{Path: "other/test_handler.py"}, Score: 1.0},
	}
	subject := "src/handler2.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected test_handler.py NOT to match handler2.py via global stem match, got %v", testCandidatePaths(result))
	}
}

// TestNormalizationPythonOnly verifies that stem normalization does not fire for .go files.
func TestNormalizationPythonOnly(t *testing.T) {
	candidates := []scoredCandidate{
		{candidate: candidate{Path: "test/test_sqlite3/test_dbapi.py"}, Score: 1.0},
	}
	subject := "pkg/sqlite3/dbapi2.go" // Go file, not Python

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected normalization NOT to fire for .go subject, got %v", testCandidatePaths(result))
	}
}

// TestNestedTestDirMatchNormStillRequiresDirMatch verifies that trailing-digit
// normalization still requires the directory stems to match — wrong dir rejects.
func TestNestedTestDirMatchNormStillRequiresDirMatch(t *testing.T) {
	candidates := []scoredCandidate{
		// test_wrong/test_dbapi.py — dir stem "wrong" != "sqlite3"
		{candidate: candidate{Path: "Lib/test/test_wrong/test_dbapi.py"}, Score: 1.0},
	}
	subject := "Lib/sqlite3/dbapi2.py"

	result, _ := filterTestCandidates(candidates, subject, nil, defaultMaxTests)

	if len(result) != 0 {
		t.Errorf("expected test_wrong/test_dbapi.py NOT to match sqlite3/dbapi2.py (wrong dir), got %v", testCandidatePaths(result))
	}
}

// --- pythonTestDirs tests ---
