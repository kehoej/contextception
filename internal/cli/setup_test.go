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

	changed, err := ensureMCPConfig(mcpPath, "claude", false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true for fresh install")
	}

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

	if _, err := ensureMCPConfig(mcpPath, "claude", false); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureMCPConfig(mcpPath, "claude", false)
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

func TestSetup_DryRun(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")

	changed, err := ensureMCPConfig(mcpPath, "claude", true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true in dry run")
	}

	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Fatal("dry run should not create files")
	}
}

func TestSetup_Uninstall_Claude(t *testing.T) {
	home := t.TempDir()
	mcpPath := filepath.Join(home, ".claude.json")
	hookPath := filepath.Join(home, ".claude", "settings.json")

	// Install MCP, plus seed a legacy hook entry to exercise both removal paths.
	if _, err := ensureMCPConfig(mcpPath, "claude", false); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "contextception hook-context"}]}
    ]
  }
}`
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

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
		t.Fatal("expected changed=true on uninstall when legacy hook present")
	}

	data, _ := os.ReadFile(mcpPath)
	if gjson.GetBytes(data, "mcpServers.contextception").Exists() {
		t.Fatal("contextception MCP entry should be removed")
	}

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

// TestSetup_Migration_RemovesLegacyHook verifies the upgrade path: a user who
// previously ran an older `contextception setup` (which wrote a `contextception
// hook-context` PreToolUse entry into ~/.claude/settings.json) sees that entry
// silently removed by the next `setup` run, while unrelated hooks survive.
func TestSetup_Migration_RemovesLegacyHook(t *testing.T) {
	home := t.TempDir()
	hookPath := filepath.Join(home, ".claude", "settings.json")

	legacy := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "contextception hook-context"}]},
      {"matcher": "Write", "hooks": [{"type": "command", "command": "contextception hook-context"}]},
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "my-custom-hook.sh"}]}
    ]
  }
}`
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := removeHookConfig(hookPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true when legacy hook entries are removed")
	}

	data, _ := os.ReadFile(hookPath)
	entries := gjson.GetBytes(data, "hooks.PreToolUse").Array()
	if len(entries) != 1 {
		t.Fatalf("expected only the unrelated Bash hook to remain, got %d entries", len(entries))
	}
	if entries[0].Get("matcher").String() != "Bash" {
		t.Fatal("non-contextception Bash hook should be preserved")
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

func TestSetup_Migration_RemovesPreContextHookCheck(t *testing.T) {
	// Older versions shipped `contextception hook-check` (a reminder-only hook).
	// `removeHookConfig` matches the substring "contextception" so it should
	// strip those entries too.
	home := t.TempDir()
	hookPath := filepath.Join(home, ".claude", "settings.json")

	existing := `{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": "contextception hook-check"}]},
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "my-other-hook.sh"}]}
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

func TestSupportedExtension_Positive(t *testing.T) {
	for _, ext := range []string{".py", ".go", ".ts", ".java", ".rs", ".cs"} {
		if !isSupportedExtension(ext) {
			t.Fatalf("%s should be supported", ext)
		}
	}
}

func TestSupportedExtension_Negative(t *testing.T) {
	for _, ext := range []string{".md", ".json", ".yaml", ""} {
		if isSupportedExtension(ext) {
			t.Fatalf("%s should not be supported", ext)
		}
	}
}

func TestSetup_UnsupportedEditor(t *testing.T) {
	_, _, err := editorPaths("/home/test", "sublime")
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

	// Add opencode.
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755)
	editors = detectInstalledEditors(home)
	if !contains(editors, "opencode") {
		t.Fatalf("expected opencode in %v", editors)
	}

	// Add VSCode (per-platform path).
	if dir := vscodeDataDir(home); dir != "" {
		os.MkdirAll(dir, 0o755)
		editors = detectInstalledEditors(home)
		if !contains(editors, "vscode") {
			t.Fatalf("expected vscode in %v", editors)
		}
	}

	// Add Warp.
	os.MkdirAll(filepath.Join(home, ".warp"), 0o755)
	editors = detectInstalledEditors(home)
	if !contains(editors, "warp") {
		t.Fatalf("expected warp in %v", editors)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func TestSetup_FreshInstall_Opencode(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	changed, err := ensureOpencodeMCPConfig(configPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true for fresh install")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := gjson.GetBytes(data, "mcp.contextception.type").String(); got != "local" {
		t.Fatalf(`expected mcp.contextception.type "local", got %q`, got)
	}
	cmdArr := gjson.GetBytes(data, "mcp.contextception.command").Array()
	if len(cmdArr) != 2 || cmdArr[0].String() != "contextception" || cmdArr[1].String() != "mcp" {
		t.Fatalf("expected command=[contextception, mcp], got %v", cmdArr)
	}
	if !gjson.GetBytes(data, "mcp.contextception.enabled").Bool() {
		t.Fatal("expected enabled=true")
	}
	if got := gjson.GetBytes(data, `\$schema`).String(); got != "https://opencode.ai/config.json" {
		t.Fatalf("expected $schema set to opencode schema URL, got %q", got)
	}
}

func TestSetup_Idempotent_Opencode(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	if _, err := ensureOpencodeMCPConfig(configPath, false); err != nil {
		t.Fatal(err)
	}
	changed, err := ensureOpencodeMCPConfig(configPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second run should report changed=false")
	}
}

func TestSetup_Uninstall_Opencode(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")

	if _, err := ensureOpencodeMCPConfig(configPath, false); err != nil {
		t.Fatal(err)
	}
	changed, err := removeOpencodeMCPConfig(configPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on uninstall")
	}
	data, _ := os.ReadFile(configPath)
	if gjson.GetBytes(data, "mcp.contextception").Exists() {
		t.Fatal("contextception entry should be removed")
	}
}

func TestSetup_FreshInstall_Vscode(t *testing.T) {
	home := t.TempDir()
	configPath := vscodeUserMCPPath(home)
	if configPath == "" {
		t.Skip("VSCode integration not supported on this platform")
	}

	changed, err := ensureVscodeMCPConfig(configPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true for fresh install")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := gjson.GetBytes(data, "servers.contextception.type").String(); got != "stdio" {
		t.Fatalf(`expected servers.contextception.type "stdio", got %q`, got)
	}
	if got := gjson.GetBytes(data, "servers.contextception.command").String(); got != "contextception" {
		t.Fatalf(`expected command "contextception", got %q`, got)
	}
	args := gjson.GetBytes(data, "servers.contextception.args").Array()
	if len(args) != 1 || args[0].String() != "mcp" {
		t.Fatalf("expected args [mcp], got %v", args)
	}
}

func TestSetup_Uninstall_Vscode(t *testing.T) {
	home := t.TempDir()
	configPath := vscodeUserMCPPath(home)
	if configPath == "" {
		t.Skip("VSCode integration not supported on this platform")
	}

	if _, err := ensureVscodeMCPConfig(configPath, false); err != nil {
		t.Fatal(err)
	}
	changed, err := removeVscodeMCPConfig(configPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on uninstall")
	}
}

func TestSetup_Vscode_PreservesOtherServers(t *testing.T) {
	home := t.TempDir()
	configPath := vscodeUserMCPPath(home)
	if configPath == "" {
		t.Skip("VSCode integration not supported on this platform")
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
  "servers": {
    "playwright": {
      "type": "stdio",
      "command": "npx",
      "args": ["playwright"]
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureVscodeMCPConfig(configPath, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(configPath)
	if got := gjson.GetBytes(data, "servers.playwright.command").String(); got != "npx" {
		t.Fatal("existing playwright server lost")
	}
	if got := gjson.GetBytes(data, "servers.contextception.type").String(); got != "stdio" {
		t.Fatal("contextception server not added")
	}
}

func TestSetup_EditorPaths_Warp(t *testing.T) {
	mcpPath, hookPath, err := editorPaths("/home/test", "warp")
	if err != nil {
		t.Fatalf("warp should be a valid editor: %v", err)
	}
	if mcpPath != "" || hookPath != "" {
		t.Fatalf("warp should have no file paths (UI-only); got mcp=%q hook=%q", mcpPath, hookPath)
	}
}

func TestSetup_EditorPaths_Opencode(t *testing.T) {
	mcpPath, _, err := editorPaths("/home/test", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	want := "/home/test/.config/opencode/opencode.json"
	if mcpPath != want {
		t.Fatalf("opencode mcpPath = %q, want %q", mcpPath, want)
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
