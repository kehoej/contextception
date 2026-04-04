package validation

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/indexer"
)

// testdataDir returns the absolute path to the project's testdata/ directory.
// Accepts testing.TB so it works in both tests and benchmarks.
func testdataDir(tb testing.TB) string {
	tb.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("cannot determine test file location")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	dir := filepath.Join(repoRoot, "testdata")
	if _, err := os.Stat(dir); err != nil {
		tb.Fatalf("testdata dir not found at %s: %v", dir, err)
	}
	return dir
}

// setupGitRepo copies a testdata repo to a temp dir and initializes git.
// Accepts testing.TB so it works in both tests and benchmarks.
func setupGitRepo(tb testing.TB, srcDir string) string {
	tb.Helper()
	dst := tb.TempDir()
	if err := copyDir(srcDir, dst); err != nil {
		tb.Fatalf("copying %s to temp dir: %v", srcDir, err)
	}

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init", "--allow-empty"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dst
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2024-01-01T00:00:00+00:00", "GIT_COMMITTER_DATE=2024-01-01T00:00:00+00:00")
		if out, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
	return dst
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// indexAndAnalyze indexes the given repo root and returns an analyzer.
func indexAndAnalyze(t *testing.T, repoRoot string) *analyzer.Analyzer {
	t.Helper()
	return indexAndAnalyzeWithConfig(t, repoRoot, analyzer.Config{})
}

// indexAndAnalyzeWithConfig indexes the given repo root with a custom analyzer config.
func indexAndAnalyzeWithConfig(t *testing.T, repoRoot string, cfg analyzer.Config) *analyzer.Analyzer {
	t.Helper()

	// Open temp index database.
	idxPath := filepath.Join(t.TempDir(), "test.sqlite")
	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		t.Fatalf("opening index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })

	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	// Run indexer.
	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: repoRoot,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatalf("creating indexer: %v", err)
	}
	if err := ix.Run(); err != nil {
		t.Fatalf("indexing: %v", err)
	}

	return analyzer.New(idx, cfg)
}

func TestValidateFixtures(t *testing.T) {
	td := testdataDir(t)
	th := DefaultThresholds()

	analyzerCfg := analyzer.Config{}

	repos := []struct {
		name       string
		repoDir    string
		fixtureDir string
	}{
		{"python_small", filepath.Join(td, "python_small"), filepath.Join(td, "fixtures", "python_small")},
		{"python_medium", filepath.Join(td, "python_medium"), filepath.Join(td, "fixtures", "python_medium")},
		{"python_large", filepath.Join(td, "python_large"), filepath.Join(td, "fixtures", "python_large")},
		{"ts_small", filepath.Join(td, "ts_small"), filepath.Join(td, "fixtures", "ts_small")},
		{"ts_monorepo", filepath.Join(td, "ts_monorepo"), filepath.Join(td, "fixtures", "ts_monorepo")},
	}

	for _, repo := range repos {
		t.Run(repo.name, func(t *testing.T) {
			// Skip if repo or fixtures don't exist yet.
			if _, err := os.Stat(repo.repoDir); err != nil {
				t.Skipf("repo dir %s not found, skipping", repo.repoDir)
			}
			if _, err := os.Stat(repo.fixtureDir); err != nil {
				t.Skipf("fixture dir %s not found, skipping", repo.fixtureDir)
			}

			// Setup git repo in temp dir.
			repoRoot := setupGitRepo(t, repo.repoDir)
			a := indexAndAnalyzeWithConfig(t, repoRoot, analyzerCfg)

			// Load fixtures.
			fixtures, err := LoadFixturesDir(repo.fixtureDir)
			if err != nil {
				t.Fatalf("loading fixtures: %v", err)
			}
			if len(fixtures) == 0 {
				t.Fatal("no fixtures found")
			}

			for _, fix := range fixtures {
				t.Run(fix.Subject, func(t *testing.T) {
					output, err := a.Analyze(fix.Subject)
					if err != nil {
						t.Fatalf("analyzing %s: %v", fix.Subject, err)
					}

					result := Compare(fix, output, th)
					t.Log(FormatResult(result))

					if !result.Pass {
						t.Errorf("fixture validation failed for %s", fix.Subject)
					}
				})
			}
		})
	}
}

// TestValidateFixturesDiagnostic runs analysis on each fixture and dumps
// detailed output for debugging fixture expectations. Run with -v to see output.
func TestValidateFixturesDiagnostic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping diagnostic test in short mode")
	}

	td := testdataDir(t)

	repos := []struct {
		name    string
		repoDir string
	}{
		{"python_small", filepath.Join(td, "python_small")},
		{"python_medium", filepath.Join(td, "python_medium")},
		{"python_large", filepath.Join(td, "python_large")},
		{"ts_small", filepath.Join(td, "ts_small")},
		{"ts_monorepo", filepath.Join(td, "ts_monorepo")},
	}

	for _, repo := range repos {
		t.Run(repo.name, func(t *testing.T) {
			if _, err := os.Stat(repo.repoDir); err != nil {
				t.Skipf("repo dir %s not found, skipping", repo.repoDir)
			}

			repoRoot := setupGitRepo(t, repo.repoDir)
			a := indexAndAnalyze(t, repoRoot)

			// Analyze a few key files and dump output.
			subjects := []string{}
			fixtureDir := filepath.Join(td, "fixtures", repo.name)
			fixtures, err := LoadFixturesDir(fixtureDir)
			if err == nil {
				for _, f := range fixtures {
					subjects = append(subjects, f.Subject)
				}
			}

			for _, subj := range subjects {
				output, err := a.Analyze(subj)
				if err != nil {
					t.Logf("ERROR analyzing %s: %v", subj, err)
					continue
				}
				t.Logf("=== %s ===", subj)
				mustReadPaths := extractMustReadPaths(output.MustRead)
				t.Logf("  must_read: %v", mustReadPaths)
				t.Logf("  related:   %v", flattenRelatedEntries(output.Related))
				t.Logf("  tests:     %v", output.Tests)
				t.Logf("  external:  %v", output.External)
			}
		})
	}
}
