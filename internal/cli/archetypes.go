package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/grader"
	"github.com/spf13/cobra"
)

func newArchetypesCmd() *cobra.Command {
	var categoriesFlag string
	var listFlag bool

	cmd := &cobra.Command{
		Use:   "archetypes",
		Short: "Detect archetype files from the index",
		Long:  `Auto-selects representative files matching architectural archetypes (Service, Model, Middleware, etc.) using index signals. Outputs JSON array. Use --categories to filter to specific categories, or --list to see available category names.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if listFlag {
				for _, name := range grader.AllArchetypeNames() {
					fmt.Println(name)
				}
				return nil
			}

			dbPath := db.IndexPath(repoRoot)
			idx, err := db.OpenIndex(dbPath)
			if err != nil {
				return fmt.Errorf("opening index: %w", err)
			}
			defer idx.Close()

			var categories []string
			if categoriesFlag != "" {
				categories = strings.Split(categoriesFlag, ",")
				for i := range categories {
					categories[i] = strings.TrimSpace(categories[i])
				}
			}

			candidates, err := grader.DetectArchetypes(idx, categories)
			if err != nil {
				return fmt.Errorf("detecting archetypes: %w", err)
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(candidates)
		},
	}

	cmd.Flags().StringVar(&categoriesFlag, "categories", "", "Comma-separated list of archetype categories to detect (default: all)")
	cmd.Flags().BoolVar(&listFlag, "list", false, "List available archetype category names")

	return cmd
}
