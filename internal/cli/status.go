package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/kehoej/contextception/internal/db"
	"github.com/spf13/cobra"
)

// defaultLookbackDays is the number of days for git signal queries in status output.
const defaultLookbackDays = 90

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show index status and diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	idxPath := db.IndexPath(repoRoot)

	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		fmt.Println("No index found. Run `contextception index` to create one.")
		return nil
	}

	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer idx.Close()

	version, err := idx.SchemaVersion()
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	lastIndexed, _ := idx.GetMeta("last_indexed_at")
	if lastIndexed == "" {
		lastIndexed = "(never)"
	}

	lastCommit, _ := idx.GetMeta("last_indexed_commit")
	if lastCommit == "" {
		lastCommit = "(none)"
	} else if len(lastCommit) > 12 {
		lastCommit = lastCommit[:12]
	}

	fileCount, _ := idx.FileCount()
	edgeCount, _ := idx.EdgeCount()
	unresolvedSummary, _ := idx.GetUnresolvedSummary()
	entrypoints, _ := idx.EntrypointCount()
	utilities, _ := idx.UtilityCount()
	pkgRoots, _ := idx.PackageRootCount()

	fmt.Printf("Index:          %s\n", idxPath)
	fmt.Printf("Schema version: %d\n", version)
	fmt.Printf("Last indexed:   %s\n", lastIndexed)
	fmt.Printf("Last commit:    %s\n", lastCommit)
	fmt.Printf("Files:          %d\n", fileCount)
	fmt.Printf("Edges:          %d\n", edgeCount)

	if unresolvedSummary.Total == 0 {
		fmt.Printf("Unresolved:     0\n")
	} else {
		// Sort reasons alphabetically for determinism.
		reasons := make([]string, 0, len(unresolvedSummary.ByReason))
		for r := range unresolvedSummary.ByReason {
			reasons = append(reasons, r)
		}
		sort.Strings(reasons)

		parts := make([]string, 0, len(reasons))
		for _, r := range reasons {
			parts = append(parts, fmt.Sprintf("%s: %d", r, unresolvedSummary.ByReason[r]))
		}
		fmt.Printf("Unresolved:     %d (%s)\n", unresolvedSummary.Total, strings.Join(parts, ", "))
	}

	fmt.Printf("Entrypoints:    %d\n", entrypoints)
	fmt.Printf("Utilities:      %d\n", utilities)
	fmt.Printf("Package roots:  %d\n", pkgRoots)

	repoProfile, _ := idx.GetMeta("repo_profile")
	profileSignals, _ := idx.GetMeta("repo_profile_signals")
	if repoProfile != "" {
		if profileSignals != "" {
			fmt.Printf("Repo profile:   %s (%s)\n", repoProfile, profileSignals)
		} else {
			fmt.Printf("Repo profile:   %s\n", repoProfile)
		}
	}

	gitChurn, _ := idx.GitSignalCount(defaultLookbackDays)
	coChangePairs, _ := idx.CoChangePairCount(defaultLookbackDays)
	fmt.Printf("Git churn:      %d files\n", gitChurn)
	fmt.Printf("Co-change:      %d pairs\n", coChangePairs)

	// Config summary.
	if repoCfg != nil && repoCfg.Version > 0 {
		fmt.Printf("Config:         %d entrypoints, %d ignore, %d generated\n",
			len(repoCfg.Entrypoints), len(repoCfg.Ignore), len(repoCfg.Generated))
	} else {
		fmt.Printf("Config:         (none)\n")
	}

	return nil
}
