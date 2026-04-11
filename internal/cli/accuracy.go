package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kehoej/contextception/internal/history"
	"github.com/spf13/cobra"
)

var (
	accuracySince  int
	accuracyFormat string
)

func newAccuracyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accuracy",
		Short: "Show recommendation accuracy from LLM feedback",
		Long: `Displays precision, recall, and usefulness metrics computed from
LLM feedback submitted via the rate_context MCP tool.

Metrics:
  Must-read precision:    % of recommended files that were actually useful
  Must-read recall:       % of needed files that were recommended
  Likely-modify accuracy: % of predicted modifications that happened
  Overall usefulness:     Average rating (1-5)

Examples:
  contextception accuracy                  # summary (last 30 days)
  contextception accuracy --since 7        # last 7 days
  contextception accuracy --format json    # JSON export`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAccuracy()
		},
	}

	cmd.Flags().IntVar(&accuracySince, "since", 30, "lookback window in days")
	cmd.Flags().StringVar(&accuracyFormat, "format", "", "output format: json")

	return cmd
}

func runAccuracy() error {
	store, err := history.Open(repoRoot)
	if err != nil {
		return fmt.Errorf("opening history: %w", err)
	}
	defer store.Close()

	since := time.Now().AddDate(0, 0, -accuracySince)

	metrics, err := store.GetAccuracyMetrics(since)
	if err != nil {
		return fmt.Errorf("querying accuracy: %w", err)
	}

	lowestRated, err := store.GetLowestRated(since, 5)
	if err != nil {
		return fmt.Errorf("querying lowest rated: %w", err)
	}

	export := &history.AccuracyExport{
		Metrics:     metrics,
		LowestRated: lowestRated,
	}

	switch accuracyFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(export)
	default:
		fmt.Print(history.FormatAccuracySummary(metrics, lowestRated))
		return nil
	}
}
