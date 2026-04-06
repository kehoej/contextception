package change

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/model"
)

// --- Test helpers ---

// openTestIndex creates a fresh SQLite index in a temp directory, runs migrations,
// and returns the *db.Index. The caller should defer idx.Close().
func openTestIndex(t *testing.T) *db.Index {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := db.OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	if _, err := idx.MigrateToLatest(); err != nil {
		idx.Close()
		t.Fatalf("MigrateToLatest: %v", err)
	}
	return idx
}

// addFile inserts a file record into the index.
func addFile(t *testing.T, idx *db.Index, path string) {
	t.Helper()
	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := db.UpsertFileTx(tx, path, "hash-"+path, time.Now().Unix(), "python", 100); err != nil {
		tx.Rollback()
		t.Fatalf("UpsertFileTx(%s): %v", path, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// addEdge inserts a dependency edge between two files in the index.
func addEdge(t *testing.T, idx *db.Index, src, dst string) {
	t.Helper()
	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := db.InsertEdgeTx(tx, src, dst, "import", dst, "direct", 1, nil); err != nil {
		tx.Rollback()
		t.Fatalf("InsertEdgeTx(%s -> %s): %v", src, dst, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// initGitRepo creates a git repo in dir with initial config.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
}

// commitFile creates or updates a file and commits it.
func commitFile(t *testing.T, dir, path, content string) {
	t.Helper()
	abs := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", path)
	cmd := exec.Command("git", "commit", "-m", "update "+path)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// deleteFile removes a file and commits the deletion.
func deleteFile(t *testing.T, dir, path string) {
	t.Helper()
	run(t, dir, "git", "rm", path)
	cmd := exec.Command("git", "commit", "-m", "delete "+path)
	cmd.Dir = dir
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

// --- ParseGitStatus tests ---

func TestParseGitStatus(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"A", "added"},
		{"D", "deleted"},
		{"M", "modified"},
		{"R100", "renamed"},
		{"R050", "renamed"},
		{"C", "modified"}, // unknown defaults to modified
		{"T", "modified"}, // type change defaults to modified
		{"", "modified"},  // empty defaults to modified
		{"X", "modified"}, // unknown letter defaults to modified
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ParseGitStatus(tc.input)
			if got != tc.want {
				t.Errorf("ParseGitStatus(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- GitDiffFiles tests ---

func TestGitDiffFiles_BasicDiff(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create initial commit on main.
	commitFile(t, dir, "app/main.py", "print('hello')")
	commitFile(t, dir, "app/utils.py", "def helper(): pass")

	// Record the base ref.
	base := getHead(t, dir)

	// Make changes.
	commitFile(t, dir, "app/main.py", "print('hello world')")
	commitFile(t, dir, "app/new_file.py", "# new")

	head := getHead(t, dir)

	diffs, err := GitDiffFiles(dir, base, head)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}

	if len(diffs) != 2 {
		t.Fatalf("got %d diffs, want 2", len(diffs))
	}

	// Build map for easy lookup.
	diffMap := make(map[string]string)
	for _, d := range diffs {
		diffMap[d.Path] = d.Status
	}

	if diffMap["app/main.py"] != "modified" {
		t.Errorf("app/main.py status = %q, want modified", diffMap["app/main.py"])
	}
	if diffMap["app/new_file.py"] != "added" {
		t.Errorf("app/new_file.py status = %q, want added", diffMap["app/new_file.py"])
	}
}

func TestGitDiffFiles_WithDeletion(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	commitFile(t, dir, "to_delete.py", "content")
	commitFile(t, dir, "keep.py", "content")
	base := getHead(t, dir)

	deleteFile(t, dir, "to_delete.py")
	head := getHead(t, dir)

	diffs, err := GitDiffFiles(dir, base, head)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}

	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	if diffs[0].Status != "deleted" {
		t.Errorf("status = %q, want deleted", diffs[0].Status)
	}
}

func TestGitDiffFiles_EmptyDiff(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	commitFile(t, dir, "file.py", "content")
	head := getHead(t, dir)

	// Same ref for base and head — no changes.
	diffs, err := GitDiffFiles(dir, head, head)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}
	if len(diffs) != 0 {
		t.Errorf("got %d diffs, want 0", len(diffs))
	}
}

// --- DetectCoupling tests ---

func TestDetectCoupling_NoCoupling(t *testing.T) {
	idx := openTestIndex(t)
	defer idx.Close()

	addFile(t, idx, "a.py")
	addFile(t, idx, "b.py")
	// No edges between a and b.

	pairs := DetectCoupling(idx, []string{"a.py", "b.py"})
	if len(pairs) != 0 {
		t.Errorf("got %d pairs, want 0", len(pairs))
	}
}

func TestDetectCoupling_SingleDirection(t *testing.T) {
	idx := openTestIndex(t)
	defer idx.Close()

	addFile(t, idx, "a.py")
	addFile(t, idx, "b.py")
	addEdge(t, idx, "a.py", "b.py")

	pairs := DetectCoupling(idx, []string{"a.py", "b.py"})
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	p := pairs[0]
	// Pair order is normalized: a < b.
	if p.FileA != "a.py" || p.FileB != "b.py" {
		t.Errorf("pair = (%s, %s), want (a.py, b.py)", p.FileA, p.FileB)
	}
	if p.Direction != "a_imports_b" {
		t.Errorf("direction = %q, want a_imports_b", p.Direction)
	}
}

func TestDetectCoupling_MutualImport(t *testing.T) {
	idx := openTestIndex(t)
	defer idx.Close()

	addFile(t, idx, "a.py")
	addFile(t, idx, "b.py")
	addEdge(t, idx, "a.py", "b.py")
	addEdge(t, idx, "b.py", "a.py")

	pairs := DetectCoupling(idx, []string{"a.py", "b.py"})
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	if pairs[0].Direction != "mutual" {
		t.Errorf("direction = %q, want mutual", pairs[0].Direction)
	}
}

func TestDetectCoupling_SingleFile(t *testing.T) {
	idx := openTestIndex(t)
	defer idx.Close()

	addFile(t, idx, "a.py")

	// Less than 2 files: should return nil.
	pairs := DetectCoupling(idx, []string{"a.py"})
	if pairs != nil {
		t.Errorf("got %v, want nil for single file", pairs)
	}
}

func TestDetectCoupling_MultipleFiles(t *testing.T) {
	idx := openTestIndex(t)
	defer idx.Close()

	addFile(t, idx, "a.py")
	addFile(t, idx, "b.py")
	addFile(t, idx, "c.py")
	addEdge(t, idx, "a.py", "b.py")
	addEdge(t, idx, "b.py", "c.py")

	pairs := DetectCoupling(idx, []string{"a.py", "b.py", "c.py"})
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2", len(pairs))
	}
}

// --- DetectTestGaps tests ---

func TestDetectTestGaps_NilAnalysis(t *testing.T) {
	gaps := DetectTestGaps([]string{"a.py"}, nil)
	if gaps != nil {
		t.Errorf("got %v, want nil for nil analysis", gaps)
	}
}

func TestDetectTestGaps_AllCovered(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Tests: []model.TestEntry{
			{File: "test_models.py", Direct: true},
		},
	}

	gaps := DetectTestGaps([]string{"models.py"}, analysis)
	if len(gaps) != 0 {
		t.Errorf("got %d gaps, want 0 (models.py has test_models.py)", len(gaps))
	}
}

func TestDetectTestGaps_WithGap(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Tests: []model.TestEntry{
			{File: "test_models.py", Direct: true},
		},
	}

	gaps := DetectTestGaps([]string{"models.py", "utils.py"}, analysis)
	if len(gaps) != 1 {
		t.Fatalf("got %d gaps, want 1", len(gaps))
	}
	if gaps[0] != "utils.py" {
		t.Errorf("gap = %q, want utils.py", gaps[0])
	}
}

func TestDetectTestGaps_SkipsTestFiles(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Tests: []model.TestEntry{},
	}

	// test_utils.py is itself a test file — should not appear in gaps.
	gaps := DetectTestGaps([]string{"test_utils.py"}, analysis)
	if len(gaps) != 0 {
		t.Errorf("got %d gaps, want 0 (test files should be skipped)", len(gaps))
	}
}

func TestDetectTestGaps_Sorted(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Tests: []model.TestEntry{},
	}

	gaps := DetectTestGaps([]string{"z_module.py", "a_module.py", "m_module.py"}, analysis)
	if len(gaps) != 3 {
		t.Fatalf("got %d gaps, want 3", len(gaps))
	}
	if !sort.StringsAreSorted(gaps) {
		t.Errorf("gaps not sorted: %v", gaps)
	}
}

// --- HasTestForFile tests ---

func TestHasTestForFile(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		analysis *model.AnalysisOutput
		want     bool
	}{
		{
			name:     "nil analysis",
			file:     "models.py",
			analysis: nil,
			want:     false,
		},
		{
			name: "matching test file",
			file: "models.py",
			analysis: &model.AnalysisOutput{
				Tests: []model.TestEntry{
					{File: "test_models.py", Direct: true},
				},
			},
			want: true,
		},
		{
			name: "no matching test",
			file: "models.py",
			analysis: &model.AnalysisOutput{
				Tests: []model.TestEntry{
					{File: "test_utils.py", Direct: true},
				},
			},
			want: false,
		},
		{
			name: "indirect test not counted",
			file: "models.py",
			analysis: &model.AnalysisOutput{
				Tests: []model.TestEntry{
					{File: "test_models.py", Direct: false},
				},
			},
			want: false,
		},
		{
			name: "go test suffix convention",
			file: "handler.go",
			analysis: &model.AnalysisOutput{
				Tests: []model.TestEntry{
					{File: "handler_test.go", Direct: true},
				},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HasTestForFile(tc.file, tc.analysis)
			if got != tc.want {
				t.Errorf("HasTestForFile(%q) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}

// --- BuildSummary tests ---

func TestBuildSummary_BasicCounts(t *testing.T) {
	files := []model.ChangedFile{
		{File: "a.py", Status: "added", Indexed: false},
		{File: "b.py", Status: "modified", Indexed: true},
		{File: "c.py", Status: "deleted", Indexed: false},
		{File: "d.py", Status: "renamed", Indexed: true},
		{File: "test_e.py", Status: "added", Indexed: true},
	}
	perFileBlast := map[string]*model.BlastRadius{
		"b.py": {Level: "high", Detail: "many dependents"},
	}

	summary := BuildSummary(files, perFileBlast)

	if summary.TotalFiles != 5 {
		t.Errorf("TotalFiles = %d, want 5", summary.TotalFiles)
	}
	if summary.Added != 2 {
		t.Errorf("Added = %d, want 2", summary.Added)
	}
	if summary.Modified != 1 {
		t.Errorf("Modified = %d, want 1", summary.Modified)
	}
	if summary.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", summary.Deleted)
	}
	if summary.Renamed != 1 {
		t.Errorf("Renamed = %d, want 1", summary.Renamed)
	}
	if summary.IndexedFiles != 3 {
		t.Errorf("IndexedFiles = %d, want 3", summary.IndexedFiles)
	}
	if summary.TestFiles != 1 {
		t.Errorf("TestFiles = %d, want 1", summary.TestFiles)
	}
	if summary.HighRiskFiles != 1 {
		t.Errorf("HighRiskFiles = %d, want 1", summary.HighRiskFiles)
	}
}

func TestBuildSummary_EmptyFiles(t *testing.T) {
	summary := BuildSummary(nil, nil)
	if summary.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", summary.TotalFiles)
	}
}

func TestBuildSummary_NoHighRisk(t *testing.T) {
	files := []model.ChangedFile{
		{File: "a.py", Status: "modified", Indexed: true},
	}
	perFileBlast := map[string]*model.BlastRadius{
		"a.py": {Level: "medium", Detail: "some deps"},
	}

	summary := BuildSummary(files, perFileBlast)
	if summary.HighRiskFiles != 0 {
		t.Errorf("HighRiskFiles = %d, want 0 (medium is not high)", summary.HighRiskFiles)
	}
}

// --- ExtractHiddenCoupling tests ---

func TestExtractHiddenCoupling_NilAnalysis(t *testing.T) {
	entries := ExtractHiddenCoupling(nil, map[string]bool{"a.py": true})
	if entries != nil {
		t.Errorf("got %v, want nil for nil analysis", entries)
	}
}

func TestExtractHiddenCoupling_NoHiddenCouplingSignals(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Subject: "a.py",
		Related: map[string][]model.RelatedEntry{
			"pkg": {
				{File: "other.py", Signals: []string{"sibling"}},
			},
		},
	}
	entries := ExtractHiddenCoupling(analysis, map[string]bool{"a.py": true})
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestExtractHiddenCoupling_FindsPartner(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Subject: "a.py",
		Related: map[string][]model.RelatedEntry{
			"pkg": {
				{File: "partner.py", Signals: []string{"hidden_coupling:5"}},
			},
		},
	}
	changedFiles := map[string]bool{"a.py": true}

	entries := ExtractHiddenCoupling(analysis, changedFiles)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Partner != "partner.py" {
		t.Errorf("partner = %q, want partner.py", entries[0].Partner)
	}
	if entries[0].Frequency != 5 {
		t.Errorf("frequency = %d, want 5", entries[0].Frequency)
	}
	if entries[0].ChangedFile != "a.py" {
		t.Errorf("changed_file = %q, want a.py", entries[0].ChangedFile)
	}
}

func TestExtractHiddenCoupling_SkipsChangedPartner(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Subject: "a.py",
		Related: map[string][]model.RelatedEntry{
			"pkg": {
				{File: "b.py", Signals: []string{"hidden_coupling:3"}},
			},
		},
	}
	// b.py is also a changed file, so it should be excluded.
	changedFiles := map[string]bool{"a.py": true, "b.py": true}

	entries := ExtractHiddenCoupling(analysis, changedFiles)
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (b.py is in changed set)", len(entries))
	}
}

func TestExtractHiddenCoupling_InvalidFrequency(t *testing.T) {
	analysis := &model.AnalysisOutput{
		Subject: "a.py",
		Related: map[string][]model.RelatedEntry{
			"pkg": {
				{File: "partner.py", Signals: []string{"hidden_coupling:notanumber"}},
			},
		},
	}

	entries := ExtractHiddenCoupling(analysis, map[string]bool{"a.py": true})
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 (invalid frequency should be skipped)", len(entries))
	}
}

// --- GitDiffFiles integration tests ---

func TestGitDiffFiles_MultipleStatusTypes(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Initial files.
	commitFile(t, dir, "existing.py", "v1")
	commitFile(t, dir, "to_remove.py", "content")
	base := getHead(t, dir)

	// Modify, delete, and add.
	commitFile(t, dir, "existing.py", "v2")
	deleteFile(t, dir, "to_remove.py")
	commitFile(t, dir, "brand_new.py", "new content")
	head := getHead(t, dir)

	diffs, err := GitDiffFiles(dir, base, head)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}

	statusMap := make(map[string]string)
	for _, d := range diffs {
		statusMap[d.Path] = d.Status
	}

	if statusMap["existing.py"] != "modified" {
		t.Errorf("existing.py: got %q, want modified", statusMap["existing.py"])
	}
	if statusMap["to_remove.py"] != "deleted" {
		t.Errorf("to_remove.py: got %q, want deleted", statusMap["to_remove.py"])
	}
	if statusMap["brand_new.py"] != "added" {
		t.Errorf("brand_new.py: got %q, want added", statusMap["brand_new.py"])
	}
}

// --- BuildReport integration tests ---

func TestBuildReport_EmptyDiff(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	commitFile(t, dir, "file.py", "content")
	head := getHead(t, dir)

	idx := openTestIndex(t)
	defer idx.Close()

	cfg := Config{RepoRoot: dir}
	report, err := BuildReport(idx, cfg, head, head)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	if report.SchemaVersion != ChangeSchemaVersion {
		t.Errorf("schema_version = %q, want %q", report.SchemaVersion, ChangeSchemaVersion)
	}
	if len(report.ChangedFiles) != 0 {
		t.Errorf("changed_files = %d, want 0", len(report.ChangedFiles))
	}
	if report.BlastRadius == nil {
		t.Fatal("blast_radius is nil, want non-nil")
	}
	if report.BlastRadius.Level != "low" {
		t.Errorf("blast_radius.level = %q, want low", report.BlastRadius.Level)
	}
}

func TestBuildReport_ChangedFileNotIndexed(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	commitFile(t, dir, "unindexed.py", "v1")
	base := getHead(t, dir)

	commitFile(t, dir, "unindexed.py", "v2")
	head := getHead(t, dir)

	idx := openTestIndex(t)
	defer idx.Close()
	// Do NOT add unindexed.py to the index.

	cfg := Config{RepoRoot: dir}
	report, err := BuildReport(idx, cfg, base, head)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	if len(report.ChangedFiles) != 1 {
		t.Fatalf("changed_files = %d, want 1", len(report.ChangedFiles))
	}
	cf := report.ChangedFiles[0]
	if cf.Indexed {
		t.Error("expected Indexed=false for unindexed file")
	}
	// Should still produce a valid report with low blast radius.
	if report.BlastRadius == nil {
		t.Fatal("blast_radius is nil")
	}
	if report.BlastRadius.Level != "low" {
		t.Errorf("blast_radius.level = %q, want low (no analyzable files)", report.BlastRadius.Level)
	}
}

func TestBuildReport_DeletedFileSkipped(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	commitFile(t, dir, "to_delete.py", "content")
	base := getHead(t, dir)

	deleteFile(t, dir, "to_delete.py")
	head := getHead(t, dir)

	idx := openTestIndex(t)
	defer idx.Close()
	// Even if the file was indexed, deleted files should not be analyzed.
	addFile(t, idx, "to_delete.py")

	cfg := Config{RepoRoot: dir}
	report, err := BuildReport(idx, cfg, base, head)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	if len(report.ChangedFiles) != 1 {
		t.Fatalf("changed_files = %d, want 1", len(report.ChangedFiles))
	}
	if report.ChangedFiles[0].Status != "deleted" {
		t.Errorf("status = %q, want deleted", report.ChangedFiles[0].Status)
	}
	// Deleted file should not be indexed in the report (FileExists skipped for deleted).
	if report.ChangedFiles[0].Indexed {
		t.Error("deleted file should not be marked as indexed in the report")
	}
}

func TestBuildReport_SingleIndexedFile(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	commitFile(t, dir, "app/models.py", "class User: pass")
	base := getHead(t, dir)

	commitFile(t, dir, "app/models.py", "class User:\n    name = 'test'")
	head := getHead(t, dir)

	idx := openTestIndex(t)
	defer idx.Close()
	addFile(t, idx, "app/models.py")

	cfg := Config{RepoRoot: dir}
	report, err := BuildReport(idx, cfg, base, head)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	if len(report.ChangedFiles) != 1 {
		t.Fatalf("changed_files = %d, want 1", len(report.ChangedFiles))
	}
	cf := report.ChangedFiles[0]
	if cf.File != "app/models.py" {
		t.Errorf("file = %q, want app/models.py", cf.File)
	}
	if !cf.Indexed {
		t.Error("expected Indexed=true for indexed file")
	}
	if cf.Status != "modified" {
		t.Errorf("status = %q, want modified", cf.Status)
	}

	// Verify report structure.
	if report.RefRange != base+".."+head {
		t.Errorf("ref_range = %q, want %q", report.RefRange, base+".."+head)
	}
	if report.BlastRadius == nil {
		t.Fatal("blast_radius is nil")
	}
}

func TestBuildReport_MultipleIndexedFiles(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	commitFile(t, dir, "app/models.py", "class User: pass")
	commitFile(t, dir, "app/views.py", "from .models import User")
	base := getHead(t, dir)

	commitFile(t, dir, "app/models.py", "class User:\n    v2 = True")
	commitFile(t, dir, "app/views.py", "from .models import User\n# v2")
	head := getHead(t, dir)

	idx := openTestIndex(t)
	defer idx.Close()
	addFile(t, idx, "app/models.py")
	addFile(t, idx, "app/views.py")
	addEdge(t, idx, "app/views.py", "app/models.py")

	cfg := Config{RepoRoot: dir}
	report, err := BuildReport(idx, cfg, base, head)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	if len(report.ChangedFiles) != 2 {
		t.Fatalf("changed_files = %d, want 2", len(report.ChangedFiles))
	}

	// Coupling should detect the edge between views and models.
	if len(report.Coupling) == 0 {
		t.Error("expected coupling between views.py and models.py")
	}

	// Summary should reflect 2 modified, 2 indexed.
	if report.Summary.TotalFiles != 2 {
		t.Errorf("summary.total_files = %d, want 2", report.Summary.TotalFiles)
	}
	if report.Summary.Modified != 2 {
		t.Errorf("summary.modified = %d, want 2", report.Summary.Modified)
	}
	if report.Summary.IndexedFiles != 2 {
		t.Errorf("summary.indexed_files = %d, want 2", report.Summary.IndexedFiles)
	}
}

// --- getHead helper ---

func getHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}
