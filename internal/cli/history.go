package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kehoej/contextception/internal/history"
	"github.com/spf13/cobra"
)

var (
	historyLimit  int
	historyBranch string
	historyDays   int
)

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "View historical analysis trends",
		Long: `Shows blast radius trends, hotspot evolution, and risk distribution
over time.`,
	}

	cmd.AddCommand(newHistoryTrendCmd())
	cmd.AddCommand(newHistoryHotspotsCmd())
	cmd.AddCommand(newHistoryDistCmd())
	cmd.AddCommand(newHistoryFileCmd())
	return cmd
}

func newHistoryTrendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trend",
		Short: "Show blast radius trend over recent runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistoryTrend()
		},
	}
	cmd.Flags().IntVar(&historyLimit, "limit", 20, "number of runs to show")
	cmd.Flags().StringVar(&historyBranch, "branch", "", "filter by branch")
	return cmd
}

func newHistoryHotspotsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hotspots",
		Short: "Show files that frequently appear as hotspots",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistoryHotspots()
		},
	}
	cmd.Flags().IntVar(&historyLimit, "limit", 10, "number of hotspots to show")
	cmd.Flags().IntVar(&historyDays, "days", 30, "look-back period in days")
	return cmd
}

func newHistoryDistCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "distribution",
		Short: "Show blast radius distribution across runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistoryDist()
		},
	}
	cmd.Flags().IntVar(&historyDays, "days", 30, "look-back period in days")
	return cmd
}

func newHistoryFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file <path>",
		Short: "Show risk history for a specific file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistoryFile(args[0])
		},
	}
	cmd.Flags().IntVar(&historyLimit, "limit", 20, "number of entries to show")
	return cmd
}

func openHistory() (*history.Store, error) {
	return history.Open(repoRoot)
}

func runHistoryTrend() error {
	store, err := openHistory()
	if err != nil {
		return err
	}
	defer store.Close()

	trends, err := store.GetBlastRadiusTrend(historyLimit, historyBranch)
	if err != nil {
		return fmt.Errorf("querying trends: %w", err)
	}

	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(trends)
	}

	if len(trends) == 0 {
		fmt.Println("No analysis history yet. Run `contextception analyze-change` to start tracking.")
		return nil
	}

	fmt.Println("Blast Radius Trend")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%-20s %-8s %-6s %s\n", "Date", "Level", "Files", "Branch")
	fmt.Println(strings.Repeat("-", 60))
	for _, t := range trends {
		date := t.Date
		if len(date) > 16 {
			date = date[:16]
		}
		indicator := levelIndicator(t.Level)
		fmt.Printf("%-20s %-8s %-6d %s %s\n", date, t.Level, t.Files, t.Branch, indicator)
	}
	return nil
}

func runHistoryHotspots() error {
	store, err := openHistory()
	if err != nil {
		return err
	}
	defer store.Close()

	since := time.Now().AddDate(0, 0, -historyDays)
	hotspots, err := store.GetHotspotEvolution(historyLimit, since)
	if err != nil {
		return fmt.Errorf("querying hotspots: %w", err)
	}

	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(hotspots)
	}

	if len(hotspots) == 0 {
		fmt.Printf("No hotspots recorded in the last %d days.\n", historyDays)
		return nil
	}

	fmt.Printf("Hotspot Evolution (last %d days)\n", historyDays)
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-40s %-8s %s\n", "File", "Count", "Last Seen")
	fmt.Println(strings.Repeat("-", 70))
	for _, h := range hotspots {
		lastSeen := h.LastSeen
		if len(lastSeen) > 10 {
			lastSeen = lastSeen[:10]
		}
		fmt.Printf("%-40s %-8d %s\n", truncPath(h.File, 40), h.Count, lastSeen)
	}
	return nil
}

func runHistoryDist() error {
	store, err := openHistory()
	if err != nil {
		return err
	}
	defer store.Close()

	since := time.Now().AddDate(0, 0, -historyDays)
	dist, err := store.GetBlastDistribution(since)
	if err != nil {
		return fmt.Errorf("querying distribution: %w", err)
	}

	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(dist)
	}

	total, _ := store.RunCount()
	fmt.Printf("Blast Radius Distribution (last %d days, %d total runs)\n", historyDays, total)
	fmt.Println(strings.Repeat("-", 40))
	for _, d := range dist {
		bar := strings.Repeat("#", int(d.Percent/2))
		fmt.Printf("%-8s %3d (%5.1f%%) %s\n", d.Level, d.Count, d.Percent, bar)
	}
	return nil
}

func runHistoryFile(filePath string) error {
	store, err := openHistory()
	if err != nil {
		return err
	}
	defer store.Close()

	records, err := store.GetFileRiskHistory(filePath, historyLimit)
	if err != nil {
		return fmt.Errorf("querying file history: %w", err)
	}

	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(records)
	}

	if len(records) == 0 {
		fmt.Printf("No history for %s\n", filePath)
		return nil
	}

	fmt.Printf("Risk History: %s\n", filePath)
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("%-20s %-10s %s\n", "Date", "Risk", "Status")
	fmt.Println(strings.Repeat("-", 50))
	for _, r := range records {
		date := r.Date
		if len(date) > 16 {
			date = date[:16]
		}
		fmt.Printf("%-20s %-10s %s\n", date, r.BlastLevel, r.Status)
	}
	return nil
}

func levelIndicator(level string) string {
	switch level {
	case "high":
		return "[!!!]"
	case "medium":
		return "[!!]"
	default:
		return "[.]"
	}
}

func truncPath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
