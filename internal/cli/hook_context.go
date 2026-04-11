package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/history"
	"github.com/kehoej/contextception/internal/model"
	"github.com/spf13/cobra"
)

// hookOutput is the JSON envelope expected by Claude Code PreToolUse hooks.
type hookOutput struct {
	HookSpecificOutput hookSpecific `json:"hookSpecificOutput"`
}

type hookSpecific struct {
	HookEventName      string `json:"hookEventName"`
	PermissionDecision string `json:"permissionDecision"`
	AdditionalContext  string `json:"additionalContext,omitempty"`
}

func newHookContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook-context",
		Short:  "Inject dependency context into Claude Code edits via PreToolUse hooks",
		Long:   "Reads the TOOL_INPUT environment variable, runs the analyzer, and outputs JSON with additionalContext for Claude Code. Always exits 0.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHookContext()
		},
	}
}

func runHookContext() error {
	// Parse TOOL_INPUT env var.
	input := os.Getenv("TOOL_INPUT")
	if input == "" {
		emitHookAllow("")
		return nil
	}

	var toolInput struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(input), &toolInput); err != nil {
		emitHookAllow("")
		return nil
	}
	if toolInput.FilePath == "" {
		emitHookAllow("")
		return nil
	}

	// Check for supported extension.
	ext := filepath.Ext(toolInput.FilePath)
	if ext == "" || !isSupportedExtension(ext) {
		emitHookAllow("")
		return nil
	}

	// Detect repo root.
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		emitHookAllow("")
		return nil
	}
	root := strings.TrimSpace(string(out))

	// Check if index exists.
	idxPath := db.IndexPath(root)
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		emitHookAllow("No contextception index found. Run `contextception index` to enable dependency context.")
		return nil
	}

	// Open index.
	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		emitHookAllow("")
		return nil
	}
	defer idx.Close()

	// Run migrations (fast no-op check).
	if _, err := idx.MigrateToLatest(); err != nil {
		emitHookAllow("")
		return nil
	}

	// Load config (fallback to empty).
	cfg, err := config.Load(root)
	if err != nil {
		cfg = config.Empty()
	}

	// Resolve file to repo-relative path.
	target := toolInput.FilePath
	if filepath.IsAbs(target) {
		rel, err := filepath.Rel(root, target)
		if err != nil {
			emitHookAllow("")
			return nil
		}
		target = rel
	}
	target = strings.ReplaceAll(filepath.Clean(target), string(os.PathSeparator), "/")

	// Run analyzer with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a := analyzer.New(idx, analyzer.Config{
		RepoConfig:   cfg,
		OmitExternal: true,
		RepoRoot:     root,
		Caps: analyzer.Caps{
			Mode: "hook",
		},
	})

	start := time.Now()
	resultCh := make(chan *model.AnalysisOutput, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := a.Analyze(target)
		if err != nil {
			errCh <- err
		} else {
			resultCh <- result
		}
	}()

	var output *model.AnalysisOutput
	select {
	case output = <-resultCh:
	case <-errCh:
		emitHookAllow("")
		return nil
	case <-ctx.Done():
		emitHookAllow("")
		return nil
	}
	durationMs := time.Since(start).Milliseconds()

	// Record usage analytics (best-effort, non-blocking for hook latency).
	go func() {
		if hist, hErr := history.Open(root); hErr == nil {
			entry := history.UsageEntryFromAnalysis("hook", "get_context", []string{target}, output, durationMs, "hook", 0)
			_, _ = hist.RecordUsage(entry)
			hist.Close()
		}
	}()

	// Format and emit.
	additional := formatHookContext(output)
	emitHookAllow(additional)
	return nil
}

// formatHookContext produces a compact plain-text summary of the analysis output.
func formatHookContext(output *model.AnalysisOutput) string {
	if output == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[contextception] Dependency context for %s\n", output.Subject)

	if len(output.MustRead) > 0 {
		b.WriteString("\nMust-read before editing:\n")
		for _, entry := range output.MustRead {
			b.WriteString("  ")
			b.WriteString(entry.File)
			if len(entry.Symbols) > 0 {
				fmt.Fprintf(&b, " (%s)", strings.Join(entry.Symbols, ", "))
			}
			var tags []string
			if entry.Direction != "" {
				tags = append(tags, entry.Direction)
			}
			if entry.Stable {
				tags = append(tags, "stable")
			}
			if entry.Circular {
				tags = append(tags, "circular")
			}
			if len(tags) > 0 {
				fmt.Fprintf(&b, " [%s]", strings.Join(tags, ", "))
			}
			b.WriteString("\n")
		}
	}

	// Tests (direct only).
	var directTests []string
	for _, t := range output.Tests {
		if t.Direct {
			directTests = append(directTests, t.File)
		}
	}
	if len(directTests) > 0 {
		fmt.Fprintf(&b, "\nTests: %s\n", strings.Join(directTests, ", "))
	}

	if output.BlastRadius != nil {
		fmt.Fprintf(&b, "\nBlast radius: %s", output.BlastRadius.Level)
		if output.BlastRadius.Detail != "" {
			fmt.Fprintf(&b, " (%s)", output.BlastRadius.Detail)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// emitHookAllow writes the JSON envelope to stdout.
func emitHookAllow(additionalContext string) {
	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			AdditionalContext:  additionalContext,
		},
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}
