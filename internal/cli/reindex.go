package cli

import (
	"fmt"
	"os"

	"github.com/kehoej/contextception/internal/db"
	"github.com/spf13/cobra"
)

func newReindexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reindex",
		Short: "Delete and rebuild the contextception index from scratch",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReindex()
		},
	}
}

func runReindex() error {
	idxPath := db.IndexPath(repoRoot)

	// Remove existing index if present.
	if _, err := os.Stat(idxPath); err == nil {
		if err := os.Remove(idxPath); err != nil {
			return fmt.Errorf("removing existing index: %w", err)
		}
		// Also remove WAL and SHM files if they exist.
		os.Remove(idxPath + "-wal")
		os.Remove(idxPath + "-shm")

		if verbose {
			fmt.Println("Removed existing index")
		}
	}

	return runIndex()
}
