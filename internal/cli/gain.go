package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kehoej/contextception/internal/history"
	"github.com/spf13/cobra"
)

var (
	gainDaily   bool
	gainWeekly  bool
	gainMonthly bool
	gainSince   int
	gainFormat  string
)

func newGainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gain",
		Short: "Show usage analytics dashboard",
		Long: `Displays how contextception is being used: analysis counts, top files,
confidence trends, and blast radius distribution.

Inspired by RTK's analytics dashboard.

Examples:
  contextception gain                  # summary (last 30 days)
  contextception gain --daily          # day-by-day breakdown
  contextception gain --weekly         # week-by-week breakdown
  contextception gain --since 7        # last 7 days only
  contextception gain --format json    # JSON export`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGain()
		},
	}

	cmd.Flags().BoolVar(&gainDaily, "daily", false, "show day-by-day breakdown")
	cmd.Flags().BoolVar(&gainWeekly, "weekly", false, "show week-by-week breakdown")
	cmd.Flags().BoolVar(&gainMonthly, "monthly", false, "show month-by-month breakdown")
	cmd.Flags().IntVar(&gainSince, "since", 30, "lookback window in days")
	cmd.Flags().StringVar(&gainFormat, "format", "", "output format: json, csv")

	return cmd
}

func runGain() error {
	store, err := history.Open(repoRoot)
	if err != nil {
		return fmt.Errorf("opening history: %w", err)
	}
	defer store.Close()

	since := time.Now().AddDate(0, 0, -gainSince)

	summary, err := store.GetUsageSummary(since)
	if err != nil {
		return fmt.Errorf("querying summary: %w", err)
	}

	topFiles, err := store.GetTopFiles(since, 10)
	if err != nil {
		return fmt.Errorf("querying top files: %w", err)
	}

	// Build export structure.
	export := &history.GainExport{
		Summary:  summary,
		TopFiles: topFiles,
	}

	// Fetch period breakdowns if requested or for default daily view.
	fetchDaily := gainDaily || (!gainWeekly && !gainMonthly && gainFormat == "")
	if fetchDaily || gainFormat != "" {
		daily, err := store.GetUsageByPeriod("day", since, 90)
		if err != nil {
			return fmt.Errorf("querying daily: %w", err)
		}
		export.Daily = daily
	}
	if gainWeekly || gainFormat != "" {
		weekly, err := store.GetUsageByPeriod("week", since, 52)
		if err != nil {
			return fmt.Errorf("querying weekly: %w", err)
		}
		export.Weekly = weekly
	}
	if gainMonthly || gainFormat != "" {
		monthly, err := store.GetUsageByPeriod("month", since, 12)
		if err != nil {
			return fmt.Errorf("querying monthly: %w", err)
		}
		export.Monthly = monthly
	}

	switch gainFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(export)
	case "csv":
		return writeGainCSV(export)
	default:
		return writeGainText(export)
	}
}

func writeGainText(export *history.GainExport) error {
	fmt.Print(history.FormatGainSummary(export.Summary, export.TopFiles, export.Daily))

	if gainWeekly && len(export.Weekly) > 0 {
		fmt.Println("\n  Weekly breakdown:")
		fmt.Println("  " + repeatDash(46))
		for i := len(export.Weekly) - 1; i >= 0; i-- {
			w := export.Weekly[i]
			fmt.Printf("  %-12s %3d analyses  avg conf: %.2f  avg %dms\n",
				w.Period, w.Analyses, w.AvgConfidence, int(w.AvgDurationMs))
		}
	}

	if gainMonthly && len(export.Monthly) > 0 {
		fmt.Println("\n  Monthly breakdown:")
		fmt.Println("  " + repeatDash(46))
		for i := len(export.Monthly) - 1; i >= 0; i-- {
			m := export.Monthly[i]
			fmt.Printf("  %-12s %3d analyses  avg conf: %.2f  avg %dms\n",
				m.Period, m.Analyses, m.AvgConfidence, int(m.AvgDurationMs))
		}
	}

	return nil
}

func writeGainCSV(export *history.GainExport) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	header := []string{"period", "analyses", "files", "avg_confidence", "avg_duration_ms"}
	_ = w.Write(header)

	periods := export.Daily
	if gainWeekly {
		periods = export.Weekly
	}
	if gainMonthly {
		periods = export.Monthly
	}

	for _, p := range periods {
		_ = w.Write([]string{
			p.Period,
			fmt.Sprintf("%d", p.Analyses),
			fmt.Sprintf("%d", p.Files),
			fmt.Sprintf("%.2f", p.AvgConfidence),
			fmt.Sprintf("%.0f", p.AvgDurationMs),
		})
	}

	return nil
}

func repeatDash(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '-'
	}
	return string(b)
}
