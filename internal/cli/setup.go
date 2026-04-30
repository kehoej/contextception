package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kehoej/contextception/internal/version"
	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"
)

var (
	setupEditor       string
	setupDryRun       bool
	setupUninstall    bool
	setupInstructions bool
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure contextception for your AI code editor",
		Long: `Automatically configure the MCP server and slash commands for contextception.

Supports Claude Code, Cursor, Windsurf, opencode, and VSCode. Warp is
detected for the no-op case but its MCP config is UI-only and cannot be
written from the CLI. Use --editor=auto (default) to auto-detect installed
editors. For Claude Code, also installs the /pr-risk and /pr-fix slash
commands and silently removes any legacy PreToolUse hook entries left
over from earlier versions.

Run with --uninstall to reverse setup and remove all configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(setupEditor, setupDryRun, setupUninstall, setupInstructions)
		},
	}

	cmd.Flags().StringVar(&setupEditor, "editor", "auto", "target editor: auto, claude, cursor, windsurf, opencode, vscode, warp")
	cmd.Flags().BoolVar(&setupDryRun, "dry-run", false, "show what would change without writing files")
	cmd.Flags().BoolVar(&setupUninstall, "uninstall", false, "remove contextception configuration")
	cmd.Flags().BoolVar(&setupInstructions, "instructions", false, "also write the agent instruction snippet (CLAUDE.md, AGENTS.md, .cursor/rules/, etc.) into the current project")

	return cmd
}

func runSetup(editor string, dryRun, uninstall, instructions bool) error {
	editor = strings.ToLower(editor)

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	// Tracks instruction-file paths we've already written this run, so
	// editors that share a destination (opencode + warp both use AGENTS.md)
	// don't print duplicate "already configured" lines.
	written := make(map[string]bool)

	// Auto-detect: find all installed editors and set up each one.
	if editor == "auto" {
		editors := detectInstalledEditors(home)
		if len(editors) == 0 {
			return fmt.Errorf("no supported editors detected (looked for Claude Code, Cursor, Windsurf, opencode, VSCode, Warp)")
		}
		fmt.Printf("Detected: %s\n\n", strings.Join(editorNames(editors), ", "))
		for _, ed := range editors {
			if err := runSetupForEditor(ed, home, dryRun, uninstall, instructions, written); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %s setup failed: %v\n", editorDisplayName(ed), err)
			}
			fmt.Println()
		}
		return nil
	}

	return runSetupForEditor(editor, home, dryRun, uninstall, instructions, written)
}

func runSetupForEditor(editor, home string, dryRun, uninstall, instructions bool, written map[string]bool) error {
	// Validate editor.
	mcpPath, hookPath, err := editorPaths(home, editor)
	if err != nil {
		return err
	}

	// PATH check (install only).
	if !uninstall {
		if _, err := exec.LookPath("contextception"); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: contextception not found on PATH. The MCP server won't launch until it's added.")
		}
	}

	editorLabel := editorDisplayName(editor)
	if uninstall {
		fmt.Printf("Removing contextception from %s...\n", editorLabel)
	} else {
		fmt.Printf("Setting up contextception for %s...\n", editorLabel)
	}

	if uninstall {
		return runUninstall(editor, mcpPath, hookPath, dryRun, instructions, written)
	}
	return runInstall(editor, mcpPath, hookPath, dryRun, instructions, written)
}

// detectInstalledEditors checks for the presence of supported editors.
func detectInstalledEditors(home string) []string {
	var found []string

	// Claude Code: ~/.claude/ directory exists.
	if dirExists(filepath.Join(home, ".claude")) {
		found = append(found, "claude")
	}

	// Cursor: ~/.cursor/ directory exists.
	if dirExists(filepath.Join(home, ".cursor")) {
		found = append(found, "cursor")
	}

	// Windsurf: ~/.codeium/windsurf/ directory exists.
	if dirExists(filepath.Join(home, ".codeium", "windsurf")) {
		found = append(found, "windsurf")
	}

	// opencode: ~/.config/opencode/ directory exists.
	if dirExists(filepath.Join(home, ".config", "opencode")) {
		found = append(found, "opencode")
	}

	// VSCode: user data directory exists (cross-platform path varies).
	if dir := vscodeDataDir(home); dir != "" && dirExists(dir) {
		found = append(found, "vscode")
	}

	// Warp: ~/.warp/ directory exists. (We can detect it but cannot
	// write its MCP config — that has to be done in Warp's UI.)
	if dirExists(filepath.Join(home, ".warp")) {
		found = append(found, "warp")
	}

	return found
}

// vscodeDataDir returns the per-OS directory where VSCode stores user-level
// configuration. Returns "" on unsupported platforms.
func vscodeDataDir(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code")
	case "linux":
		return filepath.Join(home, ".config", "Code")
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "Code")
	}
	return ""
}

// vscodeUserMCPPath returns the path to VSCode's user-level MCP config.
func vscodeUserMCPPath(home string) string {
	dir := vscodeDataDir(home)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "User", "mcp.json")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func editorNames(editors []string) []string {
	names := make([]string, len(editors))
	for i, ed := range editors {
		names[i] = editorDisplayName(ed)
	}
	return names
}

func runInstall(editor, mcpPath, hookPath string, dryRun, instructions bool, written map[string]bool) error {
	home, _ := os.UserHomeDir()

	// Warp: MCP is configured through Warp's UI (Settings → Agents →
	// MCP servers). Print the manual steps and skip the MCP write —
	// instructions installation (--instructions) still applies because
	// Warp reads AGENTS.md from the project root.
	if editor == "warp" {
		printWarpManualSetup(dryRun, instructions)
		if instructions {
			if err := installInstructionFile(editor, dryRun, written); err != nil {
				return err
			}
		}
		return nil
	}

	// MCP config — schema differs per editor.
	var (
		changed bool
		err     error
	)
	switch editor {
	case "opencode":
		changed, err = ensureOpencodeMCPConfig(mcpPath, dryRun)
	case "vscode":
		changed, err = ensureVscodeMCPConfig(mcpPath, dryRun)
	default:
		changed, err = ensureMCPConfig(mcpPath, editor, dryRun)
	}
	if err != nil {
		return fmt.Errorf("MCP config: %w", err)
	}
	printStatus(changed, dryRun, "MCP server", mcpPath)

	// Migration: silently remove any legacy contextception PreToolUse hook
	// entries from earlier versions that shipped a hook-based reminder.
	// We only print when something was actually removed.
	if editor == "claude" {
		removed, err := removeHookConfig(hookPath, dryRun)
		if err != nil {
			return fmt.Errorf("hook cleanup: %w", err)
		}
		if removed {
			if dryRun {
				fmt.Printf("  [dry-run] Would remove legacy PreToolUse hook from %s\n", tildeDisplay(hookPath))
			} else {
				fmt.Printf("  ✓ Removed legacy PreToolUse hook from %s\n", tildeDisplay(hookPath))
			}
		}
	}

	// Install slash commands (version-stamped so they auto-update on upgrade).
	ver := version.Version
	for name, content := range slashCommands {
		cmdPath := slashCommandDir(home, editor)
		if cmdPath == "" {
			break
		}
		fullPath := filepath.Join(cmdPath, name+".md")
		stamped := versionStamp(content, ver)
		changed, err = ensureSlashCommand(fullPath, stamped, dryRun)
		if err != nil {
			return fmt.Errorf("slash command %s: %w", name, err)
		}
		printStatus(changed, dryRun, "/"+name+" command", fullPath)
	}

	// Optionally write the agent instruction snippet into the current
	// project (--instructions flag). For all editors when set, this
	// upserts the contextception block into the right per-editor file
	// (CLAUDE.md / AGENTS.md / .cursor/rules/ / etc.) using begin/end
	// markers so user-authored content around our block is preserved.
	if instructions {
		if err := installInstructionFile(editor, dryRun, written); err != nil {
			return err
		}
	} else {
		// Helpful nudge for editors whose instruction file is per-project
		// and therefore not written by the global setup.
		switch editor {
		case "opencode":
			fmt.Println("  Note: pass --instructions next time, or copy integrations/AGENTS.md to AGENTS.md at your project root.")
		case "vscode":
			fmt.Println("  Note: pass --instructions next time, or copy integrations/AGENTS.md to .github/copilot-instructions.md.")
		}
	}

	// Record the version that was set up (for freshness checks).
	if !dryRun {
		writeSetupVersion(ver)
	}

	if dryRun {
		fmt.Println("\nDry run complete. No files were modified.")
	} else {
		fmt.Printf("\nSetup complete! Restart %s to activate.\n", editorDisplayName(editor))
	}
	return nil
}

// printWarpManualSetup prints instructions for the parts of warp setup that
// can't be automated. Warp configures MCP servers through its app UI. The
// `instructionsHandled` flag suppresses the AGENTS.md hint when --instructions
// was passed (the AGENTS.md block is being written by ensureInstructionFile
// in this same run).
func printWarpManualSetup(dryRun, instructionsHandled bool) {
	if dryRun {
		fmt.Println("  [dry-run] Warp is UI-only — no files would be written.")
		return
	}
	fmt.Println("  Warp configures MCP servers through its app UI, not a config file.")
	fmt.Println("  To finish setup:")
	fmt.Println("    1. Open Settings → Agents → MCP servers (or run \"Open MCP Servers\" from the command palette).")
	fmt.Println("    2. Click + Add → CLI command. Name: contextception. Command: contextception. Args: mcp.")
	if !instructionsHandled {
		fmt.Println("    3. Save, then copy integrations/AGENTS.md to AGENTS.md (all caps, required) at your project root.")
	} else {
		fmt.Println("    3. Save. The AGENTS.md instruction block is being written into this project below.")
	}
}

func runUninstall(editor, mcpPath, hookPath string, dryRun, instructions bool, written map[string]bool) error {
	home, _ := os.UserHomeDir()

	// Warp: nothing for us to remove from MCP. The UI entry is manual;
	// the instruction-file block can still be cleaned up if --instructions
	// is set.
	if editor == "warp" {
		fmt.Println("  Warp configures MCP through its UI — remove the contextception entry from Settings → Agents → MCP servers.")
		if instructions {
			if err := uninstallInstructionFile(editor, dryRun, written); err != nil {
				return err
			}
		} else {
			fmt.Println("  Pass --instructions to also strip the contextception block from AGENTS.md in this project.")
		}
		return nil
	}

	var (
		changed bool
		err     error
	)
	switch editor {
	case "opencode":
		changed, err = removeOpencodeMCPConfig(mcpPath, dryRun)
	case "vscode":
		changed, err = removeVscodeMCPConfig(mcpPath, dryRun)
	default:
		changed, err = removeMCPConfig(mcpPath, dryRun)
	}
	if err != nil {
		return fmt.Errorf("MCP config: %w", err)
	}
	printRemoveStatus(changed, dryRun, "MCP server", mcpPath)

	if editor == "claude" {
		changed, err = removeHookConfig(hookPath, dryRun)
		if err != nil {
			return fmt.Errorf("legacy hook cleanup: %w", err)
		}
		// Only mention the hook on uninstall when one was actually present.
		if changed {
			printRemoveStatus(changed, dryRun, "legacy PreToolUse hook", hookPath)
		}
	}

	// Remove slash commands.
	for name := range slashCommands {
		cmdDir := slashCommandDir(home, editor)
		if cmdDir == "" {
			break
		}
		fullPath := filepath.Join(cmdDir, name+".md")
		changed, err = removeSlashCommand(fullPath, dryRun)
		if err != nil {
			return fmt.Errorf("slash command %s: %w", name, err)
		}
		printRemoveStatus(changed, dryRun, "/"+name+" command", fullPath)
	}

	// Strip the agent instruction block from the per-project file too if
	// the user passes --instructions on uninstall.
	if instructions {
		if err := uninstallInstructionFile(editor, dryRun, written); err != nil {
			return err
		}
	}

	if dryRun {
		fmt.Println("\nDry run complete. No files were modified.")
	} else {
		fmt.Println("\nUninstall complete.")
	}
	return nil
}

// installInstructionFile writes the contextception agent-instruction block
// into the current project's per-editor instruction file (CLAUDE.md,
// AGENTS.md, .cursor/rules/contextception.mdc, etc.). It uses begin/end
// markers so user-authored content around the block is preserved.
//
// `written` is shared across editors in a single setup run so that editors
// sharing a destination (e.g. opencode + warp both use AGENTS.md) skip the
// duplicate write/print.
func installInstructionFile(editor string, dryRun bool, written map[string]bool) error {
	target, ok := instructionTargetForEditor(editor)
	if !ok {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	abs, err := filepath.Abs(filepath.Join(cwd, target.relPath))
	if err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	if written[abs] {
		return nil
	}
	written[abs] = true

	changed, err := ensureInstructionFile(abs, agentsMDBody, target.frontmatter, dryRun)
	if err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	printStatus(changed, dryRun, "agent instructions", abs)
	return nil
}

// uninstallInstructionFile is the inverse of installInstructionFile — it
// strips the contextception block from the per-editor instruction file.
func uninstallInstructionFile(editor string, dryRun bool, written map[string]bool) error {
	target, ok := instructionTargetForEditor(editor)
	if !ok {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	abs, err := filepath.Abs(filepath.Join(cwd, target.relPath))
	if err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	if written[abs] {
		return nil
	}
	written[abs] = true

	changed, err := removeInstructionBlock(abs, dryRun)
	if err != nil {
		return fmt.Errorf("instructions: %w", err)
	}
	if changed {
		printRemoveStatus(changed, dryRun, "agent instructions", abs)
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

// ensureOpencodeMCPConfig adds the contextception entry to opencode.json.
// opencode uses a different schema from claude/cursor/windsurf:
//
//	{
//	  "$schema": "https://opencode.ai/config.json",
//	  "mcp": {
//	    "contextception": {
//	      "type": "local",
//	      "command": ["contextception", "mcp"],
//	      "enabled": true
//	    }
//	  }
//	}
func ensureOpencodeMCPConfig(configPath string, dryRun bool) (bool, error) {
	data, err := readOrCreateJSON(configPath)
	if err != nil {
		return false, err
	}

	if gjson.GetBytes(data, "mcp.contextception").Exists() {
		return false, nil
	}

	// Add the $schema pointer if the file is brand new (helps editors
	// give validation/autocomplete in opencode.json).
	if !gjson.GetBytes(data, `\$schema`).Exists() {
		data, err = sjson.SetBytes(data, `\$schema`, "https://opencode.ai/config.json")
		if err != nil {
			return false, fmt.Errorf("setting $schema: %w", err)
		}
	}

	entry := map[string]any{
		"type":    "local",
		"command": []string{"contextception", "mcp"},
		"enabled": true,
	}
	data, err = sjson.SetBytes(data, "mcp.contextception", entry)
	if err != nil {
		return false, fmt.Errorf("setting mcp.contextception: %w", err)
	}

	if dryRun {
		return true, nil
	}
	return true, writeJSON(configPath, data)
}

// removeOpencodeMCPConfig deletes the contextception entry from opencode.json.
func removeOpencodeMCPConfig(configPath string, dryRun bool) (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !gjson.GetBytes(data, "mcp.contextception").Exists() {
		return false, nil
	}
	data, err = sjson.DeleteBytes(data, "mcp.contextception")
	if err != nil {
		return false, fmt.Errorf("deleting mcp.contextception: %w", err)
	}
	if dryRun {
		return true, nil
	}
	return true, writeJSON(configPath, data)
}

// ensureVscodeMCPConfig adds the contextception entry to VSCode's user-level
// mcp.json. The VSCode/Copilot Chat schema uses `servers` rather than
// `mcpServers`:
//
//	{
//	  "servers": {
//	    "contextception": {
//	      "type": "stdio",
//	      "command": "contextception",
//	      "args": ["mcp"]
//	    }
//	  }
//	}
func ensureVscodeMCPConfig(configPath string, dryRun bool) (bool, error) {
	data, err := readOrCreateJSON(configPath)
	if err != nil {
		return false, err
	}

	if gjson.GetBytes(data, "servers.contextception").Exists() {
		return false, nil
	}

	entry := map[string]any{
		"type":    "stdio",
		"command": "contextception",
		"args":    []string{"mcp"},
	}
	data, err = sjson.SetBytes(data, "servers.contextception", entry)
	if err != nil {
		return false, fmt.Errorf("setting servers.contextception: %w", err)
	}

	if dryRun {
		return true, nil
	}
	return true, writeJSON(configPath, data)
}

// removeVscodeMCPConfig deletes the contextception entry from VSCode's mcp.json.
func removeVscodeMCPConfig(configPath string, dryRun bool) (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !gjson.GetBytes(data, "servers.contextception").Exists() {
		return false, nil
	}
	data, err = sjson.DeleteBytes(data, "servers.contextception")
	if err != nil {
		return false, fmt.Errorf("deleting servers.contextception: %w", err)
	}
	if dryRun {
		return true, nil
	}
	return true, writeJSON(configPath, data)
}

// removeHookConfig removes contextception hook entries from PreToolUse.
// Used for both --uninstall and as an upgrade-path migration in runInstall:
// earlier versions of contextception shipped a PreToolUse hook that injected
// dependency context on every Edit/Write. The hook proved too noisy in
// practice and was removed; this function strips any leftover entries.
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
			if strings.Contains(cmd, "contextception") {
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

// editorPaths returns the MCP config path and hooks config path for the editor.
// Both paths may be empty for editors that don't have a writable config file
// (e.g. warp configures MCP through its UI).
func editorPaths(home, editor string) (mcpPath, hookPath string, err error) {
	switch editor {
	case "claude":
		mcpPath = filepath.Join(home, ".claude.json")
		hookPath = filepath.Join(home, ".claude", "settings.json")
	case "cursor":
		mcpPath = filepath.Join(home, ".cursor", "mcp.json")
	case "windsurf":
		mcpPath = filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	case "opencode":
		mcpPath = filepath.Join(home, ".config", "opencode", "opencode.json")
	case "vscode":
		mcpPath = vscodeUserMCPPath(home)
		if mcpPath == "" {
			return "", "", fmt.Errorf("vscode integration not supported on %s", runtime.GOOS)
		}
	case "warp":
		// Warp configures MCP through its UI; runInstall handles this case
		// without writing files.
	default:
		return "", "", fmt.Errorf("unsupported editor: %s (supported: claude, cursor, windsurf, opencode, vscode, warp)", editor)
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
	case "opencode":
		return "opencode"
	case "vscode":
		return "VSCode"
	case "warp":
		return "Warp"
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

// slashCommandDir returns the directory where slash command files are installed.
// Returns "" if the editor doesn't support slash commands.
func slashCommandDir(home, editor string) string {
	switch editor {
	case "claude":
		return filepath.Join(home, ".claude", "commands")
	case "cursor":
		return filepath.Join(home, ".cursor", "rules")
	case "windsurf":
		return filepath.Join(home, ".windsurf", "rules")
	default:
		return ""
	}
}

// slashCommandPath returns the full path for a named command (used by tests).
func slashCommandPath(home, editor string) string {
	dir := slashCommandDir(home, editor)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "pr-risk.md")
}

// versionStamp returns the content with a version comment prepended.
// This ensures command files update when the binary version changes.
func versionStamp(content, ver string) string {
	if ver == "" || ver == "dev" {
		return content
	}
	return "<!-- contextception " + ver + " -->\n" + content
}

// ensureSlashCommand writes a command file if it doesn't exist or has changed.
func ensureSlashCommand(cmdPath, content string, dryRun bool) (bool, error) {
	existing, err := os.ReadFile(cmdPath)
	if err == nil && string(existing) == content {
		return false, nil // already installed and up to date
	}

	if dryRun {
		return true, nil
	}

	dir := filepath.Dir(cmdPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("creating directory %s: %w", dir, err)
	}
	return true, os.WriteFile(cmdPath, []byte(content), 0o644)
}

// removeSlashCommand removes the /pr-risk command file.
func removeSlashCommand(cmdPath string, dryRun bool) (bool, error) {
	if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
		return false, nil
	}

	// Only remove if it's ours (contains "contextception").
	data, err := os.ReadFile(cmdPath)
	if err != nil {
		return false, err
	}
	if !strings.Contains(string(data), "contextception") {
		return false, nil // not our file
	}

	if dryRun {
		return true, nil
	}
	return true, os.Remove(cmdPath)
}

// slashCommands maps command names to their embedded content.
// Each is installed as a Claude Code command, Cursor rule, or Windsurf rule.
var slashCommands = map[string]string{
	"pr-risk": prRiskCommand,
	"pr-fix":  prFixCommand,
}

// prRiskCommand is the embedded /pr-risk slash command content.
const prRiskCommand = `Run a risk analysis on the current branch and present an actionable review.

## Instructions

Run this command to get the raw risk data:

` + "```" + `
contextception analyze-change --json
` + "```" + `

If contextception is not installed or the command fails, tell the user to install it and stop.

Parse the JSON output and present a review following this structure:

### 1. One-Sentence Verdict

Start with a single sentence answering: "Is this PR safe to merge?"

Based on the aggregate_risk.score:
- 0-20: "This PR is low risk — safe to merge with standard review."
- 21-50: "This PR has moderate risk — a few files deserve closer attention."
- 51-75: "This PR is high risk — targeted testing recommended before merging."
- 76-100: "This PR has critical risk — careful review required, regressions likely without it."

### 2. Files That Need Attention

Only show files in REVIEW, TEST, or CRITICAL tiers. Skip SAFE files entirely.

For each file worth discussing, explain in plain language:
- What it does (infer from path/name)
- Why it's flagged (translate risk_factors — don't say "fragility 0.83", say "this file imports many packages but few things depend on it, making it sensitive to upstream changes")
- What to check (specific advice)

Group related files. If 5 CLI handlers share the same risk profile, summarize as a group.

### 3. What's Safe to Skip

One line: "N files are low risk (new files with tests, documentation, etc.) — no special attention needed."

### 4. Test Coverage

If test_coverage_ratio < 0.5, flag it.
Name the most important untested files from test_gaps (not all of them).
Present test_suggestions as actionable items.

### 5. Offer Next Steps

End with 2-3 concrete options:
- "Want me to look at [highest-risk file] more closely?"
- "Should I check [file with test gap] for missing test coverage?"
- "Should I run the tests to verify nothing broke?"

Only offer options that make sense for the specific results.

### Rules

- Never show raw JSON, risk scores, or factor lists
- Never say "risk_score", "risk_tier", "risk_factors", or "fragility"
- Keep the entire review under 40 lines
- Use basenames when unambiguous
- If the PR is all SAFE files, keep the review to 3 lines
`

// prFixCommand is the embedded /pr-fix slash command content.
const prFixCommand = `Analyze a PR or current branch for risk, then build a plan to fix every issue found.

Use: /pr-fix (current branch vs main) or /pr-fix 123 (open PR #123)

## Instructions

### Step 1: Get the diff

If the user provided a PR number as an argument:

` + "```" + `
gh pr view <number> --json baseRefName,headRefName,url,title
` + "```" + `

Extract the base and head refs, then:

` + "```" + `
contextception analyze-change --json <baseRef>..<headRef>
` + "```" + `

If no PR number was provided (pre-PR mode on current branch):

` + "```" + `
contextception analyze-change --json
` + "```" + `

If contextception is not installed, tell the user to install it and stop.

### Step 2: Brief Risk Assessment

Present a short assessment (under 15 lines) following the /pr-risk format:
1. One-sentence verdict
2. Files needing attention (names + plain-language reasons)
3. Safe file count
4. Test coverage note

Keep this brief — the plan is the main event.

### Step 3: Build the Fix Plan

Analyze every issue and create a concrete, ordered plan to resolve them.

For each category, generate specific tasks:

**Test gaps** (from test_gaps and test_suggestions):
- For each high-risk untested file, name the test file to create and what to test
- Example: "Create handler_test.go — test the analytics wiring by verifying RecordUsage is called correctly"

**Coupling risks** (from coupling where direction is "mutual" or circular_deps):
- Suggest integration tests or dependency direction fixes
- Example: "server.go and tools.go have a mutual dependency — add a test covering the full MCP request path"

**High-fragility files** (risk_factors containing "fragility" in TEST/CRITICAL tiers):
- Suggest reducing imports or adding interface boundaries
- Only for TEST/CRITICAL — don't over-optimize REVIEW files

**Foundation file coverage** (files with 5+ importers and no direct tests):
- Prioritize testing these — a bug here affects many consumers
- Example: "history.go has 10 importers — add migration tests for backward compatibility"

### Step 4: Order by Impact

1. Tests for CRITICAL/TEST tier files (highest regression prevention value)
2. Tests for foundation files (high importer count)
3. Coupling fixes (mutual/circular deps)
4. Everything else

### Step 5: Present the Plan

` + "```" + `
## Fix Plan (N tasks)

Addressing these will bring the PR from [CURRENT LEVEL] toward lower risk.

1. **[File]**: [What to do and why]
2. **[File]**: [What to do and why]
...
` + "```" + `

### Step 6: Execute on Request

After presenting the plan, ask:

"Want me to start working through this plan? I'll tackle them in order."

If yes, work through tasks one at a time:
- Before each: say what you're about to do
- After each: re-run contextception analyze-change --json and verify improvement
- Report: "Task 1/N complete — risk moved from [LEVEL] toward [LEVEL]"

### Rules

- Never show raw JSON or numeric scores
- Translate all technical factors into plain language
- Each task must be specific enough to execute immediately
- Don't suggest fixes for SAFE files
- Don't suggest architectural refactors for REVIEW files
- If all files are SAFE, say "This PR looks clean — no fixes needed" and stop
- In PR mode, mention the PR title and link for context
`

// setupVersionFile returns the path to the file that tracks which version last ran setup.
func setupVersionFile() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "contextception", "setup-version")
}

// writeSetupVersion records the current binary version as the last setup version.
func writeSetupVersion(ver string) {
	path := setupVersionFile()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755) //nolint:errcheck
	os.WriteFile(path, []byte(ver), 0o644) //nolint:errcheck
}

// readSetupVersion returns the version that last ran setup, or "" if unknown.
func readSetupVersion() string {
	path := setupVersionFile()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// CheckSetupFreshness compares the current binary version to the last setup version.
// Returns a user-facing message if setup should be re-run, or "" if up to date.
func CheckSetupFreshness() string {
	current := version.Version
	if current == "" || current == "dev" {
		return ""
	}
	last := readSetupVersion()
	if last == "" {
		// Never run setup — don't nag, they may not use editor integrations.
		return ""
	}
	if last == current {
		return ""
	}
	return fmt.Sprintf(
		"contextception updated (%s → %s). Run 'contextception setup' to update editor commands.",
		last, current,
	)
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
