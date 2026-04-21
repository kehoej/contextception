package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/history"
	"github.com/kehoej/contextception/internal/model"
	"github.com/spf13/cobra"
)

// fragilityDisplayThreshold: only show fragility in output when below this value.
// At or above this, blast radius is already escalated so the number adds no signal.
const fragilityDisplayThreshold = 0.7

var (
	jsonOutput      bool
	stableThreshold int
	noExternal      bool
	maxMustRead     int
	maxRelated      int
	maxLikelyModify int
	maxTests        int
	signatures      bool
	ciMode          bool
	failOn          string
	mode            string
	tokenBudget     int
	compactOutput   bool
)

func newAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze <file> [file2 ...]",
		Short: "Analyze context dependencies for one or more files",
		Long:  "Determines what code must be understood before making a safe change to the given file(s).",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAnalyze(args)
		},
	}
	registerAnalysisFlags(cmd)
	cmd.Flags().IntVar(&stableThreshold, "stable-threshold", 0, "indegree threshold for marking files as stable (default: adaptive, max(5, maxIndegree/2))")
	return cmd
}

func runAnalyze(files []string) error {
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
	if err := autoReindex(idx); err != nil {
		// Non-fatal: proceed with stale index rather than failing.
		fmt.Fprintf(os.Stderr, "Warning: auto-reindex failed: %v\n", err)
	}

	// Convert all files to repo-relative paths.
	targets := make([]string, len(files))
	for i, file := range files {
		target := file
		if filepath.IsAbs(target) {
			rel, err := filepath.Rel(repoRoot, target)
			if err != nil {
				return fmt.Errorf("resolving path: %w", err)
			}
			target = rel
		}
		// Normalize to forward slashes for consistency.
		targets[i] = strings.ReplaceAll(filepath.Clean(target), string(os.PathSeparator), "/")
	}

	// Run analysis.
	a := analyzer.New(idx, analyzer.Config{
		RepoConfig:      repoCfg,
		StableThreshold: stableThreshold,
		OmitExternal:    noExternal,
		Signatures:      signatures,
		RepoRoot:        repoRoot,
		Caps: analyzer.Caps{
			MaxMustRead:     maxMustRead,
			MaxRelated:      maxRelated,
			MaxLikelyModify: maxLikelyModify,
			MaxTests:        maxTests,
			Mode:            mode,
			TokenBudget:     tokenBudget,
		},
	})

	start := time.Now()
	var output *model.AnalysisOutput
	if len(targets) == 1 {
		output, err = a.Analyze(targets[0])
	} else {
		output, err = a.AnalyzeMulti(targets)
	}
	if err != nil {
		return err
	}
	durationMs := time.Since(start).Milliseconds()

	// Record usage (best-effort).
	if hist, hErr := history.Open(repoRoot); hErr == nil {
		defer hist.Close()
		entry := history.UsageEntryFromAnalysis("cli", "analyze", targets, output, durationMs, mode, tokenBudget)
		_, _ = hist.RecordUsage(entry)
	}

	// CI mode: suppress normal output, exit with code based on blast radius.
	if ciMode {
		level := "low"
		if output.BlastRadius != nil {
			level = output.BlastRadius.Level
		}
		fmt.Fprintf(os.Stderr, "contextception: %s blast_radius=%s\n", output.Subject, level)
		handleCIExit(level, nil)
		return nil
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(output)
	}

	if compactOutput {
		fmt.Print(analyzer.FormatCompact(output))
		return nil
	}

	fmt.Print(formatPretty(output))
	return nil
}

// formatPretty renders the analysis output as human-readable text.
func formatPretty(output *model.AnalysisOutput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Context for: %s\n", output.Subject)

	// --- Must Read ---
	if len(output.MustRead) > 0 {
		fmt.Fprintf(&b, "\nMust Read (%d files):\n", len(output.MustRead))
		for _, entry := range output.MustRead {
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
			if len(tags) > 0 {
				line += " [" + strings.Join(tags, ", ") + "]"
			}
			b.WriteString(line + "\n")
			for _, def := range entry.Definitions {
				b.WriteString("    | " + def + "\n")
			}
		}
	} else if output.MustReadNote != "" {
		fmt.Fprintf(&b, "\nMust Read: %s\n", output.MustReadNote)
	}

	// --- Likely Modify ---
	lmCount := 0
	for _, entries := range output.LikelyModify {
		lmCount += len(entries)
	}
	if lmCount > 0 {
		b.WriteString("\nLikely Modify:\n")
		for _, key := range sortedKeys(output.LikelyModify) {
			entries := output.LikelyModify[key]
			// Extract short package name for display.
			shortKey := shortPackageName(key)
			for _, e := range entries {
				confLabel := strings.ToUpper(e.Confidence)
				if len(confLabel) < 4 {
					confLabel = confLabel + strings.Repeat(" ", 4-len(confLabel))
				}
				signals := formatSignals(e.Signals)
				fmt.Fprintf(&b, "  %-5s %-12s %-28s %s\n", confLabel, shortKey, e.File, signals)
			}
		}
		if output.LikelyModifyNote != "" {
			fmt.Fprintf(&b, "  (%s)\n", output.LikelyModifyNote)
		}
	}

	// --- Tests ---
	if len(output.Tests) > 0 {
		fmt.Fprintf(&b, "\nTests (%d files):\n", len(output.Tests))
		for _, t := range output.Tests {
			tier := "direct"
			if !t.Direct {
				tier = "dependency"
			}
			fmt.Fprintf(&b, "  %s [%s]\n", t.File, tier)
		}
		if output.TestsNote != "" {
			fmt.Fprintf(&b, "  (%s)\n", output.TestsNote)
		}
	} else if output.TestsNote != "" {
		fmt.Fprintf(&b, "\nTests: %s\n", output.TestsNote)
	}

	// --- Related ---
	relCount := 0
	for _, entries := range output.Related {
		relCount += len(entries)
	}
	if relCount > 0 {
		fmt.Fprintf(&b, "\nRelated (%d files):\n", relCount)
		for _, key := range sortedKeys(output.Related) {
			entries := output.Related[key]
			shortKey := shortPackageName(key)
			for _, e := range entries {
				signals := formatSignals(e.Signals)
				fmt.Fprintf(&b, "  %-12s %-28s %s\n", shortKey, e.File, signals)
			}
		}
	}

	// --- External ---
	if len(output.External) > 0 {
		fmt.Fprintf(&b, "\nExternal: %s\n", strings.Join(output.External, ", "))
	}

	// --- Blast Radius ---
	if output.BlastRadius != nil {
		line := fmt.Sprintf("\nBlast Radius: %s (%s)", output.BlastRadius.Level, output.BlastRadius.Detail)
		if output.BlastRadius.Fragility > 0 && output.BlastRadius.Fragility < fragilityDisplayThreshold {
			line += fmt.Sprintf(" | fragility: %.2f", output.BlastRadius.Fragility)
		}
		b.WriteString(line + "\n")
	}

	// --- Hotspots ---
	if len(output.Hotspots) > 0 {
		fmt.Fprintf(&b, "\nHotspots (%d files — high churn + high fan-in):\n", len(output.Hotspots))
		for _, h := range output.Hotspots {
			b.WriteString("  " + h + "\n")
		}
	}

	// --- Circular Dependencies ---
	if len(output.CircularDeps) > 0 {
		fmt.Fprintf(&b, "\nCircular Dependencies (%d cycles):\n", len(output.CircularDeps))
		for _, cycle := range output.CircularDeps {
			b.WriteString("  " + strings.Join(cycle, " -> ") + "\n")
		}
	}

	// --- Stats ---
	if output.Stats != nil {
		fmt.Fprintf(&b, "\nIndex: %d files, %d edges, %d unresolved\n",
			output.Stats.TotalFiles, output.Stats.TotalEdges, output.Stats.UnresolvedCount)
	}

	return b.String()
}

// shortPackageName returns the last component of a package key.
// "packages/agents-core" → "agents-core", "myapp" → "myapp", "." → "(root)"
func shortPackageName(key string) string {
	if key == "." {
		return "(root)"
	}
	parts := strings.Split(key, "/")
	return parts[len(parts)-1]
}

// formatSignals formats signal strings for human display.
func formatSignals(signals []string) string {
	display := make([]string, 0, len(signals))
	for _, s := range signals {
		if strings.HasPrefix(s, "co_change:") {
			n := strings.TrimPrefix(s, "co_change:")
			display = append(display, fmt.Sprintf("co-changed %sx", n))
		} else if strings.HasPrefix(s, "hidden_coupling:") {
			n := strings.TrimPrefix(s, "hidden_coupling:")
			display = append(display, fmt.Sprintf("hidden coupling (co-changed %sx, no import path)", n))
		} else {
			display = append(display, strings.ReplaceAll(s, "_", " "))
		}
	}
	return strings.Join(display, ", ")
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys[V any](m map[string][]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
