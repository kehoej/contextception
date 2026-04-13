package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/change"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/history"
	"github.com/kehoej/contextception/internal/indexer"
	"github.com/kehoej/contextception/internal/model"
	"github.com/spf13/cobra"
)

func newAnalyzeChangeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze-change [base..head]",
		Short: "Analyze the impact of a PR or commit range",
		Long: `Runs git diff to find changed files, analyzes each one, and produces
a unified PR impact report with combined blast radius, coupling signals,
test coverage gaps, and aggregated hotspots.

If no ref range is given, defaults to the diff against the merge base of
HEAD and the default branch (main or master).

Examples:
  contextception analyze-change                  # changes vs default branch
  contextception analyze-change main..HEAD       # explicit range
  contextception analyze-change abc123..def456   # commit range
  contextception analyze-change HEAD~3..HEAD     # last 3 commits`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			refRange := ""
			if len(args) == 1 {
				refRange = args[0]
			}
			return runAnalyzeChange(refRange)
		},
	}

	registerAnalysisFlags(cmd)
	return cmd
}

func runAnalyzeChange(refRange string) error {
	// Open the index.
	idxPath := db.IndexPath(repoRoot)
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		return fmt.Errorf("no index found — run `contextception index` first")
	}

	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer idx.Close()

	// Auto-reindex to ensure the index reflects current files on disk.
	// This is especially important for pre-PR workflows where new files
	// (tests, source) exist on disk but aren't committed yet.
	if err := autoReindex(idx); err != nil {
		// Non-fatal: proceed with stale index rather than failing.
		fmt.Fprintf(os.Stderr, "Warning: auto-reindex failed: %v\n", err)
	}

	// Determine the ref range.
	if refRange == "" {
		base, err := detectDefaultBranch()
		if err != nil {
			return fmt.Errorf("detecting default branch: %w", err)
		}
		// Use merge-base for a cleaner diff.
		mergeBase, err := gitMergeBase(base, "HEAD")
		if err != nil {
			// Fall back to direct diff if merge-base fails (e.g., no common ancestor).
			refRange = base + "..HEAD"
		} else {
			refRange = mergeBase + "..HEAD"
		}
	}

	// Parse the ref range.
	parts := strings.SplitN(refRange, "..", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid ref range %q — expected format: base..head", refRange)
	}
	base, head := parts[0], parts[1]

	start := time.Now()

	// Build the change report using the shared package.
	cfg := change.Config{
		RepoRoot: repoRoot,
		RepoCfg:  repoCfg,
		AnalyzerCfg: analyzer.Config{
			OmitExternal: noExternal,
			Signatures:   signatures,
			Caps: analyzer.Caps{
				MaxMustRead:     maxMustRead,
				MaxRelated:      maxRelated,
				MaxLikelyModify: maxLikelyModify,
				MaxTests:        maxTests,
				Mode:            mode,
				TokenBudget:     tokenBudget,
			},
		},
	}

	report, err := change.BuildReport(idx, cfg, base, head)
	if err != nil {
		return err
	}

	if len(report.ChangedFiles) == 0 {
		if ciMode {
			fmt.Fprintln(os.Stderr, "contextception: no changed files")
			return nil
		}
		fmt.Println("No changed files in range", refRange)
		return nil
	}

	durationMs := time.Since(start).Milliseconds()

	// Record to history (best-effort).
	if hist, err := history.Open(repoRoot); err == nil {
		defer hist.Close()
		commitSHA, _ := gitHeadSHA()
		branch, _ := gitCurrentBranch()
		hist.RecordRun(report, commitSHA, branch)

		// Record risk scores for percentile tracking.
		var riskEntries []history.RiskScoreEntry
		for _, cf := range report.ChangedFiles {
			if cf.RiskScore > 0 {
				riskEntries = append(riskEntries, history.RiskScoreEntry{
					FilePath:  cf.File,
					RiskScore: cf.RiskScore,
					RefRange:  refRange,
				})
			}
		}
		_ = hist.StoreRiskScores(riskEntries)

		// Compute percentile for aggregate risk (best-effort).
		if report.AggregateRisk != nil {
			if pct, err := hist.ComputePercentile(report.AggregateRisk.Score); err == nil && pct > 0 {
				report.AggregateRisk.Percentile = pct
			}
		}

		// Record usage analytics.
		entry := history.UsageEntryFromChangeReport("cli", nil, report, durationMs, mode, tokenBudget)
		_, _ = hist.RecordUsage(entry)
	}

	// CI mode.
	if ciMode {
		level := report.BlastRadius.Level
		fmt.Fprintf(os.Stderr, "contextception: %s blast_radius=%s files=%d\n",
			refRange, level, len(report.ChangedFiles))
		// Append risk badge.
		badge := analyzer.FormatRiskBadge(report)
		if badge != "" {
			fmt.Fprintln(os.Stderr, badge)
		}
		handleCIExit(level, report)
		return nil
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	if compactOutput {
		fmt.Print(analyzer.FormatCompactChange(report))
		return nil
	}

	fmt.Print(formatChangeReport(report))
	return nil
}

// detectDefaultBranch finds the default branch (main or master).
func detectDefaultBranch() (string, error) {
	// Try git symbolic-ref for remote HEAD.
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		// refs/remotes/origin/main -> main
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// Fall back: check if main or master exist.
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", branch)
		cmd.Dir = repoRoot
		if err := cmd.Run(); err == nil {
			return branch, nil
		}
	}

	return "", fmt.Errorf("could not detect default branch (tried origin/HEAD, main, master)")
}

// gitMergeBase returns the merge base of two refs.
func gitMergeBase(a, b string) (string, error) {
	cmd := exec.Command("git", "merge-base", a, b)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// formatChangeReport renders the change report as human-readable text.
func formatChangeReport(report *model.ChangeReport) string {
	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "PR Impact Report: %s\n", report.RefRange)
	b.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Summary
	s := report.Summary
	fmt.Fprintf(&b, "Changed Files: %d", s.TotalFiles)
	parts := []string{}
	if s.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", s.Added))
	}
	if s.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d modified", s.Modified))
	}
	if s.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", s.Deleted))
	}
	if s.Renamed > 0 {
		parts = append(parts, fmt.Sprintf("%d renamed", s.Renamed))
	}
	if len(parts) > 0 {
		b.WriteString(" (" + strings.Join(parts, ", ") + ")")
	}
	b.WriteString("\n")

	if s.TestFiles > 0 {
		fmt.Fprintf(&b, "Test Files:    %d\n", s.TestFiles)
	}
	if s.IndexedFiles < s.TotalFiles {
		fmt.Fprintf(&b, "Indexed:       %d of %d (new/deleted files may not be indexed)\n",
			s.IndexedFiles, s.TotalFiles)
	}

	// Blast Radius
	if report.BlastRadius != nil {
		fmt.Fprintf(&b, "\nBlast Radius: %s\n", formatBlastLevel(report.BlastRadius.Level))
		fmt.Fprintf(&b, "  %s\n", report.BlastRadius.Detail)
		if report.BlastRadius.Fragility > 0 {
			fmt.Fprintf(&b, "  Fragility: %.2f\n", report.BlastRadius.Fragility)
		}
	}

	// High-risk files
	if s.HighRiskFiles > 0 {
		fmt.Fprintf(&b, "\nHigh-Risk Files (%d):\n", s.HighRiskFiles)
		for _, f := range report.ChangedFiles {
			if f.BlastRadius != nil && f.BlastRadius.Level == "high" {
				fmt.Fprintf(&b, "  %s [%s]\n", f.File, f.Status)
			}
		}
	}

	// Coupling
	if len(report.Coupling) > 0 {
		fmt.Fprintf(&b, "\nCross-File Coupling (%d connections):\n", len(report.Coupling))
		for _, c := range report.Coupling {
			arrow := " -> "
			switch c.Direction {
			case "mutual":
				arrow = " <-> "
			case "b_imports_a":
				arrow = " <- "
			}
			fmt.Fprintf(&b, "  %s%s%s\n", c.FileA, arrow, c.FileB)
		}
	}

	// Must Read
	if len(report.MustRead) > 0 {
		fmt.Fprintf(&b, "\nMust Read (%d files):\n", len(report.MustRead))
		for _, entry := range report.MustRead {
			line := "  " + entry.File
			if len(entry.Symbols) > 0 {
				line += " (" + strings.Join(entry.Symbols, ", ") + ")"
			}
			var tags []string
			if entry.Direction != "" {
				tags = append(tags, entry.Direction)
			}
			if entry.Stable {
				tags = append(tags, "stable")
			}
			if entry.Circular {
				tags = append(tags, "circular")
			}
			if len(tags) > 0 {
				line += " [" + strings.Join(tags, ", ") + "]"
			}
			b.WriteString(line + "\n")
		}
	}

	// Tests
	if len(report.Tests) > 0 {
		fmt.Fprintf(&b, "\nTest Coverage (%d files):\n", len(report.Tests))
		direct := 0
		for _, t := range report.Tests {
			if t.Direct {
				direct++
			}
		}
		fmt.Fprintf(&b, "  %d direct, %d dependency\n", direct, len(report.Tests)-direct)
	}

	// Test Gaps
	if len(report.TestGaps) > 0 {
		fmt.Fprintf(&b, "\nTest Gaps (%d files with no direct tests):\n", len(report.TestGaps))
		for _, f := range report.TestGaps {
			fmt.Fprintf(&b, "  %s\n", f)
		}
	}

	// Hotspots
	if len(report.Hotspots) > 0 {
		fmt.Fprintf(&b, "\nHotspots (%d files — high churn + high fan-in):\n", len(report.Hotspots))
		for _, h := range report.Hotspots {
			b.WriteString("  " + h + "\n")
		}
	}

	// Circular Dependencies
	if len(report.CircularDeps) > 0 {
		fmt.Fprintf(&b, "\nCircular Dependencies (%d cycles):\n", len(report.CircularDeps))
		for _, cycle := range report.CircularDeps {
			b.WriteString("  " + strings.Join(cycle, " -> ") + "\n")
		}
	}

	// Stats
	if report.Stats != nil {
		fmt.Fprintf(&b, "\nIndex: %d files, %d edges, %d unresolved\n",
			report.Stats.TotalFiles, report.Stats.TotalEdges, report.Stats.UnresolvedCount)
	}

	return b.String()
}

// gitHeadSHA returns the current HEAD commit SHA.
func gitHeadSHA() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitCurrentBranch returns the current branch name.
func gitCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// autoReindex runs an incremental index update to ensure new/modified files
// on disk are reflected in the analysis. This is silent — no output unless verbose.
func autoReindex(idx *db.Index) error {
	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: repoRoot,
		Index:    idx,
		Verbose:  verbose,
		Config:   repoCfg,
	})
	if err != nil {
		return err
	}
	return ix.Run()
}

// formatBlastLevel returns a formatted blast radius level with visual indicator.
func formatBlastLevel(level string) string {
	switch level {
	case "high":
		return "HIGH [!!!]"
	case "medium":
		return "MEDIUM [!!]"
	default:
		return "LOW [.]"
	}
}
