package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	// Verify the file was created.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}

	// Verify pragmas by querying them.
	var journalMode string
	if err := idx.DB.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var fk int
	if err := idx.DB.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("querying foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestSchemaVersionFreshDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	version, err := idx.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 0 {
		t.Errorf("SchemaVersion = %d, want 0 for fresh database", version)
	}
}

func TestMigrateToLatest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatalf("MigrateToLatest: %v", err)
	}

	version, err := idx.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 4 {
		t.Errorf("SchemaVersion = %d, want 4 after migration", version)
	}

	// Verify tables exist by querying them.
	tables := []string{"meta", "files", "edges", "signals", "unresolved", "package_roots", "git_signals", "co_change_pairs"}
	for _, table := range tables {
		var count int
		if err := idx.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Errorf("table %q does not exist or cannot be queried: %v", table, err)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	// Run migrations twice — second run should be a no-op.
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatalf("first MigrateToLatest: %v", err)
	}
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatalf("second MigrateToLatest: %v", err)
	}

	version, err := idx.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 4 {
		t.Errorf("SchemaVersion = %d, want 4", version)
	}
}

func TestGetUnresolvedByFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// Insert a file first (foreign key).
	if err = InsertFileTx(tx, "app/main.py", "abc", 1000, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertUnresolvedTx(tx, "app/main.py", "os", "stdlib", 1); err != nil {
		t.Fatal(err)
	}
	if err = InsertUnresolvedTx(tx, "app/main.py", "requests", "external", 2); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	records, err := idx.GetUnresolvedByFile("app/main.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	// Verify fields.
	found := map[string]bool{}
	for _, r := range records {
		found[r.Specifier] = true
		if r.SrcFile != "app/main.py" {
			t.Errorf("SrcFile = %q, want %q", r.SrcFile, "app/main.py")
		}
	}
	if !found["os"] || !found["requests"] {
		t.Errorf("expected os and requests, got %v", records)
	}
}

func TestGetUnresolvedSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "a.py", "h1", 1, "python", 10); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "b.py", "h2", 1, "python", 10); err != nil {
		t.Fatal(err)
	}
	if err = InsertUnresolvedTx(tx, "a.py", "os", "stdlib", 1); err != nil {
		t.Fatal(err)
	}
	if err = InsertUnresolvedTx(tx, "a.py", "sys", "stdlib", 2); err != nil {
		t.Fatal(err)
	}
	if err = InsertUnresolvedTx(tx, "b.py", "requests", "external", 1); err != nil {
		t.Fatal(err)
	}
	if err = InsertUnresolvedTx(tx, "b.py", "missing_mod", "not_found", 3); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	summary, err := idx.GetUnresolvedSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 4 {
		t.Errorf("Total = %d, want 4", summary.Total)
	}
	if summary.ByReason["stdlib"] != 2 {
		t.Errorf("stdlib count = %d, want 2", summary.ByReason["stdlib"])
	}
	if summary.ByReason["external"] != 1 {
		t.Errorf("external count = %d, want 1", summary.ByReason["external"])
	}
	if summary.ByReason["not_found"] != 1 {
		t.Errorf("not_found count = %d, want 1", summary.ByReason["not_found"])
	}
}

func TestGetUnresolvedByFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	records, err := idx.GetUnresolvedByFile("nonexistent.py")
	if err != nil {
		t.Fatal(err)
	}
	if records != nil {
		t.Errorf("expected nil slice for file with no unresolved, got %v", records)
	}
}

func TestIndexPath(t *testing.T) {
	got := IndexPath("/home/user/repo")
	want := filepath.Join("/home/user/repo", IndexDir, IndexFile)
	if got != want {
		t.Errorf("IndexPath = %q, want %q", got, want)
	}
}

func TestMigrateV1ToV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	version, err := idx.SchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	if version != 4 {
		t.Errorf("SchemaVersion = %d, want 4", version)
	}

	// Verify new tables exist.
	for _, table := range []string{"git_signals", "co_change_pairs"} {
		var count int
		if err := idx.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Errorf("table %q does not exist: %v", table, err)
		}
	}
}

func TestGitSignalInsertAndQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	// Insert a file first (FK constraint on git_signals).
	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "app/models.py", "abc", 1000, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "app/views.py", "def", 1000, "python", 200); err != nil {
		t.Fatal(err)
	}
	if err = InsertGitSignalTx(tx, GitSignalRecord{
		FilePath: "app/models.py", SignalType: "churn", Value: 5.0, WindowDays: 90, ComputedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err = InsertGitSignalTx(tx, GitSignalRecord{
		FilePath: "app/views.py", SignalType: "churn", Value: 3.0, WindowDays: 90, ComputedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Query churn.
	churn, err := idx.GetChurnForFiles([]string{"app/models.py", "app/views.py"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if churn["app/models.py"] != 5.0 {
		t.Errorf("models.py churn = %f, want 5.0", churn["app/models.py"])
	}
	if churn["app/views.py"] != 3.0 {
		t.Errorf("views.py churn = %f, want 4.0", churn["app/views.py"])
	}

	// Query max churn.
	max, err := idx.GetMaxChurn(90)
	if err != nil {
		t.Fatal(err)
	}
	if max != 5.0 {
		t.Errorf("max churn = %f, want 5.0", max)
	}
}

func TestCoChangePairInsertAndQuery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// co_change_pairs has no FK, so no need to insert files.
	if err = InsertCoChangePairTx(tx, CoChangePairRecord{
		FileA: "a.py", FileB: "b.py", Frequency: 5, WindowDays: 90, ComputedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err = InsertCoChangePairTx(tx, CoChangePairRecord{
		FileA: "a.py", FileB: "c.py", Frequency: 2, WindowDays: 90, ComputedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Query partners for a.py.
	partners, err := idx.GetCoChangePartners("a.py", 90, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(partners) != 2 {
		t.Fatalf("expected 2 partners, got %d", len(partners))
	}
	// Should be sorted by frequency desc.
	if partners[0].Path != "b.py" || partners[0].Frequency != 5 {
		t.Errorf("partner[0] = %+v, want b.py/5", partners[0])
	}
	if partners[1].Path != "c.py" || partners[1].Frequency != 2 {
		t.Errorf("partner[1] = %+v, want c.py/2", partners[1])
	}

	// Query partners for b.py (should find a.py via reverse lookup).
	partners2, err := idx.GetCoChangePartners("b.py", 90, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(partners2) != 1 {
		t.Fatalf("expected 1 partner for b.py, got %d", len(partners2))
	}
	if partners2[0].Path != "a.py" {
		t.Errorf("partner = %q, want a.py", partners2[0].Path)
	}
}

func TestClearGitDataTx(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	// Insert some data.
	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "x.py", "h", 1, "python", 10); err != nil {
		t.Fatal(err)
	}
	if err = InsertGitSignalTx(tx, GitSignalRecord{
		FilePath: "x.py", SignalType: "churn", Value: 3.0, WindowDays: 90, ComputedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err = InsertCoChangePairTx(tx, CoChangePairRecord{
		FileA: "x.py", FileB: "y.py", Frequency: 1, WindowDays: 90, ComputedAt: "2024-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Clear git data.
	tx2, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = ClearGitDataTx(tx2); err != nil {
		t.Fatal(err)
	}
	if err = tx2.Commit(); err != nil {
		t.Fatal(err)
	}

	// Verify both tables are empty.
	var count int
	if err := idx.DB.QueryRow("SELECT COUNT(*) FROM git_signals").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("git_signals count = %d, want 0", count)
	}
	if err := idx.DB.QueryRow("SELECT COUNT(*) FROM co_change_pairs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("co_change_pairs count = %d, want 0", count)
	}

	// files table should be untouched.
	if err := idx.DB.QueryRow("SELECT COUNT(*) FROM files").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("files count = %d, want 1 (should not be cleared)", count)
	}
}

func TestGetMaxChurnEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	max, err := idx.GetMaxChurn(90)
	if err != nil {
		t.Fatal(err)
	}
	if max != 0 {
		t.Errorf("max churn = %f, want 0 for empty table", max)
	}
}

func TestMigrateToLatestReturnsBool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	// First call: migrations should be applied.
	applied, err := idx.MigrateToLatest()
	if err != nil {
		t.Fatalf("first MigrateToLatest: %v", err)
	}
	if !applied {
		t.Error("expected applied=true on fresh database")
	}

	// Second call: no-op, should return false.
	applied2, err := idx.MigrateToLatest()
	if err != nil {
		t.Fatalf("second MigrateToLatest: %v", err)
	}
	if applied2 {
		t.Error("expected applied=false when already up to date")
	}
}

func TestGetImporterSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/models.py", "h1", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/views.py", "h2", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/api.py", "h3", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	// views.py imports models.py with symbols ["User", "Post"]
	if err = InsertEdgeTx(tx, "pkg/views.py", "pkg/models.py", "import", "models", "relative", 1, []string{"User", "Post"}); err != nil {
		t.Fatal(err)
	}
	// api.py imports models.py with symbols ["User"]
	if err = InsertEdgeTx(tx, "pkg/api.py", "pkg/models.py", "import", "models", "relative", 1, []string{"User"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	result, err := idx.GetImporterSymbols("pkg/models.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 importers, got %d", len(result))
	}
	// views.py should have ["Post", "User"] (sorted).
	viewSyms := result["pkg/views.py"]
	if len(viewSyms) != 2 || viewSyms[0] != "Post" || viewSyms[1] != "User" {
		t.Errorf("views.py symbols = %v, want [Post User]", viewSyms)
	}
	// api.py should have ["User"].
	apiSyms := result["pkg/api.py"]
	if len(apiSyms) != 1 || apiSyms[0] != "User" {
		t.Errorf("api.py symbols = %v, want [User]", apiSyms)
	}
}

func TestGetImporterSymbolsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	result, err := idx.GetImporterSymbols("nonexistent.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestGetSubjectResolutionRate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "app/main.py", "h1", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "app/models.py", "h2", 1, "python", 100); err != nil {
		t.Fatal(err)
	}
	// 2 resolved edges.
	if err = InsertEdgeTx(tx, "app/main.py", "app/models.py", "import", "models", "relative", 1, nil); err != nil {
		t.Fatal(err)
	}
	// 1 not_found unresolved.
	if err = InsertUnresolvedTx(tx, "app/main.py", "missing_mod", "not_found", 5); err != nil {
		t.Fatal(err)
	}
	// 1 external unresolved (should NOT count).
	if err = InsertUnresolvedTx(tx, "app/main.py", "requests", "external", 2); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	resolved, notFound, err := idx.GetSubjectResolutionRate("app/main.py")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != 1 {
		t.Errorf("resolved = %d, want 1", resolved)
	}
	if notFound != 1 {
		t.Errorf("notFound = %d, want 1", notFound)
	}
}

func TestGetSubjectResolutionRateLeafFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "leaf.py", "h1", 1, "python", 10); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	resolved, notFound, err := idx.GetSubjectResolutionRate("leaf.py")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != 0 {
		t.Errorf("resolved = %d, want 0", resolved)
	}
	if notFound != 0 {
		t.Errorf("notFound = %d, want 0", notFound)
	}
}

func TestGetPackageImporterCount(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// Set up: pkg/provider.go and pkg/types.go in same package.
	// consumer/main.go and consumer/app.go import pkg/types.go.
	// pkg/helper.go imports pkg/provider.go (same-package, should be excluded).
	if err = InsertFileTx(tx, "pkg/provider.go", "h1", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/types.go", "h2", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/helper.go", "h3", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "consumer/main.go", "h4", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "consumer/app.go", "h5", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "other/util.go", "h6", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	// Edges: cross-directory imports to pkg/
	if err = InsertEdgeTx(tx, "consumer/main.go", "pkg/types.go", "import", "pkg", "direct", 1, nil); err != nil {
		t.Fatal(err)
	}
	if err = InsertEdgeTx(tx, "consumer/app.go", "pkg/types.go", "import", "pkg", "direct", 1, nil); err != nil {
		t.Fatal(err)
	}
	if err = InsertEdgeTx(tx, "other/util.go", "pkg/provider.go", "import", "pkg", "direct", 1, nil); err != nil {
		t.Fatal(err)
	}
	// Same-package edge (should be excluded from count)
	if err = InsertEdgeTx(tx, "pkg/helper.go", "pkg/provider.go", "import", ".", "direct", 1, nil); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	count, err := idx.GetPackageImporterCount("pkg")
	if err != nil {
		t.Fatal(err)
	}
	// 3 distinct cross-directory importers: consumer/main.go, consumer/app.go, other/util.go
	// pkg/helper.go is excluded (same directory)
	if count != 3 {
		t.Errorf("GetPackageImporterCount(\"pkg\") = %d, want 4", count)
	}
}

func TestGetGoSiblingsInDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")
	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	// Set up: pkg/ has provider.go, types.go, helper.go, provider_test.go, and sub/nested.go
	if err = InsertFileTx(tx, "pkg/provider.go", "h1", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/types.go", "h2", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/helper.go", "h3", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/provider_test.go", "h4", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = InsertFileTx(tx, "pkg/sub/nested.go", "h5", 1, "go", 100); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	siblings, err := idx.GetGoSiblingsInDir("pkg", "pkg/provider.go")
	if err != nil {
		t.Fatal(err)
	}
	// Should return: helper.go, types.go (sorted by path)
	// Excludes: provider.go (self), provider_test.go (test), sub/nested.go (subdirectory)
	if len(siblings) != 2 {
		t.Fatalf("GetGoSiblingsInDir(\"pkg\", \"pkg/provider.go\") returned %d files, want 2: %v", len(siblings), siblings)
	}
	if siblings[0] != "pkg/helper.go" {
		t.Errorf("siblings[0] = %q, want %q", siblings[0], "pkg/helper.go")
	}
	if siblings[1] != "pkg/types.go" {
		t.Errorf("siblings[1] = %q, want %q", siblings[1], "pkg/types.go")
	}
}

func TestGetJavaSiblingsInDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatalf("MigrateToLatest: %v", err)
	}

	// Insert Java files: source, test, sibling, nested subdirectory file.
	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	files := []struct {
		path string
		lang string
	}{
		{"src/main/java/com/example/KafkaProducer.java", "java"},
		{"src/main/java/com/example/KafkaConsumer.java", "java"},
		{"src/main/java/com/example/Utils.java", "java"},
		{"src/main/java/com/example/KafkaProducerTest.java", "java"},  // test file (suffix)
		{"src/main/java/com/example/TestKafkaProducer.java", "java"},  // test file (prefix)
		{"src/main/java/com/example/inner/Nested.java", "java"},       // subdirectory
		{"src/test/java/com/example/KafkaProducerTests.java", "java"}, // test dir
	}
	for _, f := range files {
		if err = InsertFileTx(tx, f.path, "hash", 0, f.lang, 100); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	siblings, err := idx.GetJavaSiblingsInDir("src/main/java/com/example", "src/main/java/com/example/KafkaProducer.java")
	if err != nil {
		t.Fatal(err)
	}
	// Should return: KafkaConsumer.java, Utils.java (sorted by path)
	// Excludes: KafkaProducer.java (self), KafkaProducerTest.java (test suffix),
	//           TestKafkaProducer.java (test prefix), inner/Nested.java (subdirectory)
	if len(siblings) != 2 {
		t.Fatalf("GetJavaSiblingsInDir returned %d files, want 2: %v", len(siblings), siblings)
	}
	if siblings[0] != "src/main/java/com/example/KafkaConsumer.java" {
		t.Errorf("siblings[0] = %q, want KafkaConsumer.java", siblings[0])
	}
	if siblings[1] != "src/main/java/com/example/Utils.java" {
		t.Errorf("siblings[1] = %q, want Utils.java", siblings[1])
	}
}

func TestGetRustSiblingsInDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatalf("MigrateToLatest: %v", err)
	}

	// Insert Rust files: source, test (suffix), sibling, nested subdirectory.
	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	files := []struct {
		path string
		lang string
	}{
		{"crates/bevy_ecs/src/system/function_system.rs", "rust"},
		{"crates/bevy_ecs/src/system/system_param.rs", "rust"},
		{"crates/bevy_ecs/src/system/mod.rs", "rust"},
		{"crates/bevy_ecs/src/system/function_system_test.rs", "rust"}, // test file (suffix)
		{"crates/bevy_ecs/src/system/test_helpers.rs", "rust"},         // test file (prefix)
		{"crates/bevy_ecs/src/system/commands/mod.rs", "rust"},         // subdirectory
	}
	for _, f := range files {
		if err = InsertFileTx(tx, f.path, "hash", 0, f.lang, 100); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	siblings, err := idx.GetRustSiblingsInDir("crates/bevy_ecs/src/system", "crates/bevy_ecs/src/system/function_system.rs")
	if err != nil {
		t.Fatal(err)
	}
	// Should return: mod.rs, system_param.rs (sorted by path)
	// Excludes: function_system.rs (self), function_system_test.rs (test suffix),
	//           test_helpers.rs (test prefix), commands/mod.rs (subdirectory)
	if len(siblings) != 2 {
		t.Fatalf("GetRustSiblingsInDir returned %d files, want 2: %v", len(siblings), siblings)
	}
	if siblings[0] != "crates/bevy_ecs/src/system/mod.rs" {
		t.Errorf("siblings[0] = %q, want mod.rs", siblings[0])
	}
	if siblings[1] != "crates/bevy_ecs/src/system/system_param.rs" {
		t.Errorf("siblings[1] = %q, want system_param.rs", siblings[1])
	}
}

func TestGetTestFilesByName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.sqlite")

	idx, err := OpenIndex(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	if _, err = idx.MigrateToLatest(); err != nil {
		t.Fatal(err)
	}

	tx, err := idx.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a Python project with tests in tests/ directory.
	files := []string{
		"src/transformers/pipelines/__init__.py",
		"src/transformers/configuration_utils.py",
		"tests/test_pipelines.py",
		"tests/test_configuration_utils.py",
		"tests/models/test_bert.py",
		"tests/test_unrelated.py",
		"other/test_pipelines.py",
	}
	for _, f := range files {
		if err = InsertFileTx(tx, f, "abc", 0, "python", 100); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Search for test_pipelines.py under tests/
	results, err := idx.GetTestFilesByName("tests", []string{"test_pipelines.py"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != "tests/test_pipelines.py" {
		t.Errorf("GetTestFilesByName(tests, [test_pipelines.py]) = %v, want [tests/test_pipelines.py]", results)
	}

	// Search for test_bert.py under tests/ (nested)
	results, err = idx.GetTestFilesByName("tests", []string{"test_bert.py"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != "tests/models/test_bert.py" {
		t.Errorf("GetTestFilesByName(tests, [test_bert.py]) = %v, want [tests/models/test_bert.py]", results)
	}

	// Search for multiple basenames
	results, err = idx.GetTestFilesByName("tests", []string{"test_pipelines.py", "test_configuration_utils.py"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("GetTestFilesByName with 2 basenames returned %d results, want 2: %v", len(results), results)
	}

	// Search with empty basenames
	results, err = idx.GetTestFilesByName("tests", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("GetTestFilesByName with nil basenames returned %d results, want 0", len(results))
	}
}
