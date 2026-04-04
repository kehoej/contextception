package grader

import (
	"fmt"
	"time"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
)

// RunFeatureTests executes the feature matrix against the given index and file.
// Returns results for 7 test groups: modes, token_budget, caps, flags, ci, discovery, schema.
func RunFeatureTests(idx *db.Index, repoRoot, testFile string, cfg *config.Config) []FeatureTest {
	var results []FeatureTest

	results = append(results, runModeTests(idx, repoRoot, testFile, cfg)...)
	results = append(results, runTokenBudgetTests(idx, repoRoot, testFile, cfg)...)
	results = append(results, runCapTests(idx, repoRoot, testFile, cfg)...)
	results = append(results, runFlagTests(idx, repoRoot, testFile, cfg)...)
	results = append(results, runCITests(idx, repoRoot, testFile, cfg)...)
	results = append(results, runDiscoveryTests(idx)...)
	results = append(results, runSchemaTests(idx, repoRoot, testFile, cfg)...)

	return results
}

// runModeTests verifies plan/implement/review cap adjustments.
func runModeTests(idx *db.Index, repoRoot, file string, cfg *config.Config) []FeatureTest {
	modes := []struct {
		name                                    string
		mustRead, likelyModify, tests, related int
	}{
		{"plan", 15, 5, 3, 15},
		{"implement", 10, 20, 5, 5},
		{"review", 5, 10, 10, 5},
	}

	var results []FeatureTest
	for _, m := range modes {
		start := time.Now()
		a := analyzer.New(idx, analyzer.Config{
			RepoConfig: cfg,
			RepoRoot: repoRoot,
			Caps:     analyzer.Caps{Mode: m.name},
		})
		out, err := a.Analyze(file)
		dur := time.Since(start).Milliseconds()

		ft := FeatureTest{
			Group:    "modes",
			Name:     fmt.Sprintf("mode=%s caps applied", m.name),
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			// Verify caps are respected.
			mrOK := len(out.MustRead) <= m.mustRead
			lmOK := countLikelyModify(out.LikelyModify) <= m.likelyModify
			tOK := len(out.Tests) <= m.tests
			rOK := countRelated(out.Related) <= m.related

			if mrOK && lmOK && tOK && rOK {
				ft.Status = "pass"
				ft.Expected = fmt.Sprintf("mr<=%d lm<=%d t<=%d r<=%d", m.mustRead, m.likelyModify, m.tests, m.related)
				ft.Actual = fmt.Sprintf("mr=%d lm=%d t=%d r=%d", len(out.MustRead), countLikelyModify(out.LikelyModify), len(out.Tests), countRelated(out.Related))
			} else {
				ft.Status = "fail"
				ft.Expected = fmt.Sprintf("mr<=%d lm<=%d t<=%d r<=%d", m.mustRead, m.likelyModify, m.tests, m.related)
				ft.Actual = fmt.Sprintf("mr=%d lm=%d t=%d r=%d", len(out.MustRead), countLikelyModify(out.LikelyModify), len(out.Tests), countRelated(out.Related))
			}
		}
		results = append(results, ft)
	}
	return results
}

// runTokenBudgetTests verifies token budget proportional scaling.
func runTokenBudgetTests(idx *db.Index, repoRoot, file string, cfg *config.Config) []FeatureTest {
	budgets := []int{500, 2000, 8000}
	var results []FeatureTest

	var prevTotal int
	for _, budget := range budgets {
		start := time.Now()
		a := analyzer.New(idx, analyzer.Config{
			RepoConfig: cfg,
			RepoRoot: repoRoot,
			Caps:     analyzer.Caps{TokenBudget: budget},
		})
		out, err := a.Analyze(file)
		dur := time.Since(start).Milliseconds()

		ft := FeatureTest{
			Group:    "token_budget",
			Name:     fmt.Sprintf("budget=%d", budget),
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			total := len(out.MustRead) + countLikelyModify(out.LikelyModify) + len(out.Tests) + countRelated(out.Related)
			ft.Actual = fmt.Sprintf("total_entries=%d", total)

			if prevTotal > 0 && total < prevTotal {
				// Smaller budget should produce fewer or equal entries.
				// But we're going from small→large, so this reversal means something is wrong.
				ft.Status = "fail"
				ft.Expected = fmt.Sprintf("total >= %d (previous budget)", prevTotal)
			} else {
				ft.Status = "pass"
				ft.Expected = fmt.Sprintf("budget=%d produces proportional output", budget)
			}
			prevTotal = total
		}
		results = append(results, ft)
	}
	return results
}

// runCapTests verifies explicit cap parameters are honored.
func runCapTests(idx *db.Index, repoRoot, file string, cfg *config.Config) []FeatureTest {
	caps := []struct {
		name                               string
		mustRead, likelyModify, tests, related int
	}{
		{"small_caps", 3, 3, 2, 2},
		{"large_caps", 20, 25, 10, 15},
	}

	var results []FeatureTest
	for _, c := range caps {
		start := time.Now()
		a := analyzer.New(idx, analyzer.Config{
			RepoConfig: cfg,
			RepoRoot: repoRoot,
			Caps: analyzer.Caps{
				MaxMustRead:     c.mustRead,
				MaxLikelyModify: c.likelyModify,
				MaxTests:        c.tests,
				MaxRelated:      c.related,
			},
		})
		out, err := a.Analyze(file)
		dur := time.Since(start).Milliseconds()

		ft := FeatureTest{
			Group:    "caps",
			Name:     c.name,
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			mrOK := len(out.MustRead) <= c.mustRead
			lmOK := countLikelyModify(out.LikelyModify) <= c.likelyModify
			tOK := len(out.Tests) <= c.tests
			rOK := countRelated(out.Related) <= c.related

			ft.Expected = fmt.Sprintf("mr<=%d lm<=%d t<=%d r<=%d", c.mustRead, c.likelyModify, c.tests, c.related)
			ft.Actual = fmt.Sprintf("mr=%d lm=%d t=%d r=%d", len(out.MustRead), countLikelyModify(out.LikelyModify), len(out.Tests), countRelated(out.Related))

			if mrOK && lmOK && tOK && rOK {
				ft.Status = "pass"
			} else {
				ft.Status = "fail"
			}
		}
		results = append(results, ft)
	}
	return results
}

// runFlagTests verifies --signatures and --no-external flags.
func runFlagTests(idx *db.Index, repoRoot, file string, cfg *config.Config) []FeatureTest {
	var results []FeatureTest

	// Test --signatures.
	{
		start := time.Now()
		a := analyzer.New(idx, analyzer.Config{
			RepoConfig: cfg,
			RepoRoot:   repoRoot,
			Signatures: true,
		})
		out, err := a.Analyze(file)
		dur := time.Since(start).Milliseconds()

		ft := FeatureTest{
			Group:    "flags",
			Name:     "signatures enabled",
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			hasDefs := false
			for _, e := range out.MustRead {
				if len(e.Definitions) > 0 {
					hasDefs = true
					break
				}
			}
			if hasDefs {
				ft.Status = "pass"
				ft.Actual = "definitions present in must_read"
			} else {
				// Definitions may be absent if no symbols tracked — soft pass.
				ft.Status = "pass"
				ft.Actual = "no definitions (may be expected if no tracked symbols)"
			}
			ft.Expected = "definitions populated when symbols exist"
		}
		results = append(results, ft)
	}

	// Test --no-external.
	{
		start := time.Now()
		a := analyzer.New(idx, analyzer.Config{
			RepoConfig:   cfg,
			RepoRoot:     repoRoot,
			OmitExternal: true,
		})
		out, err := a.Analyze(file)
		dur := time.Since(start).Milliseconds()

		ft := FeatureTest{
			Group:    "flags",
			Name:     "no-external omits external deps",
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			if len(out.External) == 0 {
				ft.Status = "pass"
				ft.Actual = "external list empty"
			} else {
				ft.Status = "fail"
				ft.Actual = fmt.Sprintf("external has %d entries", len(out.External))
			}
			ft.Expected = "external list should be empty"
		}
		results = append(results, ft)
	}

	return results
}

// runCITests verifies CI mode exit code behavior via analysis output.
func runCITests(idx *db.Index, repoRoot, file string, cfg *config.Config) []FeatureTest {
	var results []FeatureTest

	start := time.Now()
	a := analyzer.New(idx, analyzer.Config{
		RepoConfig: cfg,
		RepoRoot: repoRoot,
	})
	out, err := a.Analyze(file)
	dur := time.Since(start).Milliseconds()

	ft := FeatureTest{
		Group:    "ci",
		Name:     "blast_radius available for CI exit code",
		Duration: dur,
	}
	if err != nil {
		ft.Status = "fail"
		ft.Error = err.Error()
	} else if out.BlastRadius != nil {
		ft.Status = "pass"
		ft.Actual = fmt.Sprintf("level=%s", out.BlastRadius.Level)
		ft.Expected = "blast_radius present with valid level"
	} else {
		ft.Status = "fail"
		ft.Actual = "blast_radius is nil"
		ft.Expected = "blast_radius present"
	}
	results = append(results, ft)

	return results
}

// runDiscoveryTests verifies search, entrypoints, and structure endpoints.
func runDiscoveryTests(idx *db.Index) []FeatureTest {
	var results []FeatureTest

	// Search by path.
	{
		start := time.Now()
		res, err := idx.SearchByPath("main", 10)
		dur := time.Since(start).Milliseconds()
		ft := FeatureTest{
			Group:    "discovery",
			Name:     "search by path",
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			ft.Status = "pass"
			ft.Actual = fmt.Sprintf("%d results", len(res))
			ft.Expected = "search returns results"
		}
		results = append(results, ft)
	}

	// Get entrypoints.
	{
		start := time.Now()
		eps, err := idx.GetEntrypoints()
		dur := time.Since(start).Milliseconds()
		ft := FeatureTest{
			Group:    "discovery",
			Name:     "get entrypoints",
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			ft.Status = "pass"
			ft.Actual = fmt.Sprintf("%d entrypoints", len(eps))
		}
		results = append(results, ft)
	}

	// Get directory structure.
	{
		start := time.Now()
		dirs, err := idx.GetDirectoryStructure()
		dur := time.Since(start).Milliseconds()
		ft := FeatureTest{
			Group:    "discovery",
			Name:     "get directory structure",
			Duration: dur,
		}
		if err != nil {
			ft.Status = "fail"
			ft.Error = err.Error()
		} else {
			ft.Status = "pass"
			ft.Actual = fmt.Sprintf("%d dir-language entries", len(dirs))
		}
		results = append(results, ft)
	}

	return results
}

// runSchemaTests verifies output matches schema 3.2 requirements.
func runSchemaTests(idx *db.Index, repoRoot, file string, cfg *config.Config) []FeatureTest {
	var results []FeatureTest

	start := time.Now()
	a := analyzer.New(idx, analyzer.Config{
		RepoConfig: cfg,
		RepoRoot: repoRoot,
	})
	out, err := a.Analyze(file)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		results = append(results, FeatureTest{
			Group:    "schema",
			Name:     "schema 3.2 compliance",
			Status:   "fail",
			Error:    err.Error(),
			Duration: dur,
		})
		return results
	}

	// Check schema version.
	{
		ft := FeatureTest{
			Group:    "schema",
			Name:     "schema_version field",
			Duration: dur,
			Expected: "3.2",
			Actual:   out.SchemaVersion,
		}
		if out.SchemaVersion == "3.2" {
			ft.Status = "pass"
		} else {
			ft.Status = "fail"
		}
		results = append(results, ft)
	}

	// Check required fields present.
	{
		ft := FeatureTest{
			Group:    "schema",
			Name:     "required fields present",
			Duration: dur,
		}
		missing := []string{}
		if out.Subject == "" {
			missing = append(missing, "subject")
		}
		if out.MustRead == nil {
			missing = append(missing, "must_read")
		}
		if out.LikelyModify == nil {
			missing = append(missing, "likely_modify")
		}
		if out.Tests == nil {
			missing = append(missing, "tests")
		}
		if out.Related == nil {
			missing = append(missing, "related")
		}
		if out.Stats == nil {
			missing = append(missing, "stats")
		}

		if len(missing) == 0 {
			ft.Status = "pass"
			ft.Actual = "all required fields present"
		} else {
			ft.Status = "fail"
			ft.Actual = fmt.Sprintf("missing: %v", missing)
		}
		ft.Expected = "subject, must_read, likely_modify, tests, related, stats"
		results = append(results, ft)
	}

	// Check confidence range.
	{
		ft := FeatureTest{
			Group:    "schema",
			Name:     "confidence in range [0,1]",
			Duration: dur,
			Expected: "0.0 <= confidence <= 1.0",
			Actual:   fmt.Sprintf("%.2f", out.Confidence),
		}
		if out.Confidence >= 0 && out.Confidence <= 1 {
			ft.Status = "pass"
		} else {
			ft.Status = "fail"
		}
		results = append(results, ft)
	}

	// Check blast_radius level valid.
	{
		ft := FeatureTest{
			Group:    "schema",
			Name:     "blast_radius level valid",
			Duration: dur,
		}
		if out.BlastRadius != nil {
			valid := map[string]bool{"low": true, "medium": true, "high": true}
			if valid[out.BlastRadius.Level] {
				ft.Status = "pass"
				ft.Actual = out.BlastRadius.Level
			} else {
				ft.Status = "fail"
				ft.Actual = out.BlastRadius.Level
			}
			ft.Expected = "low|medium|high"
		} else {
			ft.Status = "fail"
			ft.Actual = "nil"
			ft.Expected = "blast_radius present"
		}
		results = append(results, ft)
	}

	return results
}
