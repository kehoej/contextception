package cli

import (
	"fmt"
	"os"

	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/indexer"
	"github.com/spf13/cobra"
)

func newIndexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Build or update the contextception index",
		Long:  "Scans the repository, extracts imports, resolves dependencies, and stores them in the index.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIndex()
		},
	}
}

func runIndex() error {
	idxPath := db.IndexPath(repoRoot)

	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer idx.Close()

	applied, err := idx.MigrateToLatest()
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	if applied {
		idx.SetMeta("last_indexed_commit", "")
		fmt.Fprintln(os.Stderr, "Schema migrated — running full reindex")
	}

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: repoRoot,
		Index:    idx,
		Verbose:  verbose,
		Output:   os.Stderr,
		Config:   repoCfg,
	})
	if err != nil {
		return fmt.Errorf("creating indexer: %w", err)
	}

	if err := ix.Run(); err != nil {
		return fmt.Errorf("indexing: %w", err)
	}

	// Print summary.
	fileCount, _ := idx.FileCount()
	edgeCount, _ := idx.EdgeCount()
	unresolvedCount, _ := idx.UnresolvedCount()

	fmt.Printf("Index:      %s\n", idxPath)
	fmt.Printf("Files:      %d\n", fileCount)
	fmt.Printf("Edges:      %d\n", edgeCount)
	fmt.Printf("Unresolved: %d\n", unresolvedCount)

	return nil
}
