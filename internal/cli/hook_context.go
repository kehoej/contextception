package cli

import (
	"context"
	"encoding/json"
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

	// Format and emit using compact output.
	additional := "[contextception] " + analyzer.FormatCompact(output)
	emitHookAllow(additional)
	return nil
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
