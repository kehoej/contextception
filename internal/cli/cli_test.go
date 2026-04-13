package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/indexer"
	"github.com/kehoej/contextception/internal/model"
)

// --- Test helpers ---

// createTestRepo creates a temp directory with Python files suitable for indexing.
func createTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"myapp/__init__.py": "",
		"myapp/main.py": `from .models import User
from .utils import helper
`,
		"myapp/models.py": `from .base import BaseModel
`,
		"myapp/utils.py": `import json
`,
		"myapp/base.py":       "# no imports\n",
		"tests/test_main.py":  "from myapp.main import something\n",
		"tests/test_utils.py": "from myapp.utils import helper\n",
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// buildTestIndex creates a repo, builds the index, and returns the repo root.
func buildTestIndex(t *testing.T) string {
	t.Helper()
	root := createTestRepo(t)

	idxPath := db.IndexPath(root)
	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	if _, err := idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: root,
		Index:    idx,
		Verbose:  false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ix.Run(); err != nil {
		t.Fatal(err)
	}

	return root
}

// executeCmd creates a fresh root command, sets args, and executes it.
// Returns the error from execution. Output goes to os.Stdout/os.Stderr since
// the command handlers use fmt.Printf directly.
func executeCmd(args ...string) error {
	cmd := NewRootCmd()
	// Silence usage/error output from cobra itself.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	return cmd.Execute()
}

// executeCmdCaptureCobraOut captures cobra's own output (help, version).
func executeCmdCaptureCobraOut(args ...string) (string, error) {
	cmd := NewRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

// --- Root command tests ---

func TestRootCmd_NoArgs(t *testing.T) {
	out, err := executeCmdCaptureCobraOut("--repo", t.TempDir())
	if err != nil {
		t.Fatalf("root command should not error with no args: %v", err)
	}
	if !strings.Contains(out, "contextception") {
		t.Errorf("root output should contain 'contextception', got: %s", out)
	}
}

func TestRootCmd_Version(t *testing.T) {
	out, err := executeCmdCaptureCobraOut("--version")
	if err != nil {
		t.Fatalf("--version should not error: %v", err)
	}
	if !strings.Contains(out, "contextception") {
		t.Errorf("version output should contain 'contextception', got: %s", out)
	}
}

func TestRootCmd_UnknownSubcommand(t *testing.T) {
	err := executeCmd("--repo", t.TempDir(), "nonexistent-subcommand")
	if err == nil {
		t.Fatal("unknown subcommand should error")
	}
}

// --- Index command tests ---

func TestIndexCmd_HappyPath(t *testing.T) {
	root := createTestRepo(t)
	err := executeCmd("--repo", root, "index")
	if err != nil {
		t.Fatalf("index command failed: %v", err)
	}

	// Verify the index file was created.
	idxPath := db.IndexPath(root)
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		t.Errorf("index file should exist at %s", idxPath)
	}

	// Verify the index has data.
	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	fileCount, _ := idx.FileCount()
	if fileCount == 0 {
		t.Error("index should have files after indexing")
	}
}

func TestIndexCmd_EmptyRepo(t *testing.T) {
	root := t.TempDir()
	err := executeCmd("--repo", root, "index")
	if err != nil {
		t.Fatalf("index on empty repo should not error: %v", err)
	}
}

func TestIndexCmd_Idempotent(t *testing.T) {
	root := createTestRepo(t)

	// Run index twice — second run should succeed (incremental).
	if err := executeCmd("--repo", root, "index"); err != nil {
		t.Fatalf("first index failed: %v", err)
	}
	if err := executeCmd("--repo", root, "index"); err != nil {
		t.Fatalf("second index failed: %v", err)
	}
}

// --- Reindex command tests ---

func TestReindexCmd_HappyPath(t *testing.T) {
	root := buildTestIndex(t)

	// Verify index exists before reindex.
	idxPath := db.IndexPath(root)
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		t.Fatal("index should exist before reindex")
	}

	err := executeCmd("--repo", root, "reindex")
	if err != nil {
		t.Fatalf("reindex command failed: %v", err)
	}

	// Verify index still exists after reindex.
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		t.Error("index should exist after reindex")
	}
}

func TestReindexCmd_NoExistingIndex(t *testing.T) {
	root := createTestRepo(t)
	err := executeCmd("--repo", root, "reindex")
	if err != nil {
		t.Fatalf("reindex with no existing index failed: %v", err)
	}

	// Should have created an index.
	idxPath := db.IndexPath(root)
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		t.Error("reindex should create an index")
	}
}

// --- Status command tests ---

func TestStatusCmd_NoIndex(t *testing.T) {
	root := t.TempDir()
	err := executeCmd("--repo", root, "status")
	if err != nil {
		t.Fatalf("status with no index should not error: %v", err)
	}
}

func TestStatusCmd_WithIndex(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "status")
	if err != nil {
		t.Fatalf("status command failed: %v", err)
	}
}

// --- Analyze command tests ---

func TestAnalyzeCmd_MissingArgs(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze")
	if err == nil {
		t.Fatal("analyze with no args should error")
	}
}

func TestAnalyzeCmd_NoIndex(t *testing.T) {
	root := createTestRepo(t)
	err := executeCmd("--repo", root, "analyze", "myapp/main.py")
	if err == nil {
		t.Fatal("analyze with no index should error")
	}
	if !strings.Contains(err.Error(), "no index found") {
		t.Errorf("error should mention no index, got: %v", err)
	}
}

func TestAnalyzeCmd_HappyPath(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze", "myapp/main.py")
	if err != nil {
		t.Fatalf("analyze command failed: %v", err)
	}
}

func TestAnalyzeCmd_JSONOutput(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze", "--json", "myapp/main.py")
	if err != nil {
		t.Fatalf("analyze --json failed: %v", err)
	}
}

func TestAnalyzeCmd_MultiFile(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze", "myapp/main.py", "myapp/models.py")
	if err != nil {
		t.Fatalf("multi-file analyze failed: %v", err)
	}
}

func TestAnalyzeCmd_NonexistentFile(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze", "nonexistent.py")
	// Should return an error for a file not in the index.
	if err == nil {
		t.Fatal("analyze for nonexistent file should error")
	}
}

func TestAnalyzeCmd_CapsFlags(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze",
		"--max-must-read", "3",
		"--max-related", "2",
		"--max-likely-modify", "1",
		"--max-tests", "1",
		"myapp/main.py")
	if err != nil {
		t.Fatalf("analyze with caps flags failed: %v", err)
	}
}

func TestAnalyzeCmd_ModeFlag(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{"plan", "plan"},
		{"implement", "implement"},
		{"review", "review"},
	}
	root := buildTestIndex(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executeCmd("--repo", root, "analyze", "--mode", tt.mode, "myapp/main.py")
			if err != nil {
				t.Fatalf("analyze --mode %s failed: %v", tt.mode, err)
			}
		})
	}
}

func TestAnalyzeCmd_NoExternalFlag(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze", "--no-external", "myapp/main.py")
	if err != nil {
		t.Fatalf("analyze --no-external failed: %v", err)
	}
}

func TestAnalyzeCmd_TokenBudgetFlag(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze", "--token-budget", "5000", "myapp/main.py")
	if err != nil {
		t.Fatalf("analyze --token-budget failed: %v", err)
	}
}

// --- Search command tests ---

func TestSearchCmd_MissingArgs(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "search")
	if err == nil {
		t.Fatal("search with no args should error")
	}
}

func TestSearchCmd_PathSearch(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "search", "main")
	if err != nil {
		t.Fatalf("search command failed: %v", err)
	}
}

func TestSearchCmd_NoResults(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "search", "xyznonexistent")
	if err != nil {
		t.Fatalf("search with no results should not error: %v", err)
	}
}

func TestSearchCmd_NoIndex(t *testing.T) {
	root := createTestRepo(t)
	err := executeCmd("--repo", root, "search", "main")
	if err != nil {
		t.Fatalf("search with no index should not error: %v", err)
	}
}

func TestSearchCmd_JSONOutput(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "search", "--json", "main")
	if err != nil {
		t.Fatalf("search --json failed: %v", err)
	}
}

func TestSearchCmd_InvalidType(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "search", "--type", "invalid", "main")
	if err == nil {
		t.Fatal("search with invalid type should error")
	}
}

func TestSearchCmd_SymbolType(t *testing.T) {
	root := buildTestIndex(t)
	// Symbol search should not error even if no symbols match.
	err := executeCmd("--repo", root, "search", "--type", "symbol", "User")
	if err != nil {
		t.Fatalf("search --type symbol failed: %v", err)
	}
}

func TestSearchCmd_LimitFlag(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "search", "--limit", "1", "myapp")
	if err != nil {
		t.Fatalf("search with limit failed: %v", err)
	}
}

// --- Archetypes command tests ---

func TestArchetypesCmd_ListFlag(t *testing.T) {
	root := t.TempDir()
	err := executeCmd("--repo", root, "archetypes", "--list")
	if err != nil {
		t.Fatalf("archetypes --list failed: %v", err)
	}
}

func TestArchetypesCmd_WithIndex(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "archetypes")
	if err != nil {
		t.Fatalf("archetypes command failed: %v", err)
	}
}

func TestArchetypesCmd_NoIndex(t *testing.T) {
	root := t.TempDir()
	// Archetypes on an empty repo opens/creates the DB and returns empty results.
	err := executeCmd("--repo", root, "archetypes")
	if err != nil {
		t.Fatalf("archetypes on empty repo should not error: %v", err)
	}
}

func TestArchetypesCmd_CategoriesFlag(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "archetypes", "--categories", "Service,Model")
	if err != nil {
		t.Fatalf("archetypes with --categories failed: %v", err)
	}
}

// --- History command tests ---

func TestHistoryCmd_SubcommandRequired(t *testing.T) {
	root := t.TempDir()
	err := executeCmd("--repo", root, "history")
	// history with no subcommand shows help (no error).
	if err != nil {
		t.Fatalf("history with no subcommand should not error: %v", err)
	}
}

func TestHistoryTrendCmd_EmptyHistory(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "history", "trend")
	if err != nil {
		t.Fatalf("history trend failed: %v", err)
	}
}

func TestHistoryHotspotsCmd_EmptyHistory(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "history", "hotspots")
	if err != nil {
		t.Fatalf("history hotspots failed: %v", err)
	}
}

func TestHistoryDistCmd_EmptyHistory(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "history", "distribution")
	if err != nil {
		t.Fatalf("history distribution failed: %v", err)
	}
}

func TestHistoryFileCmd_MissingArgs(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "history", "file")
	if err == nil {
		t.Fatal("history file with no args should error")
	}
}

func TestHistoryFileCmd_EmptyHistory(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "history", "file", "myapp/main.py")
	if err != nil {
		t.Fatalf("history file failed: %v", err)
	}
}

func TestHistoryTrendCmd_LimitFlag(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "history", "trend", "--limit", "5")
	if err != nil {
		t.Fatalf("history trend --limit failed: %v", err)
	}
}

func TestHistoryHotspotsCmd_DaysFlag(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "history", "hotspots", "--days", "7")
	if err != nil {
		t.Fatalf("history hotspots --days failed: %v", err)
	}
}

// --- Analyze-change command tests ---

func TestAnalyzeChangeCmd_InvalidRefRange(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze-change", "notavalidrange")
	if err == nil {
		t.Fatal("analyze-change with invalid ref range should error")
	}
	if !strings.Contains(err.Error(), "invalid ref range") {
		t.Errorf("error should mention invalid ref range, got: %v", err)
	}
}

func TestAnalyzeChangeCmd_TooManyArgs(t *testing.T) {
	root := buildTestIndex(t)
	err := executeCmd("--repo", root, "analyze-change", "a..b", "extra")
	if err == nil {
		t.Fatal("analyze-change with too many args should error")
	}
}

// buildTestRepoWithGit creates a git repo with an initial commit, modifies a file,
// commits the change, and returns the repo root and the base commit SHA.
func buildTestRepoWithGit(t *testing.T) (root string, baseSHA string) {
	t.Helper()
	root = createTestRepo(t)

	// Initialize git and create initial commit.
	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Get base SHA.
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	baseSHA = strings.TrimSpace(string(out))

	// Index.
	idxPath := db.IndexPath(root)
	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.MigrateToLatest(); err != nil {
		idx.Close()
		t.Fatal(err)
	}
	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx})
	if err != nil {
		idx.Close()
		t.Fatal(err)
	}
	if err := ix.Run(); err != nil {
		idx.Close()
		t.Fatal(err)
	}
	idx.Close()

	// Modify a file and commit.
	if err := os.WriteFile(filepath.Join(root, "myapp/main.py"),
		[]byte("from .models import User\nfrom .utils import helper\nimport os\n# modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "myapp/main.py"},
		{"git", "commit", "-m", "modify main.py"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	return root, baseSHA
}

func TestAnalyzeChangeCmd_JSONWithRiskTriage(t *testing.T) {
	root, baseSHA := buildTestRepoWithGit(t)

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := executeCmd("--repo", root, "analyze-change", baseSHA+"..HEAD", "--json")

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("analyze-change --json: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify JSON contains risk_triage.
	if !strings.Contains(output, "risk_triage") {
		t.Errorf("JSON output should contain risk_triage: %.200s...", output)
	}
	if !strings.Contains(output, "aggregate_risk") {
		t.Errorf("JSON output should contain aggregate_risk: %.200s...", output)
	}
	if !strings.Contains(output, "risk_score") {
		t.Errorf("JSON output should contain per-file risk_score: %.200s...", output)
	}
}

func TestAnalyzeChangeCmd_CompactWithRiskBadge(t *testing.T) {
	root, baseSHA := buildTestRepoWithGit(t)

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := executeCmd("--repo", root, "analyze-change", baseSHA+"..HEAD", "--compact")

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("analyze-change --compact: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Compact output should include the risk badge.
	if !strings.Contains(output, "RISK") {
		t.Errorf("compact output should contain risk badge: %s", output)
	}
}

func TestAnalyzeChangeCmd_WorkingTreeFallback(t *testing.T) {
	root := createTestRepo(t)

	// Initialize git and commit.
	for _, args := range [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "initial"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Index.
	idxPath := db.IndexPath(root)
	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := idx.MigrateToLatest(); err != nil {
		idx.Close()
		t.Fatal(err)
	}
	ix, err := indexer.NewIndexer(indexer.Config{RepoRoot: root, Index: idx})
	if err != nil {
		idx.Close()
		t.Fatal(err)
	}
	if err := ix.Run(); err != nil {
		idx.Close()
		t.Fatal(err)
	}
	idx.Close()

	// Modify a file WITHOUT committing.
	if err := os.WriteFile(filepath.Join(root, "myapp/main.py"),
		[]byte("from .models import User\nfrom .utils import helper\nimport os\n# uncommitted change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run analyze-change with HEAD..HEAD — committed diff is empty, should fall back to working tree.
	baseSHA := "HEAD"
	err = executeCmd("--repo", root, "analyze-change", baseSHA+"..HEAD", "--json")

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("analyze-change with working tree: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should detect the uncommitted change via working tree fallback.
	if !strings.Contains(output, "working-tree") {
		t.Errorf("expected 'working-tree' in ref_range for uncommitted changes: %.200s...", output)
	}
	if !strings.Contains(output, "myapp/main.py") {
		t.Errorf("should detect uncommitted myapp/main.py change: %.200s...", output)
	}
}

// --- Unit tests for formatPretty ---

func TestFormatPretty_MinimalOutput(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Context for: test.py") {
		t.Errorf("formatPretty should include subject, got: %s", result)
	}
}

func TestFormatPretty_WithMustRead(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		MustRead: []model.MustReadEntry{
			{File: "dep.py", Direction: "upstream", Symbols: []string{"Foo"}},
		},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Must Read (1 files)") {
		t.Errorf("formatPretty should show must read count, got: %s", result)
	}
	if !strings.Contains(result, "dep.py (Foo) [upstream]") {
		t.Errorf("formatPretty should show dep with symbols and direction, got: %s", result)
	}
}

func TestFormatPretty_WithMustReadStable(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		MustRead: []model.MustReadEntry{
			{File: "dep.py", Direction: "upstream", Stable: true},
		},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "[upstream, stable]") {
		t.Errorf("formatPretty should show stable tag, got: %s", result)
	}
}

func TestFormatPretty_WithMustReadDefinitions(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		MustRead: []model.MustReadEntry{
			{File: "dep.py", Definitions: []string{"def foo():", "class Bar:"}},
		},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "| def foo():") {
		t.Errorf("formatPretty should show definitions, got: %s", result)
	}
	if !strings.Contains(result, "| class Bar:") {
		t.Errorf("formatPretty should show all definitions, got: %s", result)
	}
}

func TestFormatPretty_WithBlastRadius(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		BlastRadius: &model.BlastRadius{
			Level:     "high",
			Detail:    "12 dependents",
			Fragility: 0.5,
		},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Blast Radius: high") {
		t.Errorf("formatPretty should show blast radius, got: %s", result)
	}
	if !strings.Contains(result, "fragility: 0.50") {
		t.Errorf("formatPretty should show fragility when below threshold, got: %s", result)
	}
}

func TestFormatPretty_FragilityAboveThreshold(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		BlastRadius: &model.BlastRadius{
			Level:     "high",
			Detail:    "12 dependents",
			Fragility: 0.8,
		},
	}
	result := formatPretty(output)
	if strings.Contains(result, "fragility") {
		t.Errorf("formatPretty should NOT show fragility when above threshold, got: %s", result)
	}
}

func TestFormatPretty_FragilityAtThreshold(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		BlastRadius: &model.BlastRadius{
			Level:     "medium",
			Detail:    "5 dependents",
			Fragility: fragilityDisplayThreshold,
		},
	}
	result := formatPretty(output)
	if strings.Contains(result, "fragility") {
		t.Errorf("formatPretty should NOT show fragility at exact threshold, got: %s", result)
	}
}

func TestFormatPretty_WithStats(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		Stats: &model.IndexStats{
			TotalFiles:      100,
			TotalEdges:      200,
			UnresolvedCount: 5,
		},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Index: 100 files, 200 edges, 5 unresolved") {
		t.Errorf("formatPretty should show stats, got: %s", result)
	}
}

func TestFormatPretty_WithCircularDeps(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		CircularDeps:  [][]string{{"a.py", "b.py", "a.py"}},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Circular Dependencies (1 cycles)") {
		t.Errorf("formatPretty should show circular deps, got: %s", result)
	}
	if !strings.Contains(result, "a.py -> b.py -> a.py") {
		t.Errorf("formatPretty should show cycle path, got: %s", result)
	}
}

func TestFormatPretty_WithTests(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		Tests: []model.TestEntry{
			{File: "test_a.py", Direct: true},
			{File: "test_b.py", Direct: false},
		},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Tests (2 files)") {
		t.Errorf("formatPretty should show test count, got: %s", result)
	}
	if !strings.Contains(result, "[direct]") {
		t.Errorf("formatPretty should show direct tier, got: %s", result)
	}
	if !strings.Contains(result, "[dependency]") {
		t.Errorf("formatPretty should show dependency tier, got: %s", result)
	}
}

func TestFormatPretty_WithTestsNote(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		TestsNote:     "no tests found",
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Tests: no tests found") {
		t.Errorf("formatPretty should show tests note when no tests, got: %s", result)
	}
}

func TestFormatPretty_WithHotspots(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		Hotspots:      []string{"hot.py"},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Hotspots (1 files") {
		t.Errorf("formatPretty should show hotspots, got: %s", result)
	}
	if !strings.Contains(result, "hot.py") {
		t.Errorf("formatPretty should show hotspot file, got: %s", result)
	}
}

func TestFormatPretty_WithExternal(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		External:      []string{"os", "json", "requests"},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "External: os, json, requests") {
		t.Errorf("formatPretty should show external deps, got: %s", result)
	}
}

func TestFormatPretty_WithMustReadNote(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		MustReadNote:  "file not in index",
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Must Read: file not in index") {
		t.Errorf("formatPretty should show must read note, got: %s", result)
	}
}

func TestFormatPretty_WithLikelyModify(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		LikelyModify: map[string][]model.LikelyModifyEntry{
			"myapp": {
				{File: "handler.py", Confidence: "high", Signals: []string{"imports"}},
			},
		},
	}
	result := formatPretty(output)
	if !strings.Contains(result, "Likely Modify:") {
		t.Errorf("formatPretty should show likely modify section, got: %s", result)
	}
	if !strings.Contains(result, "handler.py") {
		t.Errorf("formatPretty should show likely modify file, got: %s", result)
	}
}

func TestFormatPretty_WithLikelyModifyNote(t *testing.T) {
	output := &model.AnalysisOutput{
		Subject:       "test.py",
		SchemaVersion: "3.2",
		LikelyModify: map[string][]model.LikelyModifyEntry{
			"myapp": {
				{File: "handler.py", Confidence: "high", Signals: []string{"imports"}},
			},
		},
		LikelyModifyNote: "showing 1 of 5 candidates",
	}
	result := formatPretty(output)
	if !strings.Contains(result, "(showing 1 of 5 candidates)") {
		t.Errorf("formatPretty should show likely modify note, got: %s", result)
	}
}

// --- Unit tests for formatChangeReport ---

func TestFormatChangeReport_Minimal(t *testing.T) {
	report := &model.ChangeReport{
		RefRange: "main..HEAD",
		Summary: model.ChangeSummary{
			TotalFiles: 2,
			Modified:   2,
		},
		ChangedFiles: []model.ChangedFile{
			{File: "a.py", Status: "modified"},
			{File: "b.py", Status: "modified"},
		},
	}
	result := formatChangeReport(report)
	if !strings.Contains(result, "PR Impact Report: main..HEAD") {
		t.Errorf("formatChangeReport should show ref range header, got: %s", result)
	}
	if !strings.Contains(result, "Changed Files: 2") {
		t.Errorf("formatChangeReport should show changed file count, got: %s", result)
	}
}

func TestFormatChangeReport_WithBlastRadius(t *testing.T) {
	report := &model.ChangeReport{
		RefRange: "main..HEAD",
		Summary:  model.ChangeSummary{TotalFiles: 1, Modified: 1},
		ChangedFiles: []model.ChangedFile{
			{File: "a.py", Status: "modified"},
		},
		BlastRadius: &model.BlastRadius{
			Level:  "high",
			Detail: "10 dependents",
		},
	}
	result := formatChangeReport(report)
	if !strings.Contains(result, "HIGH [!!!]") {
		t.Errorf("formatChangeReport should show blast radius level, got: %s", result)
	}
}

func TestFormatChangeReport_WithTestGaps(t *testing.T) {
	report := &model.ChangeReport{
		RefRange: "main..HEAD",
		Summary:  model.ChangeSummary{TotalFiles: 1, Modified: 1},
		ChangedFiles: []model.ChangedFile{
			{File: "a.py", Status: "modified"},
		},
		TestGaps: []string{"a.py", "b.py"},
	}
	result := formatChangeReport(report)
	if !strings.Contains(result, "Test Gaps (2 files") {
		t.Errorf("formatChangeReport should show test gaps, got: %s", result)
	}
}

func TestFormatChangeReport_WithCoupling(t *testing.T) {
	report := &model.ChangeReport{
		RefRange: "main..HEAD",
		Summary:  model.ChangeSummary{TotalFiles: 2, Modified: 2},
		ChangedFiles: []model.ChangedFile{
			{File: "a.py", Status: "modified"},
			{File: "b.py", Status: "modified"},
		},
		Coupling: []model.CouplingPair{
			{FileA: "a.py", FileB: "b.py", Direction: "a_imports_b"},
		},
	}
	result := formatChangeReport(report)
	if !strings.Contains(result, "Cross-File Coupling") {
		t.Errorf("formatChangeReport should show coupling, got: %s", result)
	}
}

func TestFormatChangeReport_SummaryParts(t *testing.T) {
	report := &model.ChangeReport{
		RefRange: "main..HEAD",
		Summary: model.ChangeSummary{
			TotalFiles: 4,
			Added:      1,
			Modified:   2,
			Deleted:    1,
		},
		ChangedFiles: []model.ChangedFile{
			{File: "a.py", Status: "added"},
		},
	}
	result := formatChangeReport(report)
	if !strings.Contains(result, "1 added") {
		t.Errorf("formatChangeReport should show added count, got: %s", result)
	}
	if !strings.Contains(result, "2 modified") {
		t.Errorf("formatChangeReport should show modified count, got: %s", result)
	}
	if !strings.Contains(result, "1 deleted") {
		t.Errorf("formatChangeReport should show deleted count, got: %s", result)
	}
}

// --- Unit tests for shortPackageName ---

func TestShortPackageName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"packages/agents-core", "agents-core"},
		{"myapp", "myapp"},
		{".", "(root)"},
		{"a/b/c/deep", "deep"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shortPackageName(tt.input)
			if got != tt.want {
				t.Errorf("shortPackageName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Unit tests for formatSignals ---

func TestFormatSignals(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect string
	}{
		{"co_change", []string{"co_change:5"}, "co-changed 5x"},
		{"hidden_coupling", []string{"hidden_coupling:3"}, "hidden coupling (co-changed 3x, no import path)"},
		{"underscore_replacement", []string{"same_package"}, "same package"},
		{"multiple", []string{"co_change:2", "same_package"}, "co-changed 2x, same package"},
		{"empty", []string{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSignals(tt.input)
			if got != tt.expect {
				t.Errorf("formatSignals(%v) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

// --- Unit tests for sortedKeys ---

func TestSortedKeys(t *testing.T) {
	m := map[string][]string{
		"zebra": {"z"},
		"alpha": {"a"},
		"mid":   {"m"},
	}
	got := sortedKeys(m)
	want := []string{"alpha", "mid", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("sortedKeys returned %d keys, want %d", len(got), len(want))
	}
	for i, k := range got {
		if k != want[i] {
			t.Errorf("sortedKeys[%d] = %q, want %q", i, k, want[i])
		}
	}
}

func TestSortedKeys_Empty(t *testing.T) {
	m := map[string][]string{}
	got := sortedKeys(m)
	if len(got) != 0 {
		t.Errorf("sortedKeys of empty map should return empty slice, got %d", len(got))
	}
}

// --- Unit tests for formatBlastLevel ---

func TestFormatBlastLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"high", "HIGH [!!!]"},
		{"medium", "MEDIUM [!!]"},
		{"low", "LOW [.]"},
		{"unknown", "LOW [.]"},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := formatBlastLevel(tt.level)
			if got != tt.want {
				t.Errorf("formatBlastLevel(%q) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

// --- Unit tests for levelIndicator ---

func TestLevelIndicator(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"high", "[!!!]"},
		{"medium", "[!!]"},
		{"low", "[.]"},
		{"other", "[.]"},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := levelIndicator(tt.level)
			if got != tt.want {
				t.Errorf("levelIndicator(%q) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

// --- Unit tests for truncPath ---

func TestTruncPath(t *testing.T) {
	tests := []struct {
		path   string
		maxLen int
		want   string
	}{
		{"short.py", 20, "short.py"},
		{"very/long/path/to/some/file.py", 15, "...some/file.py"},
		{"exact", 5, "exact"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := truncPath(tt.path, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncPath(%q, %d) = %q, want %q", tt.path, tt.maxLen, got, tt.want)
			}
		})
	}
}
