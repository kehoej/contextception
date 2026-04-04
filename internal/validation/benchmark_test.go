package validation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/indexer"
)

// TestPerformanceTargets verifies that index and analyze operations
// meet the wall-time targets from docs/testing-strategy.md.
func TestPerformanceTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	td := testdataDir(t)

	tests := []struct {
		name          string
		repoDir       string
		indexTarget   time.Duration
		analyzeTarget time.Duration
		subjects      []string
	}{
		{
			name:          "python_small",
			repoDir:       filepath.Join(td, "python_small"),
			indexTarget:   5 * time.Second,
			analyzeTarget: 2 * time.Second,
			subjects:      []string{"mylib/api.py", "mylib/models.py", "mylib/config.py"},
		},
		{
			name:          "python_medium",
			repoDir:       filepath.Join(td, "python_medium"),
			indexTarget:   5 * time.Second,
			analyzeTarget: 2 * time.Second,
			subjects:      []string{"shop/orders/services.py", "shop/common/utils.py", "shop/products/views.py"},
		},
		{
			name:          "python_large",
			repoDir:       filepath.Join(td, "python_large"),
			indexTarget:   10 * time.Second,
			analyzeTarget: 2 * time.Second,
			subjects:      []string{"webapp/core/base.py", "webapp/orders/services.py", "webapp/urls.py"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := os.Stat(tt.repoDir); err != nil {
				t.Skipf("repo dir %s not found, skipping", tt.repoDir)
			}

			repoRoot := setupGitRepo(t, tt.repoDir)

			// Measure index time.
			idxPath := filepath.Join(t.TempDir(), "perf.sqlite")
			idx, err := db.OpenIndex(idxPath)
			if err != nil {
				t.Fatal(err)
			}
			defer idx.Close()
			if _, err := idx.MigrateToLatest(); err != nil {
				t.Fatal(err)
			}

			indexStart := time.Now()
			ix, err := indexer.NewIndexer(indexer.Config{
				RepoRoot: repoRoot,
				Index:    idx,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := ix.Run(); err != nil {
				t.Fatal(err)
			}
			indexDur := time.Since(indexStart)

			t.Logf("index time: %v (target: %v)", indexDur, tt.indexTarget)
			if indexDur > tt.indexTarget {
				t.Errorf("index took %v, exceeds target %v", indexDur, tt.indexTarget)
			}

			// Measure analyze time for each subject.
			a := analyzer.New(idx, analyzer.Config{})
			for _, subj := range tt.subjects {
				analyzeStart := time.Now()
				_, err := a.Analyze(subj)
				analyzeDur := time.Since(analyzeStart)

				if err != nil {
					t.Errorf("analyze %s: %v", subj, err)
					continue
				}

				t.Logf("analyze %s: %v (target: %v)", subj, analyzeDur, tt.analyzeTarget)
				if analyzeDur > tt.analyzeTarget {
					t.Errorf("analyze %s took %v, exceeds target %v", subj, analyzeDur, tt.analyzeTarget)
				}
			}
		})
	}
}

// BenchmarkIndex benchmarks the full indexing pipeline.
func BenchmarkIndex(b *testing.B) {
	td := testdataDirB(b)

	repos := []struct {
		name    string
		repoDir string
	}{
		{"python_small", filepath.Join(td, "python_small")},
		{"python_medium", filepath.Join(td, "python_medium")},
		{"python_large", filepath.Join(td, "python_large")},
	}

	for _, repo := range repos {
		b.Run(repo.name, func(b *testing.B) {
			if _, err := os.Stat(repo.repoDir); err != nil {
				b.Skipf("repo dir %s not found, skipping", repo.repoDir)
			}

			repoRoot := setupGitRepoB(b, repo.repoDir)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idxPath := filepath.Join(b.TempDir(), "bench.sqlite")
				idx, err := db.OpenIndex(idxPath)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := idx.MigrateToLatest(); err != nil {
					b.Fatal(err)
				}
				ix, err := indexer.NewIndexer(indexer.Config{
					RepoRoot: repoRoot,
					Index:    idx,
				})
				if err != nil {
					b.Fatal(err)
				}
				if err := ix.Run(); err != nil {
					b.Fatal(err)
				}
				idx.Close()
			}
		})
	}
}

// BenchmarkAnalyze benchmarks the analyze pipeline.
func BenchmarkAnalyze(b *testing.B) {
	td := testdataDirB(b)

	repos := []struct {
		name    string
		repoDir string
		subject string
	}{
		{"python_small", filepath.Join(td, "python_small"), "mylib/api.py"},
		{"python_medium", filepath.Join(td, "python_medium"), "shop/orders/services.py"},
		{"python_large", filepath.Join(td, "python_large"), "webapp/orders/services.py"},
	}

	for _, repo := range repos {
		b.Run(repo.name, func(b *testing.B) {
			if _, err := os.Stat(repo.repoDir); err != nil {
				b.Skipf("repo dir %s not found, skipping", repo.repoDir)
			}

			repoRoot := setupGitRepoB(b, repo.repoDir)

			idxPath := filepath.Join(b.TempDir(), "bench.sqlite")
			idx, err := db.OpenIndex(idxPath)
			if err != nil {
				b.Fatal(err)
			}
			defer idx.Close()
			if _, err := idx.MigrateToLatest(); err != nil {
				b.Fatal(err)
			}
			ix, err := indexer.NewIndexer(indexer.Config{
				RepoRoot: repoRoot,
				Index:    idx,
			})
			if err != nil {
				b.Fatal(err)
			}
			if err := ix.Run(); err != nil {
				b.Fatal(err)
			}

			a := analyzer.New(idx, analyzer.Config{})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := a.Analyze(repo.subject); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// testdataDirB and setupGitRepoB are now unified via testing.TB in validation_test.go.
// These aliases maintain compatibility with existing benchmark code.
var testdataDirB = testdataDir
var setupGitRepoB = setupGitRepo
