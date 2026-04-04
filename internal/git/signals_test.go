package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// initGitRepo creates a git repo in dir with initial config.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
}

// commitFile creates or updates a file and commits it with a controlled date.
func commitFile(t *testing.T, dir, path, content string, date time.Time) {
	t.Helper()
	abs := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", path)
	dateStr := date.Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-m", "update "+path, "--date", dateStr)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+dateStr,
		"GIT_COMMITTER_DATE="+dateStr,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// commitFiles creates or updates multiple files and commits them together.
func commitFiles(t *testing.T, dir string, files map[string]string, date time.Time) {
	t.Helper()
	for path, content := range files {
		abs := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "git", "add", path)
	}
	dateStr := date.Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-m", "batch update", "--date", dateStr)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+dateStr,
		"GIT_COMMITTER_DATE="+dateStr,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestExtractBasicChurn(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Commit a.py 3 times, b.py 1 time.
	commitFile(t, dir, "a.py", "v1", now.AddDate(0, 0, -5))
	commitFile(t, dir, "a.py", "v2", now.AddDate(0, 0, -4))
	commitFile(t, dir, "a.py", "v3", now.AddDate(0, 0, -3))
	commitFile(t, dir, "b.py", "v1", now.AddDate(0, 0, -2))

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	if signals.Churn["a.py"] != 3 {
		t.Errorf("a.py churn = %d, want 3", signals.Churn["a.py"])
	}
	if signals.Churn["b.py"] != 1 {
		t.Errorf("b.py churn = %d, want 1", signals.Churn["b.py"])
	}
}

func TestExtractCoChangePairs(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Commit a.py + b.py together twice.
	commitFiles(t, dir, map[string]string{"a.py": "v1", "b.py": "v1"}, now.AddDate(0, 0, -5))
	commitFiles(t, dir, map[string]string{"a.py": "v2", "b.py": "v2"}, now.AddDate(0, 0, -3))

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	pair := [2]string{"a.py", "b.py"}
	if signals.CoChange[pair] != 2 {
		t.Errorf("co-change(a.py, b.py) = %d, want 2", signals.CoChange[pair])
	}
}

func TestExtractEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// No commits — Extract should fail (no HEAD).
	_, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err == nil {
		t.Error("expected error for empty repo")
	}
}

func TestExtractNoCommitsInWindow(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Old commit (200 days before HEAD).
	commitFile(t, dir, "a.py", "old", now.AddDate(0, 0, -200))
	// Recent HEAD commit (today) — only this file should be in the 90-day window.
	commitFile(t, dir, "b.py", "recent", now)

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	// Only the recent commit should be in window. a.py's commit is 200 days before HEAD.
	if signals.Churn["a.py"] != 0 {
		t.Errorf("a.py churn = %d, want 0 (outside 90-day window)", signals.Churn["a.py"])
	}
	if signals.Churn["b.py"] != 1 {
		t.Errorf("b.py churn = %d, want 1", signals.Churn["b.py"])
	}
}

func TestExtractLargeCommitSkipped(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Create a commit with 5 files (under the threshold of 3).
	files := make(map[string]string)
	for i := 0; i < 5; i++ {
		files[fmt.Sprintf("file_%d.py", i)] = "content"
	}
	commitFiles(t, dir, files, now.AddDate(0, 0, -5))

	// Extract with MaxCommitSize=3: the commit should be skipped.
	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90, MaxCommitSize: 3})
	if err != nil {
		t.Fatal(err)
	}

	if len(signals.Churn) != 0 {
		t.Errorf("expected 0 churn entries (commit too large), got %d", len(signals.Churn))
	}
}

func TestExtractOnlyWindowCounted(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Old commit (outside 30-day window).
	commitFile(t, dir, "a.py", "old", now.AddDate(0, 0, -60))
	// Recent commit (inside 30-day window).
	commitFile(t, dir, "a.py", "new", now.AddDate(0, 0, -5))

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 30})
	if err != nil {
		t.Fatal(err)
	}

	// Only the recent commit should be counted.
	if signals.Churn["a.py"] != 1 {
		t.Errorf("a.py churn = %d, want 1 (only within 30-day window)", signals.Churn["a.py"])
	}
}

func TestCoChangePairOrdering(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Commit z.py and a.py together (z > a lexically).
	commitFiles(t, dir, map[string]string{"z.py": "v1", "a.py": "v1"}, now.AddDate(0, 0, -5))

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	// Pair should be ordered (a.py, z.py) regardless of commit order.
	pair := [2]string{"a.py", "z.py"}
	if signals.CoChange[pair] != 1 {
		t.Errorf("co-change(a.py, z.py) = %d, want 1", signals.CoChange[pair])
	}

	// Reverse pair should not exist.
	reversePair := [2]string{"z.py", "a.py"}
	if signals.CoChange[reversePair] != 0 {
		t.Errorf("reverse pair should not exist, got %d", signals.CoChange[reversePair])
	}
}

func TestSingleFileCommitNoCoChange(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	commitFile(t, dir, "solo.py", "v1", now.AddDate(0, 0, -5))

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	if len(signals.CoChange) != 0 {
		t.Errorf("expected 0 co-change pairs for single-file commit, got %d", len(signals.CoChange))
	}
}

func TestParseGitLogEmpty(t *testing.T) {
	commits := parseGitLog([]byte(""))
	if len(commits) != 0 {
		t.Errorf("expected 0 commits for empty output, got %d", len(commits))
	}
}

func TestParseGitLogStandard(t *testing.T) {
	output := "abc123\x002024-06-15T10:00:00+00:00\nfile_a.py\nfile_b.py\n\ndef456\x002024-06-14T10:00:00+00:00\nfile_c.py\n"
	commits := parseGitLog([]byte(output))

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	if commits[0].Hash != "abc123" {
		t.Errorf("commit[0].Hash = %q, want %q", commits[0].Hash, "abc123")
	}
	if len(commits[0].Files) != 2 {
		t.Errorf("commit[0] files = %d, want 2", len(commits[0].Files))
	}
	if commits[1].Hash != "def456" {
		t.Errorf("commit[1].Hash = %q, want %q", commits[1].Hash, "def456")
	}
	if len(commits[1].Files) != 1 {
		t.Errorf("commit[1] files = %d, want 1", len(commits[1].Files))
	}
}

func TestExtractWindowDays(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()
	commitFile(t, dir, "a.py", "v1", now.AddDate(0, 0, -5))

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	if signals.WindowDays != 90 {
		t.Errorf("WindowDays = %d, want 90", signals.WindowDays)
	}
	if signals.ComputedAt == "" {
		t.Error("ComputedAt should not be empty")
	}
}

func TestExtractMultipleCoChangeCompound(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Three commits with a.py + b.py + c.py.
	for i := 0; i < 3; i++ {
		commitFiles(t, dir, map[string]string{
			"a.py": fmt.Sprintf("v%d", i),
			"b.py": fmt.Sprintf("v%d", i),
			"c.py": fmt.Sprintf("v%d", i),
		}, now.AddDate(0, 0, -(10-i)))
	}

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	// Each pair should have frequency 3.
	pairs := [][2]string{{"a.py", "b.py"}, {"a.py", "c.py"}, {"b.py", "c.py"}}
	for _, pair := range pairs {
		if signals.CoChange[pair] != 3 {
			t.Errorf("co-change(%s, %s) = %d, want 3", pair[0], pair[1], signals.CoChange[pair])
		}
	}
}

func TestExtractBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Commit a binary file alongside a text file.
	abs := filepath.Join(dir, "image.png")
	if err := os.WriteFile(abs, []byte{0x89, 0x50, 0x4E, 0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	abs2 := filepath.Join(dir, "code.py")
	if err := os.WriteFile(abs2, []byte("print('hello')"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")

	dateStr := now.AddDate(0, 0, -5).Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-m", "add files", "--date", dateStr)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+dateStr,
		"GIT_COMMITTER_DATE="+dateStr,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	// Both files should appear in churn.
	if signals.Churn["image.png"] != 1 {
		t.Errorf("image.png churn = %d, want 1", signals.Churn["image.png"])
	}
	if signals.Churn["code.py"] != 1 {
		t.Errorf("code.py churn = %d, want 1", signals.Churn["code.py"])
	}
}

func TestExtractRenamedFiles(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()

	// Create and commit a file.
	commitFile(t, dir, "old_name.py", "content", now.AddDate(0, 0, -10))

	// Rename it.
	run(t, dir, "git", "mv", "old_name.py", "new_name.py")
	dateStr := now.AddDate(0, 0, -5).Format(time.RFC3339)
	cmd := exec.Command("git", "commit", "-m", "rename", "--date", dateStr)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+dateStr,
		"GIT_COMMITTER_DATE="+dateStr,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	signals, err := Extract(Config{RepoRoot: dir, WindowDays: 90})
	if err != nil {
		t.Fatal(err)
	}

	// The rename commit should show the new name in git log --name-only.
	// old_name.py appears in the first commit, new_name.py in the rename commit.
	if signals.Churn["old_name.py"] != 1 {
		t.Errorf("old_name.py churn = %d, want 1", signals.Churn["old_name.py"])
	}
	if signals.Churn["new_name.py"] < 1 {
		t.Errorf("new_name.py churn = %d, want >= 1", signals.Churn["new_name.py"])
	}
}

func TestExtractDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	now := time.Now()
	commitFile(t, dir, "a.py", "v1", now.AddDate(0, 0, -5))

	// Zero values should use defaults.
	signals, err := Extract(Config{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}

	if signals.WindowDays != 90 {
		t.Errorf("default WindowDays = %d, want 90", signals.WindowDays)
	}
}
