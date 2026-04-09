package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

var (
	setupEditor    string
	setupDryRun    bool
	setupUninstall bool
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure contextception for your AI code editor",
		Long: `Automatically configure the MCP server and Claude Code hooks for contextception.

Supports Claude Code (default), Cursor, and Windsurf. For Claude Code, also
installs PreToolUse hooks that remind the AI to call get_context before editing files.

Run with --uninstall to reverse setup and remove all configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(setupEditor, setupDryRun, setupUninstall)
		},
	}

	cmd.Flags().StringVar(&setupEditor, "editor", "claude", "target editor: claude, cursor, windsurf")
	cmd.Flags().BoolVar(&setupDryRun, "dry-run", false, "show what would change without writing files")
	cmd.Flags().BoolVar(&setupUninstall, "uninstall", false, "remove contextception configuration")

	return cmd
}

func runSetup(editor string, dryRun bool, uninstall bool) error {
	editor = strings.ToLower(editor)

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	// Validate editor.
	mcpPath, hookPath, err := editorPaths(home, editor)
	if err != nil {
		return err
	}

	// PATH check (install only).
	if !uninstall {
		if _, err := exec.LookPath("contextception"); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: contextception not found on PATH. Hooks may not work until it's added.")
		}
	}

	editorLabel := editorDisplayName(editor)
	if uninstall {
		fmt.Printf("Removing contextception from %s...\n", editorLabel)
	} else {
		fmt.Printf("Setting up contextception for %s...\n", editorLabel)
	}

	if uninstall {
		return runUninstall(editor, mcpPath, hookPath, dryRun)
	}
	return runInstall(editor, mcpPath, hookPath, dryRun)
}

func runInstall(editor, mcpPath, hookPath string, dryRun bool) error {
	changed, err := ensureMCPConfig(mcpPath, editor, dryRun)
	if err != nil {
		return fmt.Errorf("MCP config: %w", err)
	}
	printStatus(changed, dryRun, "MCP server", mcpPath)

	if editor == "claude" {
		changed, err = ensureHookConfig(hookPath, dryRun)
		if err != nil {
			return fmt.Errorf("hook config: %w", err)
		}
		printStatus(changed, dryRun, "PreToolUse hooks", hookPath)
	}

	if dryRun {
		fmt.Println("\nDry run complete. No files were modified.")
	} else {
		fmt.Printf("\nSetup complete! Restart %s to activate.\n", editorDisplayName(editor))
	}
	return nil
}

func runUninstall(editor, mcpPath, hookPath string, dryRun bool) error {
	changed, err := removeMCPConfig(mcpPath, dryRun)
	if err != nil {
		return fmt.Errorf("MCP config: %w", err)
	}
	printRemoveStatus(changed, dryRun, "MCP server", mcpPath)

	if editor == "claude" {
		changed, err = removeHookConfig(hookPath, dryRun)
		if err != nil {
			return fmt.Errorf("hook config: %w", err)
		}
		printRemoveStatus(changed, dryRun, "PreToolUse hooks", hookPath)
	}

	if dryRun {
		fmt.Println("\nDry run complete. No files were modified.")
	} else {
		fmt.Println("\nUninstall complete.")
	}
	return nil
}

// ensureMCPConfig adds the contextception MCP server entry to the config file.
func ensureMCPConfig(configPath, editor string, dryRun bool) (bool, error) {
	data, err := readOrCreateJSON(configPath)
	if err != nil {
		return false, err
	}

	// Check if already configured.
	if gjson.GetBytes(data, "mcpServers.contextception").Exists() {
		return false, nil
	}

	entry := map[string]any{
		"command": "contextception",
		"args":    []string{"mcp"},
	}
	if editor == "cursor" || editor == "windsurf" {
		entry["transportType"] = "stdio"
	}

	data, err = sjson.SetBytes(data, "mcpServers.contextception", entry)
	if err != nil {
		return false, fmt.Errorf("setting mcpServers: %w", err)
	}

	if dryRun {
		return true, nil
	}
	return true, writeJSON(configPath, data)
}

// removeMCPConfig removes the contextception MCP server entry from the config file.
func removeMCPConfig(configPath string, dryRun bool) (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	if !gjson.GetBytes(data, "mcpServers.contextception").Exists() {
		return false, nil
	}

	data, err = sjson.DeleteBytes(data, "mcpServers.contextception")
	if err != nil {
		return false, fmt.Errorf("deleting mcpServers.contextception: %w", err)
	}

	if dryRun {
		return true, nil
	}
	return true, writeJSON(configPath, data)
}

// ensureHookConfig adds PreToolUse hook entries for Edit and Write matchers.
func ensureHookConfig(settingsPath string, dryRun bool) (bool, error) {
	data, err := readOrCreateJSON(settingsPath)
	if err != nil {
		return false, err
	}

	anyChanged := false
	for _, matcher := range []string{"Edit", "Write"} {
		if hasHookEntry(data, matcher) {
			continue
		}

		entry := map[string]any{
			"matcher": matcher,
			"hooks": []map[string]string{
				{
					"type":    "command",
					"command": "contextception hook-context",
				},
			},
		}

		data, err = sjson.SetBytes(data, "hooks.PreToolUse.-1", entry)
		if err != nil {
			return false, fmt.Errorf("adding hook for %s: %w", matcher, err)
		}
		anyChanged = true
	}

	if !anyChanged {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	return true, writeJSON(settingsPath, data)
}

// removeHookConfig removes contextception hook entries from PreToolUse.
func removeHookConfig(settingsPath string, dryRun bool) (bool, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	entries := gjson.GetBytes(data, "hooks.PreToolUse")
	if !entries.Exists() || !entries.IsArray() {
		return false, nil
	}

	// Find indices to remove (in reverse order to preserve indices).
	var toRemove []int
	entries.ForEach(func(key, value gjson.Result) bool {
		hooks := value.Get("hooks")
		if !hooks.Exists() || !hooks.IsArray() {
			return true
		}
		hooks.ForEach(func(_, hook gjson.Result) bool {
			cmd := hook.Get("command").String()
			if strings.Contains(cmd, "contextception hook-check") || strings.Contains(cmd, "contextception hook-context") || strings.Contains(cmd, "contextception-remind") {
				toRemove = append(toRemove, int(key.Int()))
				return false
			}
			return true
		})
		return true
	})

	if len(toRemove) == 0 {
		return false, nil
	}

	// Remove in reverse order to keep indices valid.
	for i := len(toRemove) - 1; i >= 0; i-- {
		path := fmt.Sprintf("hooks.PreToolUse.%d", toRemove[i])
		data, err = sjson.DeleteBytes(data, path)
		if err != nil {
			return false, fmt.Errorf("removing hook entry: %w", err)
		}
	}

	if dryRun {
		return true, nil
	}
	return true, writeJSON(settingsPath, data)
}

// hasHookEntry checks if a PreToolUse entry for the given matcher with a
// contextception command already exists.
func hasHookEntry(data []byte, matcher string) bool {
	entries := gjson.GetBytes(data, "hooks.PreToolUse")
	if !entries.Exists() || !entries.IsArray() {
		return false
	}

	found := false
	entries.ForEach(func(_, value gjson.Result) bool {
		if value.Get("matcher").String() != matcher {
			return true
		}
		hooks := value.Get("hooks")
		if !hooks.Exists() || !hooks.IsArray() {
			return true
		}
		hooks.ForEach(func(_, hook gjson.Result) bool {
			cmd := hook.Get("command").String()
			if strings.Contains(cmd, "contextception") {
				found = true
				return false
			}
			return true
		})
		return !found
	})
	return found
}

// editorPaths returns the MCP config path and hooks config path for the editor.
func editorPaths(home, editor string) (mcpPath, hookPath string, err error) {
	switch editor {
	case "claude":
		mcpPath = filepath.Join(home, ".claude.json")
		hookPath = filepath.Join(home, ".claude", "settings.json")
	case "cursor":
		mcpPath = filepath.Join(home, ".cursor", "mcp.json")
	case "windsurf":
		mcpPath = filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	default:
		return "", "", fmt.Errorf("unsupported editor: %s (supported: claude, cursor, windsurf)", editor)
	}
	return mcpPath, hookPath, nil
}

func editorDisplayName(editor string) string {
	switch editor {
	case "claude":
		return "Claude Code"
	case "cursor":
		return "Cursor"
	case "windsurf":
		return "Windsurf"
	default:
		return editor
	}
}

// readOrCreateJSON reads a JSON file or returns an empty JSON object if missing.
// Returns an error if the file contains invalid JSON.
func readOrCreateJSON(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte("{}"), nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []byte("{}"), nil
	}
	if !gjson.ValidBytes(data) {
		return nil, fmt.Errorf("invalid JSON in %s", path)
	}
	return data, nil
}

// writeJSON writes JSON data to a file, creating parent directories as needed.
// The data is pretty-printed with consistent 2-space indentation.
func writeJSON(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	formatted := pretty.Pretty(data)
	return os.WriteFile(path, formatted, 0o644)
}

func tildeDisplay(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func printStatus(changed, dryRun bool, label, path string) {
	display := tildeDisplay(path)
	if dryRun {
		if changed {
			fmt.Printf("  [dry-run] Would add %s to %s\n", label, display)
		} else {
			fmt.Printf("  %s already configured in %s\n", label, display)
		}
		return
	}
	if changed {
		fmt.Printf("  ✓ %s added to %s\n", label, display)
	} else {
		fmt.Printf("  %s already configured in %s\n", label, display)
	}
}

func printRemoveStatus(changed, dryRun bool, label, path string) {
	display := tildeDisplay(path)
	if dryRun {
		if changed {
			fmt.Printf("  [dry-run] Would remove %s from %s\n", label, display)
		} else {
			fmt.Printf("  %s not found in %s\n", label, display)
		}
		return
	}
	if changed {
		fmt.Printf("  ✓ %s removed from %s\n", label, display)
	} else {
		fmt.Printf("  %s not found in %s\n", label, display)
	}
}
