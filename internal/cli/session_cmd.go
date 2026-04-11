package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kehoej/contextception/internal/session"
	"github.com/spf13/cobra"
)

var (
	sessionLimit  int
	sessionFormat string
)

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Show contextception adoption across Claude Code sessions",
		Long: `Analyzes recent Claude Code sessions to show how often get_context
is called before editing supported files. Displays adoption rate
per session with coverage percentages.

Examples:
  contextception session                   # last 10 sessions
  contextception session --limit 20        # more sessions
  contextception session --format json     # JSON export`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSession()
		},
	}

	cmd.Flags().IntVar(&sessionLimit, "limit", 10, "max sessions to show")
	cmd.Flags().StringVar(&sessionFormat, "format", "", "output format: json")

	return cmd
}

func runSession() error {
	since := time.Now().AddDate(0, 0, -30) // last 30 days

	stats, err := session.GetSessionStats(repoRoot, since, sessionLimit, isSupportedExtension)
	if err != nil {
		return fmt.Errorf("analyzing sessions: %w", err)
	}

	switch sessionFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	default:
		fmt.Print(session.FormatSessionSummary(stats))
		return nil
	}
}
