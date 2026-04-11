package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kehoej/contextception/internal/history"
	"github.com/kehoej/contextception/internal/session"
	"github.com/spf13/cobra"
)

var (
	discoverSince  int
	discoverFormat string
	discoverAll    bool
)

func newDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Find files edited without get_context being called",
		Long: `Scans Claude Code session files to find supported files that were edited
without first calling get_context. Reports missed opportunities with
edit frequency.

Examples:
  contextception discover                  # last 7 days
  contextception discover --since 30       # last 30 days
  contextception discover --all            # include test files
  contextception discover --format json    # JSON export`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiscover()
		},
	}

	cmd.Flags().IntVar(&discoverSince, "since", 7, "lookback window in days")
	cmd.Flags().StringVar(&discoverFormat, "format", "", "output format: json")
	cmd.Flags().BoolVar(&discoverAll, "all", false, "include test files in missed list")

	return cmd
}

func runDiscover() error {
	since := time.Now().AddDate(0, 0, -discoverSince)

	// Open history to cross-reference usage_log (hook-injected context).
	var usageLog session.UsageLogQuerier
	if hist, hErr := history.Open(repoRoot); hErr == nil {
		defer hist.Close()
		usageLog = hist
	}

	result, err := session.Discover(repoRoot, since, discoverAll, isSupportedExtension, usageLog)
	if err != nil {
		return fmt.Errorf("discovering: %w", err)
	}

	switch discoverFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	default:
		fmt.Print(session.FormatDiscoverSummary(result))
		return nil
	}
}
