package cli

import (
	"fmt"
	"os"

	"github.com/kehoej/contextception/internal/update"
	"github.com/kehoej/contextception/internal/version"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update contextception to the latest version",
		Long:  `Check for and install the latest version of contextception.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentVersion := version.Version
			if currentVersion == "dev" {
				fmt.Println("Running a development build. Use 'go install' to update.")
				return nil
			}

			fmt.Println("Checking for updates...")
			latestVersion, err := update.FetchLatest("")
			if err != nil {
				return fmt.Errorf("checking for updates: %w", err)
			}

			if !update.IsNewer(currentVersion, latestVersion) {
				fmt.Printf("Already up to date (%s)\n", currentVersion)
				return nil
			}

			binPath, err := update.CurrentBinaryPath()
			if err != nil {
				return fmt.Errorf("detecting binary path: %w", err)
			}

			method := update.DetectInstallMethod(binPath)
			switch method {
			case update.Homebrew:
				fmt.Printf("Installed via Homebrew. Run 'brew upgrade contextception' to update to %s.\n", latestVersion)
				return nil
			case update.GoInstall:
				fmt.Printf("Installed via go install. Run 'go install github.com/kehoej/contextception/cmd/contextception@latest' to update to %s.\n", latestVersion)
				return nil
			}

			fmt.Printf("Updating %s → %s...\n", currentVersion, latestVersion)
			if err := update.SelfUpdate(binPath, latestVersion, ""); err != nil {
				return fmt.Errorf("update failed: %w", err)
			}

			fmt.Printf("Updated contextception from %s to %s\n", currentVersion, latestVersion)

			// Auto-run setup to install updated slash commands.
			fmt.Println("\nUpdating editor integrations...")
			if err := runSetup("auto", false, false); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: auto-setup failed: %v\n", err)
				fmt.Fprintln(os.Stderr, "Run 'contextception setup' manually to update editor commands.")
			}
			return nil
		},
	}
}
