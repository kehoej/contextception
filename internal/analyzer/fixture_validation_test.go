package analyzer

import (
	"path/filepath"
	"testing"

	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/indexer"
	"github.com/kehoej/contextception/internal/validation"
)

// buildProjectIndex indexes a real test project and returns an Analyzer.
func buildProjectIndex(t *testing.T, projectDir string) (*Analyzer, *db.Index) {
	t.Helper()
	idx := openTestIndex(t)

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: projectDir,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.Run(); err != nil {
		t.Fatal(err)
	}

	a := New(idx, Config{})
	return a, idx
}

// runFixtureValidation indexes a project, loads its fixtures, runs analysis
// for each fixture subject, and validates against thresholds.
func runFixtureValidation(t *testing.T, projectDir, fixtureDir string) {
	t.Helper()

	a, idx := buildProjectIndex(t, projectDir)
	defer idx.Close()

	fixtures, err := validation.LoadFixturesDir(fixtureDir)
	if err != nil {
		t.Fatalf("loading fixtures from %s: %v", fixtureDir, err)
	}

	if len(fixtures) == 0 {
		t.Fatalf("no fixtures found in %s", fixtureDir)
	}

	th := validation.DefaultThresholds()

	for _, fix := range fixtures {
		t.Run(fix.Subject, func(t *testing.T) {
			output, err := a.Analyze(fix.Subject)
			if err != nil {
				t.Fatalf("Analyze(%q): %v", fix.Subject, err)
			}

			result := validation.Compare(fix, output, th)

			// Always log the result for visibility.
			t.Log(validation.FormatResult(result))

			if !result.Pass {
				t.Errorf("fixture %q failed validation", fix.Subject)

				if result.MustReadRecall < th.MustReadRecall {
					t.Errorf("  must_read recall %.2f < %.2f target; missing: %v",
						result.MustReadRecall, th.MustReadRecall, result.MustReadMissing)
				}
				if result.MustReadPrecision < th.MustReadPrecision {
					t.Errorf("  must_read precision %.2f < %.2f target; extra: %v",
						result.MustReadPrecision, th.MustReadPrecision, result.MustReadExtra)
				}
				if result.RelatedRecall < th.RelatedRecall {
					t.Errorf("  related recall %.2f < %.2f target; missing: %v",
						result.RelatedRecall, th.RelatedRecall, result.RelatedMissing)
				}
				if result.TestsRecall < th.TestsRecall {
					t.Errorf("  tests recall %.2f < %.2f target; missing: %v",
						result.TestsRecall, th.TestsRecall, result.TestsMissing)
				}
				if len(result.ForbiddenViolations) > 0 {
					t.Errorf("  forbidden files in must_read: %v", result.ForbiddenViolations)
				}
				if len(result.IgnoreMissing) > 0 {
					t.Errorf("  files missing from ignore: %v", result.IgnoreMissing)
				}
			}
		})
	}
}

// TestFixtureValidationPythonSmall validates the scoring model against the
// python_small test project. This is a 15-file library with auth, db, and
// API modules — validates basic correctness.
func TestFixtureValidationPythonSmall(t *testing.T) {
	projectDir := filepath.Join("..", "..", "testdata", "python_small")
	fixtureDir := filepath.Join("..", "..", "testdata", "fixtures", "python_small")
	runFixtureValidation(t, projectDir, fixtureDir)
}

// TestFixtureValidationPythonMedium validates the scoring model against the
// python_medium test project. This is a ~40-file Django-like e-commerce app
// with products, orders, accounts modules — validates ranking quality and
// cap behavior at scale.
func TestFixtureValidationPythonMedium(t *testing.T) {
	projectDir := filepath.Join("..", "..", "testdata", "python_medium")
	fixtureDir := filepath.Join("..", "..", "testdata", "fixtures", "python_medium")
	runFixtureValidation(t, projectDir, fixtureDir)
}
