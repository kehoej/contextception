package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentsMDInSync is a parity test: the embedded copy at
// internal/cli/templates/AGENTS.md (compiled into the binary via go:embed)
// MUST be byte-for-byte identical to integrations/AGENTS.md (the
// human-facing copy). If you edit one, edit the other.
func TestAgentsMDInSync(t *testing.T) {
	canonical, err := os.ReadFile(filepath.Join("..", "..", "integrations", "AGENTS.md"))
	if err != nil {
		t.Fatalf("could not read integrations/AGENTS.md: %v", err)
	}
	if string(canonical) != agentsMDBody {
		t.Fatalf("integrations/AGENTS.md and internal/cli/templates/AGENTS.md have drifted.\n" +
			"They must be byte-identical. Update both, or run a sync:\n" +
			"  cp integrations/AGENTS.md internal/cli/templates/AGENTS.md")
	}
}

func TestInstructionTargets(t *testing.T) {
	tests := []struct {
		editor   string
		ok       bool
		wantPath string
		wantFM   bool
	}{
		{"claude", true, "CLAUDE.md", false},
		{"cursor", true, ".cursor/rules/contextception.mdc", true},
		{"windsurf", true, ".windsurf/rules/contextception.md", false},
		{"vscode", true, ".github/copilot-instructions.md", false},
		{"opencode", true, "AGENTS.md", false},
		{"warp", true, "AGENTS.md", false},
		{"sublime", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.editor, func(t *testing.T) {
			target, ok := instructionTargetForEditor(tt.editor)
			if ok != tt.ok {
				t.Fatalf("ok=%v, want %v", ok, tt.ok)
			}
			if !tt.ok {
				return
			}
			if filepath.ToSlash(target.relPath) != tt.wantPath {
				t.Fatalf("relPath = %q, want %q", target.relPath, tt.wantPath)
			}
			if (target.frontmatter != "") != tt.wantFM {
				t.Fatalf("frontmatter present=%v, want %v", target.frontmatter != "", tt.wantFM)
			}
		})
	}
}

func TestEnsureInstructionFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	changed, err := ensureInstructionFile(path, "hello body", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on fresh write")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), instructionMarkerBegin) {
		t.Fatal("expected begin marker in file")
	}
	if !strings.Contains(string(data), instructionMarkerEnd) {
		t.Fatal("expected end marker in file")
	}
	if !strings.Contains(string(data), "hello body") {
		t.Fatal("expected body in file")
	}
}

func TestEnsureInstructionFile_NewFile_WithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rule.mdc")

	fm := "---\nalwaysApply: true\n---\n"
	_, err := ensureInstructionFile(path, "body here", fm, false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), fm) {
		t.Fatalf("expected file to start with frontmatter, got:\n%s", string(data))
	}
	// Frontmatter is BEFORE the begin marker.
	if strings.Index(string(data), fm) >= strings.Index(string(data), instructionMarkerBegin) {
		t.Fatal("frontmatter should appear before contextception block")
	}
}

func TestEnsureInstructionFile_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	if _, err := ensureInstructionFile(path, "body", "", false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)

	changed, err := ensureInstructionFile(path, "body", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second run with identical body should report changed=false")
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Fatal("file content changed on idempotent re-run")
	}
}

func TestEnsureInstructionFile_AppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	existing := "# My project rules\n\nDon't break things.\nPrefer pairing.\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := ensureInstructionFile(path, "contextception body", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true when appending block to existing file")
	}

	data, _ := os.ReadFile(path)
	got := string(data)
	if !strings.HasPrefix(got, existing) {
		t.Fatalf("existing user content not preserved at top:\n%s", got)
	}
	if !strings.Contains(got, "contextception body") {
		t.Fatal("expected appended body")
	}
	if !strings.Contains(got, instructionMarkerBegin) || !strings.Contains(got, instructionMarkerEnd) {
		t.Fatal("expected markers in appended block")
	}
}

func TestEnsureInstructionFile_ReplacesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	existing := "# Project header\n\n" +
		instructionMarkerBegin + "\nOLD STALE BODY\n" + instructionMarkerEnd + "\n\n" +
		"## Trailing user content\n\nshould remain\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureInstructionFile(path, "fresh body", "", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)

	if strings.Contains(got, "OLD STALE BODY") {
		t.Fatal("old block content should have been replaced")
	}
	if !strings.Contains(got, "fresh body") {
		t.Fatal("new block content missing")
	}
	if !strings.Contains(got, "# Project header") {
		t.Fatal("user header above block was destroyed")
	}
	if !strings.Contains(got, "## Trailing user content") {
		t.Fatal("user content below block was destroyed")
	}
	if !strings.Contains(got, "should remain") {
		t.Fatal("user trailing content was destroyed")
	}
}

func TestEnsureInstructionFile_DryRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	changed, err := ensureInstructionFile(path, "body", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("dry run should report changed=true")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("dry run should not create the file")
	}
}

func TestRemoveInstructionBlock_StripsBlockOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	original := "# My rules\n\nDon't break things.\n\n" +
		instructionMarkerBegin + "\ncontextception body\n" + instructionMarkerEnd + "\n\n" +
		"## More user content\n\nstill mine\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := removeInstructionBlock(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true when stripping block")
	}

	data, _ := os.ReadFile(path)
	got := string(data)
	if strings.Contains(got, instructionMarkerBegin) {
		t.Fatal("begin marker should be gone")
	}
	if strings.Contains(got, "contextception body") {
		t.Fatal("body should be gone")
	}
	if !strings.Contains(got, "Don't break things.") {
		t.Fatal("user content above block was destroyed")
	}
	if !strings.Contains(got, "still mine") {
		t.Fatal("user content below block was destroyed")
	}
}

func TestRemoveInstructionBlock_DeletesFileWhenOnlyBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	if _, err := ensureInstructionFile(path, "body", "", false); err != nil {
		t.Fatal(err)
	}
	// File now contains only the contextception block.

	changed, err := removeInstructionBlock(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should be removed when the block was its only content")
	}
}

func TestRemoveInstructionBlock_NoMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	original := "# User-only content\n\nNothing to do here.\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := removeInstructionBlock(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when no markers present")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("user content modified despite no markers being present")
	}
}

func TestRemoveInstructionBlock_Missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	changed, err := removeInstructionBlock(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false when file does not exist")
	}
}

func TestRemoveInstructionBlock_MalformedMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	// Begin without matching end.
	bad := "Some content\n" + instructionMarkerBegin + "\nincomplete...\n"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := removeInstructionBlock(path, false); err == nil {
		t.Fatal("expected error on malformed markers")
	}
}
