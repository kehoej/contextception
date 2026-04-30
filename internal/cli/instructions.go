package cli

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// agentsMDBody is the canonical agent-instruction snippet shipped with the
// binary. It is byte-for-byte identical to integrations/AGENTS.md (verified
// by TestAgentsMDInSync). Editing one without the other will fail the test.
//
//go:embed templates/AGENTS.md
var agentsMDBody string

// Marker fences used to delineate the contextception-managed block inside a
// user's instruction file. The setup command writes/removes only the content
// between these markers, so user-authored content around them is untouched.
const (
	instructionMarkerBegin = "<!-- contextception:begin -->"
	instructionMarkerEnd   = "<!-- contextception:end -->"
)

// instructionTarget describes where to write the agent instruction snippet
// for a given editor and any frontmatter that should be added when creating
// a new file (e.g. Cursor's `.mdc` rules need `alwaysApply: true`).
type instructionTarget struct {
	relPath     string
	frontmatter string
}

// instructionTargetForEditor returns the per-editor preferred instruction
// file path (relative to the project root) and any frontmatter to prepend
// when creating a new file. Returns ok=false for editors that have no
// instruction-file convention.
func instructionTargetForEditor(editor string) (instructionTarget, bool) {
	switch editor {
	case "claude":
		return instructionTarget{relPath: "CLAUDE.md"}, true
	case "cursor":
		return instructionTarget{
			relPath:     filepath.Join(".cursor", "rules", "contextception.mdc"),
			frontmatter: "---\ndescription: Tells Cursor when to call contextception MCP tools\nalwaysApply: true\n---\n",
		}, true
	case "windsurf":
		return instructionTarget{
			relPath: filepath.Join(".windsurf", "rules", "contextception.md"),
		}, true
	case "vscode":
		return instructionTarget{
			relPath: filepath.Join(".github", "copilot-instructions.md"),
		}, true
	case "opencode", "warp":
		return instructionTarget{relPath: "AGENTS.md"}, true
	default:
		return instructionTarget{}, false
	}
}

// renderInstructionBlock wraps the canonical body in begin/end markers.
func renderInstructionBlock(body string) string {
	return instructionMarkerBegin + "\n" + strings.TrimSpace(body) + "\n" + instructionMarkerEnd + "\n"
}

// ensureInstructionFile upserts the contextception block into the file at
// path. Behavior:
//   - File missing → create with optional frontmatter + block.
//   - File present with markers → replace block in place; leaves the rest alone.
//   - File present without markers → append block at end.
//   - Block already up to date → no-op (returns changed=false).
//
// The function is idempotent: running it twice on a file that already has
// the canonical block writes nothing the second time.
func ensureInstructionFile(path, body, frontmatter string, dryRun bool) (bool, error) {
	block := renderInstructionBlock(body)

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	current := string(raw)

	var next string
	switch {
	case current == "":
		// Brand-new file.
		if frontmatter != "" {
			next = frontmatter + "\n" + block
		} else {
			next = block
		}

	case strings.Contains(current, instructionMarkerBegin):
		// File has our block already — replace whatever is between the
		// markers with the canonical block.
		beginIdx := strings.Index(current, instructionMarkerBegin)
		endIdx := strings.Index(current, instructionMarkerEnd)
		if endIdx < 0 || endIdx < beginIdx {
			return false, fmt.Errorf("malformed contextception markers in %s — found begin without end", path)
		}
		// Replace from begin marker to (and including) the end marker.
		// We keep what was before the begin marker exactly, and what came
		// after the end marker exactly, so user content around our block is
		// preserved character-for-character.
		next = current[:beginIdx] + strings.TrimRight(block, "\n") + current[endIdx+len(instructionMarkerEnd):]
		// Make sure the file ends in a single newline.
		next = strings.TrimRight(next, "\n") + "\n"

	default:
		// File has user content but no contextception block — append.
		trimmed := strings.TrimRight(current, "\n")
		next = trimmed + "\n\n" + block
	}

	if next == current {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("creating directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}

// removeInstructionBlock strips the contextception-managed block from the
// file at path, leaving any user-authored content around it intact.
//
//   - File missing → no-op.
//   - File contains no markers → no-op.
//   - File contains markers → block is stripped along with any blank-line
//     padding that was added when the block was inserted.
//   - If the file becomes empty after removal → the file itself is deleted.
func removeInstructionBlock(path string, dryRun bool) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	current := string(raw)

	if !strings.Contains(current, instructionMarkerBegin) {
		return false, nil
	}
	beginIdx := strings.Index(current, instructionMarkerBegin)
	endIdx := strings.Index(current, instructionMarkerEnd)
	if endIdx < 0 || endIdx < beginIdx {
		return false, fmt.Errorf("malformed contextception markers in %s — found begin without end", path)
	}

	before := strings.TrimRight(current[:beginIdx], "\n \t")
	after := strings.TrimLeft(current[endIdx+len(instructionMarkerEnd):], "\n")

	var next string
	switch {
	case before == "" && after == "":
		// File contained nothing but our block — remove it entirely.
		if dryRun {
			return true, nil
		}
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("removing %s: %w", path, err)
		}
		return true, nil
	case before == "":
		next = after
	case after == "":
		next = before + "\n"
	default:
		next = before + "\n\n" + after
	}

	if next == current {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}
