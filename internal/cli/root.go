// Package cli defines the cobra command tree for contextception.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/update"
	"github.com/kehoej/contextception/internal/version"
	"github.com/spf13/cobra"
)

var (
	repoRoot           string
	verbose            bool
	repoCfg            *config.Config
	noUpdateCheck      bool
	updateNotification string
	updateRefreshDone  <-chan struct{}
)

// NewRootCmd creates the root cobra command with all subcommands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "contextception",
		Short: "Code context intelligence engine",
		Long:  `Contextception answers: "What code must be understood before making a safe change?"`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Version check (cache-then-notify: sync read, async refresh).
			// Skip for the update command itself — user is already updating.
			if !noUpdateCheck && os.Getenv("CONTEXTCEPTION_NO_UPDATE_CHECK") == "" && cmd.Name() != "update" {
				configDir, err := os.UserConfigDir()
				if err == nil {
					globalCfg := config.LoadGlobal(configDir)
					if globalCfg.Update.Check {
						result := update.CheckForUpdate(version.Version, configDir, "")
						if result.ShouldNotify {
							updateNotification = fmt.Sprintf(
								"A new version of contextception is available (%s). Run 'contextception update' to upgrade.",
								result.LatestVersion,
							)
						}
						updateRefreshDone = result.RefreshDone
					}
				}
			}

			// Clean up leftover .bak file from Windows self-update.
			if runtime.GOOS == "windows" {
				if exe, err := os.Executable(); err == nil {
					os.Remove(exe + ".bak") //nolint:errcheck
				}
			}

			// Skip repo root detection for commands that don't need it.
			if cmd.Name() == "update" || cmd.Name() == "setup" || cmd.Name() == "hook-check" || cmd.Name() == "hook-context" || cmd.Name() == "contextception" {
				return nil
			}

			// Resolve repo root if not explicitly set.
			if repoRoot == "" {
				detected, err := detectRepoRoot()
				if err != nil {
					return fmt.Errorf("could not detect repository root (use --repo to specify): %w", err)
				}
				repoRoot = detected
			}
			// Ensure it's absolute.
			abs, err := filepath.Abs(repoRoot)
			if err != nil {
				return fmt.Errorf("resolving repo path: %w", err)
			}
			repoRoot = abs

			// Load config.
			repoCfg, err = config.Load(repoRoot)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Validate and print warnings if config has a version.
			if repoCfg.Version > 0 {
				warnings, vErr := repoCfg.Validate(repoRoot)
				if vErr != nil {
					return fmt.Errorf("config validation: %w", vErr)
				}
				for _, w := range warnings {
					fmt.Fprintf(os.Stderr, "config warning: %s\n", w)
				}
			}

			return nil
		},
		Version: version.Version,
	}

	// Register finalizer once per NewRootCmd() call (not per-execution) to
	// drain the background refresh goroutine and show the update notification.
	// Uses cobra.OnFinalize so it fires even when RunE returns an error.
	cobra.OnFinalize(func() {
		if updateRefreshDone != nil {
			<-updateRefreshDone
		}
		if updateNotification != "" && !ciMode && !jsonOutput {
			fmt.Fprintln(os.Stderr, updateNotification)
		}
	})

	root.PersistentFlags().StringVar(&repoRoot, "repo", "", "repository root (defaults to git root detection)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	root.PersistentFlags().BoolVar(&noUpdateCheck, "no-update-check", false, "disable update version check")

	root.AddCommand(newIndexCmd())
	root.AddCommand(newAnalyzeCmd())
	root.AddCommand(newAnalyzeChangeCmd())
	root.AddCommand(newReindexCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newHistoryCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newArchetypesCmd())
	root.AddCommand(newExtensionsCmd())
	root.AddCommand(newUpdateCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newHookCheckCmd())
	root.AddCommand(newHookContextCmd())
	root.AddCommand(newGainCmd())
	root.AddCommand(newAccuracyCmd())

	return root
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// detectRepoRoot finds the git repository root from the current directory.
func detectRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// registerAnalysisFlags adds the shared analysis flags (json, caps, CI, mode)
// to a cobra command. Both analyze and analyze-change use these.
func registerAnalysisFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output compact JSON instead of pretty text")
	cmd.Flags().BoolVar(&noExternal, "no-external", false, "omit external dependencies from output")
	cmd.Flags().IntVar(&maxMustRead, "max-must-read", 0, "max must_read entries (default 10, 0 = default)")
	cmd.Flags().IntVar(&maxRelated, "max-related", 0, "max related entries (default 10, 0 = default)")
	cmd.Flags().IntVar(&maxLikelyModify, "max-likely-modify", 0, "max likely_modify entries (default 15, 0 = default)")
	cmd.Flags().IntVar(&maxTests, "max-tests", 0, "max test entries (default 5, 0 = default)")
	cmd.Flags().BoolVar(&signatures, "signatures", false, "include code signatures for must_read symbols")
	cmd.Flags().BoolVar(&ciMode, "ci", false, "CI mode: suppress output, exit code reflects blast radius")
	cmd.Flags().StringVar(&failOn, "fail-on", "high", "blast radius level that triggers non-zero exit (high, medium)")
	cmd.Flags().StringVar(&mode, "mode", "", "workflow mode: plan, implement, review (adjusts caps)")
	cmd.Flags().IntVar(&tokenBudget, "token-budget", 0, "target token budget for output (adjusts caps automatically)")
}

// handleCIExit checks the blast radius level and exits with an appropriate code
// in CI mode. Returns true if the caller should return (CI mode handled the output).
func handleCIExit(level string) bool {
	if !ciMode {
		return false
	}
	switch failOn {
	case "medium":
		if level == "high" {
			os.Exit(2)
		}
		if level == "medium" {
			os.Exit(1)
		}
	default: // "high"
		if level == "high" {
			os.Exit(2)
		}
	}
	return true
}
