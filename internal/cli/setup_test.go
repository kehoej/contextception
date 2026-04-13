package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestSetup_FreshInstall_Claude(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")
	hookPath := filepath.Join(home, ".claude", "settings.json")

	changed, err := ensureMCPConfig(mcpPath, "claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true for fresh install")
	}

	changed, err = ensureHookConfig(hookPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true for fresh hook install")
	}

	// Verify MCP config.
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := gjson.GetBytes(data, "mcpServers.contextception.command").String()
	if cmd != "contextception" {
		t.Fatalf("expected command 'contextception', got %q", cmd)
	}
	args := gjson.GetBytes(data, "mcpServers.contextception.args").Array()
	if len(args) != 1 || args[0].String() != "mcp" {
		t.Fatalf("expected args ['mcp'], got %v", args)
	}
	// Claude should NOT have transportType.
	if gjson.GetBytes(data, "mcpServers.contextception.transportType").Exists() {
		t.Fatal("claude config should not have transportType")
	}

	// Verify hook config.
	data, err = os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	entries := gjson.GetBytes(data, "hooks.PreToolUse").Array()
	if len(entries) != 2 {
		t.Fatalf("expected 2 PreToolUse entries, got %d", len(entries))
	}
	matchers := make(map[string]bool)
	for _, e := range entries {
		matchers[e.Get("matcher").String()] = true
		hookCmd := e.Get("hooks.0.command").String()
		if hookCmd != "contextception hook-context" {
			t.Fatalf("expected hook command 'contextception hook-context', got %q", hookCmd)
		}
	}
	if !matchers["Edit"] || !matchers["Write"] {
		t.Fatalf("expected Edit and Write matchers, got %v", matchers)
	}
}

func TestSetup_FreshInstall_Cursor(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".cursor", "mcp.json")

	changed, err := ensureMCPConfig(mcpPath, "cursor", false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	tp := gjson.GetBytes(data, "mcpServers.contextception.transportType").String()
	if tp != "stdio" {
		t.Fatalf("cursor config should have transportType=stdio, got %q", tp)
	}
}

func TestSetup_FreshInstall_Windsurf(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")

	changed, err := ensureMCPConfig(mcpPath, "windsurf", false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	tp := gjson.GetBytes(data, "mcpServers.contextception.transportType").String()
	if tp != "stdio" {
		t.Fatalf("windsurf config should have transportType=stdio, got %q", tp)
	}
}

func TestSetup_Idempotent(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")
	hookPath := filepath.Join(home, ".claude", "settings.json")

	// First run.
	if _, err := ensureMCPConfig(mcpPath, "claude", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureHookConfig(hookPath, false); err != nil {
		t.Fatal(err)
	}

	// Second run should report no changes.
	changed, err := ensureMCPConfig(mcpPath, "claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false on second run")
	}

	changed, err = ensureHookConfig(hookPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false on second run")
	}
}

func TestSetup_PreservesExistingConfig(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")

	// Write existing config with another MCP server.
	existing := `{
  "numStartups": 42,
  "mcpServers": {
    "playwright": {
      "type": "stdio",
      "command": "npx",
      "args": ["playwright"]
    }
  }
}`
	if err := os.WriteFile(mcpPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureMCPConfig(mcpPath, "claude", false); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}

	// Original server should still exist.
	pw := gjson.GetBytes(data, "mcpServers.playwright.command").String()
	if pw != "npx" {
		t.Fatalf("existing playwright server lost, got %q", pw)
	}

	// New server should exist.
	cc := gjson.GetBytes(data, "mcpServers.contextception.command").String()
	if cc != "contextception" {
		t.Fatalf("contextception server not added, got %q", cc)
	}

	// Other top-level keys preserved.
	num := gjson.GetBytes(data, "numStartups").Int()
	if num != 42 {
		t.Fatalf("numStartups not preserved, got %d", num)
	}
}

func TestSetup_PreservesExistingHooks(t *testing.T) {
	home := t.TempDir()
	hookPath := filepath.Join(home, ".claude", "settings.json")

	existing := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "my-custom-hook.sh"}]
      }
    ]
  }
}`
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureHookConfig(hookPath, false); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}

	entries := gjson.GetBytes(data, "hooks.PreToolUse").Array()
	// Should have original Bash + new Edit + Write = 3.
	if len(entries) != 3 {
		t.Fatalf("expected 3 PreToolUse entries, got %d", len(entries))
	}

	// Original Bash hook should be first.
	if entries[0].Get("matcher").String() != "Bash" {
		t.Fatal("existing Bash hook not preserved")
	}
}

func TestSetup_DryRun(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")
	hookPath := filepath.Join(home, ".claude", "settings.json")

	changed, err := ensureMCPConfig(mcpPath, "claude", true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true in dry run")
	}

	// File should NOT exist.
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Fatal("dry run should not create files")
	}

	changed, err = ensureHookConfig(hookPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true in dry run")
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatal("dry run should not create files")
	}
}

func TestSetup_Uninstall_Claude(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")
	hookPath := filepath.Join(home, ".claude", "settings.json")

	// Install first.
	if _, err := ensureMCPConfig(mcpPath, "claude", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureHookConfig(hookPath, false); err != nil {
		t.Fatal(err)
	}

	// Uninstall.
	changed, err := removeMCPConfig(mcpPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on uninstall")
	}

	changed, err = removeHookConfig(hookPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on uninstall")
	}

	// Verify MCP entry removed.
	data, _ := os.ReadFile(mcpPath)
	if gjson.GetBytes(data, "mcpServers.contextception").Exists() {
		t.Fatal("contextception MCP entry should be removed")
	}

	// Verify hook entries removed.
	data, _ = os.ReadFile(hookPath)
	entries := gjson.GetBytes(data, "hooks.PreToolUse").Array()
	for _, e := range entries {
		hooks := e.Get("hooks").Array()
		for _, h := range hooks {
			if strings.Contains(h.Get("command").String(), "contextception") {
				t.Fatal("contextception hook entry should be removed")
			}
		}
	}
}

func TestSetup_Uninstall_Cursor(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".cursor", "mcp.json")

	// Install, then uninstall.
	if _, err := ensureMCPConfig(mcpPath, "cursor", false); err != nil {
		t.Fatal(err)
	}

	changed, err := removeMCPConfig(mcpPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	data, _ := os.ReadFile(mcpPath)
	if gjson.GetBytes(data, "mcpServers.contextception").Exists() {
		t.Fatal("contextception should be removed")
	}
}

func TestSetup_Uninstall_NothingInstalled(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")
	hookPath := filepath.Join(home, ".claude", "settings.json")

	changed, err := removeMCPConfig(mcpPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when nothing installed")
	}

	changed, err = removeHookConfig(hookPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when nothing installed")
	}
}

func TestSetup_InvalidJSON(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")

	// Write corrupt JSON.
	if err := os.WriteFile(mcpPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ensureMCPConfig(mcpPath, "claude", false)
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestSetup_RealisticConfig(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")

	// Simulate a realistic multi-key config file.
	config := `{
  "numStartups": 912,
  "installMethod": "native",
  "autoUpdates": false,
  "tipsHistory": {
    "new-user-warmup": 7,
    "theme-command": 894
  },
  "mcpServers": {
    "jira": {
      "command": "npx",
      "args": ["-y", "@atlassian-dc-mcp/jira"]
    }
  }
}`
	if err := os.WriteFile(mcpPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureMCPConfig(mcpPath, "claude", false); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatal(err)
	}

	// All original keys should be preserved.
	if gjson.GetBytes(data, "numStartups").Int() != 912 {
		t.Fatal("numStartups not preserved")
	}
	if gjson.GetBytes(data, "installMethod").String() != "native" {
		t.Fatal("installMethod not preserved")
	}
	if !gjson.GetBytes(data, "tipsHistory").Exists() {
		t.Fatal("tipsHistory not preserved")
	}
	if !gjson.GetBytes(data, "mcpServers.jira").Exists() {
		t.Fatal("jira MCP server not preserved")
	}
	if !gjson.GetBytes(data, "mcpServers.contextception").Exists() {
		t.Fatal("contextception not added")
	}

	// Key ordering should be preserved: numStartups should appear before mcpServers.
	str := string(data)
	numIdx := strings.Index(str, "numStartups")
	mcpIdx := strings.Index(str, "mcpServers")
	if numIdx > mcpIdx {
		t.Fatal("key ordering not preserved: numStartups should come before mcpServers")
	}
}

func TestSetup_Uninstall_RemovesOldHookScript(t *testing.T) {
	// Verify uninstall also removes entries pointing to the old
	// contextception-remind.sh script.
	home := t.TempDir()
	hookPath := filepath.Join(home, ".claude", "settings.json")

	existing := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit",
        "hooks": [{"type": "command", "command": "/home/user/.claude/hooks/contextception-remind.sh"}]
      },
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "my-other-hook.sh"}]
      }
    ]
  }
}`
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := removeHookConfig(hookPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}

	data, _ := os.ReadFile(hookPath)
	entries := gjson.GetBytes(data, "hooks.PreToolUse").Array()
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(entries))
	}
	if entries[0].Get("matcher").String() != "Bash" {
		t.Fatal("non-contextception hook should be preserved")
	}
}

func TestHookCheck_SupportedFile(t *testing.T) {
	if !isSupportedExtension(".py") {
		t.Fatal(".py should be supported")
	}
	if !isSupportedExtension(".go") {
		t.Fatal(".go should be supported")
	}
	if !isSupportedExtension(".ts") {
		t.Fatal(".ts should be supported")
	}
	if !isSupportedExtension(".java") {
		t.Fatal(".java should be supported")
	}
	if !isSupportedExtension(".rs") {
		t.Fatal(".rs should be supported")
	}
}

func TestHookCheck_UnsupportedFile(t *testing.T) {
	if isSupportedExtension(".md") {
		t.Fatal(".md should not be supported")
	}
	if isSupportedExtension(".json") {
		t.Fatal(".json should not be supported")
	}
	if isSupportedExtension(".yaml") {
		t.Fatal(".yaml should not be supported")
	}
	if isSupportedExtension("") {
		t.Fatal("empty extension should not be supported")
	}
}

func TestSetup_UpgradesOldHookCheck(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-populate with old hook-check entries.
	oldConfig := `{
		"hooks": {
			"PreToolUse": [
				{
					"matcher": "Edit",
					"hooks": [{"type": "command", "command": "contextception hook-check"}]
				},
				{
					"matcher": "Write",
					"hooks": [{"type": "command", "command": "contextception hook-check"}]
				}
			]
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(oldConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureHookConfig(settingsPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected hooks to be updated when upgrading from hook-check")
	}

	// Verify old entries replaced with new.
	data, _ := os.ReadFile(settingsPath)
	entries := gjson.GetBytes(data, "hooks.PreToolUse").Array()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		cmd := e.Get("hooks.0.command").String()
		if cmd != "contextception hook-context" {
			t.Fatalf("expected hook-context, got %q", cmd)
		}
	}
}

func TestSetup_SkipsWhenCurrent(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-populate with current hook-context entries.
	currentConfig := `{
		"hooks": {
			"PreToolUse": [
				{
					"matcher": "Edit",
					"hooks": [{"type": "command", "command": "contextception hook-context"}]
				},
				{
					"matcher": "Write",
					"hooks": [{"type": "command", "command": "contextception hook-context"}]
				}
			]
		}
	}`
	if err := os.WriteFile(settingsPath, []byte(currentConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureHookConfig(settingsPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("should not change when current hook is already installed")
	}
}

func TestSetup_UnsupportedEditor(t *testing.T) {
	_, _, err := editorPaths("/home/test", "vscode")
	if err == nil {
		t.Fatal("expected error for unsupported editor")
	}
	if !strings.Contains(err.Error(), "unsupported editor") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetup_SlashCommand_Install(t *testing.T) {
	home := t.TempDir()
	cmdPath := filepath.Join(home, ".claude", "commands", "pr-risk.md")

	changed, err := ensureSlashCommand(cmdPath, prRiskCommand, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true for fresh install")
	}

	// Verify file was written.
	data, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "contextception analyze-change") {
		t.Fatal("command file should contain contextception analyze-change")
	}
	if !strings.Contains(string(data), "One-Sentence Verdict") {
		t.Fatal("command file should contain review structure")
	}
}

func TestSetup_SlashCommand_Idempotent(t *testing.T) {
	home := t.TempDir()
	cmdPath := filepath.Join(home, ".claude", "commands", "pr-risk.md")

	// First install.
	_, err := ensureSlashCommand(cmdPath, prRiskCommand, false)
	if err != nil {
		t.Fatal(err)
	}

	// Second install should be no-op.
	changed, err := ensureSlashCommand(cmdPath, prRiskCommand, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false on second install")
	}
}

func TestSetup_SlashCommand_Remove(t *testing.T) {
	home := t.TempDir()
	cmdPath := filepath.Join(home, ".claude", "commands", "pr-risk.md")

	// Install first.
	_, err := ensureSlashCommand(cmdPath, prRiskCommand, false)
	if err != nil {
		t.Fatal(err)
	}

	// Remove.
	changed, err := removeSlashCommand(cmdPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true for removal")
	}

	// File should be gone.
	if _, err := os.Stat(cmdPath); !os.IsNotExist(err) {
		t.Fatal("command file should be removed")
	}
}

func TestSetup_SlashCommand_RemoveNonExistent(t *testing.T) {
	home := t.TempDir()
	cmdPath := filepath.Join(home, ".claude", "commands", "pr-risk.md")

	changed, err := removeSlashCommand(cmdPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when file doesn't exist")
	}
}

func TestSetup_SlashCommand_DryRun(t *testing.T) {
	home := t.TempDir()
	cmdPath := filepath.Join(home, ".claude", "commands", "pr-risk.md")

	changed, err := ensureSlashCommand(cmdPath, prRiskCommand, true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("dry-run should report changed=true")
	}

	// File should NOT be written.
	if _, err := os.Stat(cmdPath); !os.IsNotExist(err) {
		t.Fatal("dry-run should not write the file")
	}
}

func TestSetup_DetectEditors(t *testing.T) {
	home := t.TempDir()

	// No editors installed.
	editors := detectInstalledEditors(home)
	if len(editors) != 0 {
		t.Fatalf("expected 0 editors, got %v", editors)
	}

	// Install Claude Code.
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	editors = detectInstalledEditors(home)
	if len(editors) != 1 || editors[0] != "claude" {
		t.Fatalf("expected [claude], got %v", editors)
	}

	// Also install Cursor.
	os.MkdirAll(filepath.Join(home, ".cursor"), 0o755)
	editors = detectInstalledEditors(home)
	if len(editors) != 2 {
		t.Fatalf("expected 2 editors, got %v", editors)
	}
}

func TestSetup_SlashCommandPaths(t *testing.T) {
	tests := []struct {
		editor string
		expect string
	}{
		{"claude", ".claude/commands/pr-risk.md"},
		{"cursor", ".cursor/rules/pr-risk.md"},
		{"windsurf", ".windsurf/rules/pr-risk.md"},
	}
	for _, tt := range tests {
		got := slashCommandPath("/home/test", tt.editor)
		if !strings.HasSuffix(got, tt.expect) {
			t.Errorf("editor=%s: got %q, want suffix %q", tt.editor, got, tt.expect)
		}
	}
}
