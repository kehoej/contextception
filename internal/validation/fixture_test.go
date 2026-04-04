package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

// helper to create a v3 AnalysisOutput with flat must_read paths.
func makeOutput(mustRead, related []string) *model.AnalysisOutput {
	mr := make([]model.MustReadEntry, len(mustRead))
	for i, p := range mustRead {
		mr[i] = model.MustReadEntry{File: p}
	}
	rel := map[string][]model.RelatedEntry{}
	if len(related) > 0 {
		entries := make([]model.RelatedEntry, len(related))
		for i, r := range related {
			entries[i] = model.RelatedEntry{File: r}
		}
		rel["."] = entries
	}
	return &model.AnalysisOutput{
		MustRead:     mr,
		LikelyModify: map[string][]model.LikelyModifyEntry{},
		Related:      rel,
		Tests:        []model.TestEntry{},
		External:     []string{},
	}
}

func TestComparePass(t *testing.T) {
	fix := &Fixture{
		Subject:           "a.py",
		MustRead:          []string{"a.py", "b.py", "c.py"},
		MustReadForbidden: []string{"test_a.py"},
		Related:           []string{"d.py"},
		Tests:             []string{},
		IgnoreMustContain: []string{"test_a.py"},
	}
	actual := makeOutput(
		[]string{"a.py", "b.py", "c.py"},
		[]string{"d.py", "e.py"},
	)
	// test_a.py is NOT in any positive section → ignore check passes
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass, got:\n%s", FormatResult(r))
	}
}

func TestCompareFailMustReadRecall(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.py",
		MustRead: []string{"a.py", "b.py", "c.py", "d.py"},
	}
	actual := makeOutput([]string{"a.py"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to low must_read recall")
	}
	if !floatClose(r.MustReadRecall, 0.25) {
		t.Errorf("MustReadRecall = %v, want 0.25", r.MustReadRecall)
	}
}

func TestCompareFailMustReadPrecision(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.py",
		MustRead: []string{"a.py"},
	}
	actual := makeOutput([]string{"a.py", "x.py", "y.py", "z.py", "w.py"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to low must_read precision")
	}
	if !floatClose(r.MustReadPrecision, 0.20) {
		t.Errorf("MustReadPrecision = %v, want 0.20", r.MustReadPrecision)
	}
}

func TestCompareFailRelatedRecall(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.py",
		MustRead: []string{"a.py"},
		Related:  []string{"b.py", "c.py", "d.py", "e.py", "f.py"},
	}
	actual := makeOutput([]string{"a.py"}, []string{"b.py"})
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to low related recall")
	}
	if !floatClose(r.RelatedRecall, 0.20) {
		t.Errorf("RelatedRecall = %v, want 0.20", r.RelatedRecall)
	}
}

func TestCompareFailForbiddenViolation(t *testing.T) {
	fix := &Fixture{
		Subject:           "a.py",
		MustRead:          []string{"a.py", "b.py"},
		MustReadForbidden: []string{"test_a.py"},
	}
	actual := makeOutput([]string{"a.py", "b.py", "test_a.py"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to forbidden violation")
	}
	if len(r.ForbiddenViolations) != 1 || r.ForbiddenViolations[0] != "test_a.py" {
		t.Errorf("ForbiddenViolations = %v, want [test_a.py]", r.ForbiddenViolations)
	}
}

func TestCompareFailIgnoreMissing(t *testing.T) {
	fix := &Fixture{
		Subject:           "a.py",
		MustRead:          []string{"a.py"},
		IgnoreMustContain: []string{"test_a.py", "docs.py"},
	}
	// test_a.py and docs.py are not in any section → passes.
	actual := makeOutput([]string{"a.py"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass since ignored files are not in positive sections, got:\n%s", FormatResult(r))
	}

	// But if docs.py appears in must_read, it should fail.
	actual2 := makeOutput([]string{"a.py", "docs.py"}, nil)
	r2 := Compare(fix, actual2, DefaultThresholds())
	if r2.Pass {
		t.Error("expected fail because docs.py is in must_read but should be ignored")
	}
	if len(r2.IgnoreMissing) != 1 || r2.IgnoreMissing[0] != "docs.py" {
		t.Errorf("IgnoreMissing = %v, want [docs.py]", r2.IgnoreMissing)
	}
}

func TestCompareEdgeThreshold(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.py",
		MustRead: []string{"a.py", "b.py", "c.py", "d.py", "e.py", "f.py", "g.py", "h.py", "i.py", "j.py",
			"k.py", "l.py", "m.py", "n.py", "o.py", "p.py", "q.py", "r.py", "s.py", "t.py"},
	}
	// 19 of 20 = 0.95 recall.
	actual := makeOutput(
		[]string{"a.py", "b.py", "c.py", "d.py", "e.py", "f.py", "g.py", "h.py", "i.py", "j.py",
			"k.py", "l.py", "m.py", "n.py", "o.py", "p.py", "q.py", "r.py", "s.py"},
		nil,
	)
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass at exactly 0.95 recall, got:\n%s", FormatResult(r))
	}
}

func TestCompareFailRelatedForbidden(t *testing.T) {
	fix := &Fixture{
		Subject:          "a.py",
		MustRead:         []string{"a.py", "b.py"},
		Related:          []string{"c.py"},
		RelatedForbidden: []string{"test_a.py"},
	}
	// test_a.py is in related → should fail.
	actual := makeOutput([]string{"a.py", "b.py"}, []string{"c.py", "test_a.py"})
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to related_forbidden violation")
	}
	if len(r.RelatedForbiddenViolations) != 1 || r.RelatedForbiddenViolations[0] != "test_a.py" {
		t.Errorf("RelatedForbiddenViolations = %v, want [test_a.py]", r.RelatedForbiddenViolations)
	}
}

func TestComparePassRelatedForbiddenClean(t *testing.T) {
	fix := &Fixture{
		Subject:          "a.py",
		MustRead:         []string{"a.py", "b.py"},
		Related:          []string{"c.py"},
		RelatedForbidden: []string{"test_a.py"},
	}
	// test_a.py is NOT in related → should pass.
	actual := makeOutput([]string{"a.py", "b.py"}, []string{"c.py", "d.py"})
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass, got:\n%s", FormatResult(r))
	}
}

func TestCompareFailOrderingViolation(t *testing.T) {
	fix := &Fixture{
		Subject:         "a.py",
		MustRead:        []string{"b.py", "c.py", "d.py"},
		MustReadOrdered: []string{"b.py", "c.py", "d.py"}, // b before c before d
	}
	// Actual order has d before b → violates b before d.
	actual := makeOutput([]string{"d.py", "c.py", "b.py"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to ordering violation")
	}
	if len(r.OrderingViolations) == 0 {
		t.Error("expected ordering violations")
	}
}

func TestComparePassOrderingCorrect(t *testing.T) {
	fix := &Fixture{
		Subject:         "a.py",
		MustRead:        []string{"b.py", "c.py", "d.py"},
		MustReadOrdered: []string{"b.py", "d.py"}, // b before d (partial ordering)
	}
	// Actual order: b, c, d → b at 0, d at 2, correct.
	actual := makeOutput([]string{"b.py", "c.py", "d.py"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass, got:\n%s", FormatResult(r))
	}
}

func TestCompareOrderingSkipsMissingFiles(t *testing.T) {
	fix := &Fixture{
		Subject:         "a.py",
		MustRead:        []string{"b.py", "c.py"},
		MustReadOrdered: []string{"b.py", "x.py", "c.py"}, // x.py not in actual
	}
	// x.py missing from actual → skip that pair, only check b before c.
	actual := makeOutput([]string{"b.py", "c.py"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass (missing files skipped in ordering), got:\n%s", FormatResult(r))
	}
}

func TestCompareTestsRecall(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.py",
		MustRead: []string{"a.py"},
		Tests:    []string{"test_a.py"},
	}
	actual := makeOutput([]string{"a.py"}, nil)
	actual.Tests = []model.TestEntry{{File: "test_a.py", Direct: true}, {File: "test_b.py", Direct: true}}
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass: all expected tests found, got:\n%s", FormatResult(r))
	}
	if !floatClose(r.TestsRecall, 1.0) {
		t.Errorf("TestsRecall = %v, want 1.0", r.TestsRecall)
	}
}

func TestCompareFailTestsRecall(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.py",
		MustRead: []string{"a.py"},
		Tests:    []string{"test_a.py", "test_b.py", "test_c.py", "test_d.py", "test_e.py"},
	}
	actual := makeOutput([]string{"a.py"}, nil)
	actual.Tests = []model.TestEntry{{File: "test_a.py", Direct: true}} // 1 of 5 = 0.20 recall
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to low tests recall")
	}
	if !floatClose(r.TestsRecall, 0.20) {
		t.Errorf("TestsRecall = %v, want 0.20", r.TestsRecall)
	}
}

func TestLoadFixtureNewFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	content := `{
		"subject": "foo/bar.py",
		"must_read": ["foo/bar.py", "foo/baz.py"],
		"must_read_forbidden": ["tests/test_bar.py"],
		"must_read_ordered": ["foo/bar.py", "foo/baz.py"],
		"related": ["foo/qux.py"],
		"related_forbidden": ["tests/test_bar.py"],
		"tests": ["tests/test_bar.py"],
		"ignore_must_contain": ["tests/test_bar.py"]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := LoadFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.MustReadOrdered) != 2 {
		t.Errorf("MustReadOrdered len = %d, want 2", len(f.MustReadOrdered))
	}
	if len(f.RelatedForbidden) != 1 {
		t.Errorf("RelatedForbidden len = %d, want 1", len(f.RelatedForbidden))
	}
	if len(f.Tests) != 1 {
		t.Errorf("Tests len = %d, want 1", len(f.Tests))
	}
}

func TestLoadFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	content := `{
		"subject": "foo/bar.py",
		"must_read": ["foo/bar.py", "foo/baz.py"],
		"must_read_forbidden": ["tests/test_bar.py"],
		"related": ["foo/qux.py"],
		"tests": [],
		"ignore_must_contain": ["tests/test_bar.py"]
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := LoadFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Subject != "foo/bar.py" {
		t.Errorf("Subject = %q, want foo/bar.py", f.Subject)
	}
	if len(f.MustRead) != 2 {
		t.Errorf("MustRead len = %d, want 2", len(f.MustRead))
	}
	if len(f.MustReadForbidden) != 1 {
		t.Errorf("MustReadForbidden len = %d, want 1", len(f.MustReadForbidden))
	}
}

func TestLoadFixtureErrors(t *testing.T) {
	_, err := LoadFixture("/nonexistent/path.json")
	if err == nil {
		t.Error("expected error for missing file")
	}

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadFixture(bad)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadFixturesDir(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"a.json", "b.json"} {
		content := `{"subject": "` + name + `", "must_read": []}`
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("skip me"), 0o644); err != nil {
		t.Fatal(err)
	}

	fixtures, err := LoadFixturesDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 2 {
		t.Errorf("got %d fixtures, want 2", len(fixtures))
	}
}

func TestCompareExternalRecall(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.ts",
		MustRead: []string{"b.ts"},
		External: []string{"zod", "pg"},
	}
	actual := makeOutput([]string{"b.ts"}, nil)
	actual.External = []string{"zod", "pg", "express"}
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass (all expected externals found), got:\n%s", FormatResult(r))
	}
	if !floatClose(r.ExternalRecall, 1.0) {
		t.Errorf("ExternalRecall = %v, want 1.0", r.ExternalRecall)
	}
}

func TestCompareFailExternalRecall(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.ts",
		MustRead: []string{"b.ts"},
		External: []string{"zod", "pg"},
	}
	actual := makeOutput([]string{"b.ts"}, nil)
	actual.External = []string{"zod"} // missing "pg"
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail due to low external recall")
	}
	if !floatClose(r.ExternalRecall, 0.5) {
		t.Errorf("ExternalRecall = %v, want 0.5", r.ExternalRecall)
	}
	if len(r.ExternalMissing) != 1 || r.ExternalMissing[0] != "pg" {
		t.Errorf("ExternalMissing = %v, want [pg]", r.ExternalMissing)
	}
}

func TestCompareExternalEmpty(t *testing.T) {
	fix := &Fixture{
		Subject:  "a.ts",
		MustRead: []string{"b.ts"},
		// no External field → should pass by default
	}
	actual := makeOutput([]string{"b.ts"}, nil)
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass with no external expectations, got:\n%s", FormatResult(r))
	}
	if !floatClose(r.ExternalRecall, 1.0) {
		t.Errorf("ExternalRecall = %v, want 1.0 (no expectations)", r.ExternalRecall)
	}
}

func TestCompareExpectStablePass(t *testing.T) {
	fix := &Fixture{
		Subject:      "a.ts",
		MustRead:     []string{"b.ts", "c.ts"},
		ExpectStable: []string{"b.ts"},
	}
	mr := []model.MustReadEntry{
		{File: "b.ts", Stable: true},
		{File: "c.ts", Stable: false},
	}
	actual := &model.AnalysisOutput{
		MustRead:     mr,
		LikelyModify: map[string][]model.LikelyModifyEntry{},
		Related:      map[string][]model.RelatedEntry{},
		Tests:        []model.TestEntry{},
		External:     []string{},
	}
	r := Compare(fix, actual, DefaultThresholds())
	if !r.Pass {
		t.Errorf("expected pass (b.ts is stable), got:\n%s", FormatResult(r))
	}
}

func TestCompareExpectStableFail(t *testing.T) {
	fix := &Fixture{
		Subject:      "a.ts",
		MustRead:     []string{"b.ts", "c.ts"},
		ExpectStable: []string{"b.ts"},
	}
	mr := []model.MustReadEntry{
		{File: "b.ts", Stable: false},
		{File: "c.ts", Stable: false},
	}
	actual := &model.AnalysisOutput{
		MustRead:     mr,
		LikelyModify: map[string][]model.LikelyModifyEntry{},
		Related:      map[string][]model.RelatedEntry{},
		Tests:        []model.TestEntry{},
		External:     []string{},
	}
	r := Compare(fix, actual, DefaultThresholds())
	if r.Pass {
		t.Error("expected fail because b.ts is not stable")
	}
	if len(r.StableMissing) != 1 || r.StableMissing[0] != "b.ts" {
		t.Errorf("StableMissing = %v, want [b.ts]", r.StableMissing)
	}
}

func TestLoadFixturesDirError(t *testing.T) {
	_, err := LoadFixturesDir("/nonexistent/dir")
	if err == nil {
		t.Error("expected error for missing directory")
	}
}
