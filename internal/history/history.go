// Package history provides persistent storage for analysis results over time.
// This enables blast radius trends, hotspot evolution, and coupling drift tracking.
//
// History is stored in a separate SQLite database (.contextception/history.sqlite)
// to keep it independent from the index, which gets rebuilt frequently.
package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kehoej/contextception/internal/model"
	_ "modernc.org/sqlite"
)

// Store manages the historical analysis database.
type Store struct {
	db *sql.DB
}

// HistoryFile is the filename of the history database.
const HistoryFile = "history.sqlite"

// HistoryPath returns the path to the history database for a repo.
func HistoryPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".contextception", HistoryFile)
}

// Open opens or creates the history database.
func Open(repoRoot string) (*Store, error) {
	path := HistoryPath(repoRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating history directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening history database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating history database: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates the history tables if they don't exist.
func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS analysis_runs (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
		ref_range     TEXT    NOT NULL,
		commit_sha    TEXT,
		branch        TEXT,
		total_files   INTEGER NOT NULL DEFAULT 0,
		blast_level   TEXT    NOT NULL,
		blast_detail  TEXT,
		fragility     REAL    NOT NULL DEFAULT 0,
		high_risk     INTEGER NOT NULL DEFAULT 0,
		test_gaps     INTEGER NOT NULL DEFAULT 0,
		coupling      INTEGER NOT NULL DEFAULT 0,
		hotspots      INTEGER NOT NULL DEFAULT 0,
		report_json   TEXT
	);

	CREATE TABLE IF NOT EXISTS file_risks (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id      INTEGER NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
		file_path   TEXT    NOT NULL,
		status      TEXT    NOT NULL,
		blast_level TEXT,
		UNIQUE(run_id, file_path)
	);

	CREATE TABLE IF NOT EXISTS hotspot_history (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id      INTEGER NOT NULL REFERENCES analysis_runs(id) ON DELETE CASCADE,
		file_path   TEXT    NOT NULL,
		UNIQUE(run_id, file_path)
	);

	CREATE INDEX IF NOT EXISTS idx_runs_created ON analysis_runs(created_at);
	CREATE INDEX IF NOT EXISTS idx_runs_branch ON analysis_runs(branch);
	CREATE INDEX IF NOT EXISTS idx_file_risks_path ON file_risks(file_path);
	CREATE INDEX IF NOT EXISTS idx_hotspot_path ON hotspot_history(file_path);

	CREATE TABLE IF NOT EXISTS usage_log (
		id                  INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at          TEXT    NOT NULL DEFAULT (datetime('now')),
		source              TEXT    NOT NULL,
		tool                TEXT    NOT NULL,
		files_analyzed      TEXT    NOT NULL,
		file_count          INTEGER NOT NULL DEFAULT 1,
		must_read_count     INTEGER NOT NULL DEFAULT 0,
		likely_modify_count INTEGER NOT NULL DEFAULT 0,
		test_count          INTEGER NOT NULL DEFAULT 0,
		blast_level         TEXT,
		confidence          REAL,
		response_tokens     INTEGER,
		duration_ms         INTEGER,
		mode                TEXT,
		token_budget        INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_usage_log_created ON usage_log(created_at);
	CREATE INDEX IF NOT EXISTS idx_usage_log_tool ON usage_log(tool);

	CREATE TABLE IF NOT EXISTS feedback (
		id                INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at        TEXT    NOT NULL DEFAULT (datetime('now')),
		usage_id          INTEGER REFERENCES usage_log(id),
		file_path         TEXT    NOT NULL,
		usefulness        INTEGER NOT NULL CHECK(usefulness BETWEEN 1 AND 5),
		useful_files      TEXT,
		unnecessary_files TEXT,
		missing_files     TEXT,
		modified_files    TEXT,
		notes             TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_feedback_file ON feedback(file_path);
	CREATE INDEX IF NOT EXISTS idx_feedback_usefulness ON feedback(usefulness);
	`
	_, err := db.Exec(schema)
	return err
}

// RecordRun stores a change report in the history database.
func (s *Store) RecordRun(report *model.ChangeReport, commitSHA, branch string) (int64, error) {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return 0, fmt.Errorf("marshaling report: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	fragility := 0.0
	blastLevel := "low"
	blastDetail := ""
	if report.BlastRadius != nil {
		blastLevel = report.BlastRadius.Level
		blastDetail = report.BlastRadius.Detail
		fragility = report.BlastRadius.Fragility
	}

	result, err := tx.Exec(`
		INSERT INTO analysis_runs
			(ref_range, commit_sha, branch, total_files, blast_level, blast_detail,
			 fragility, high_risk, test_gaps, coupling, hotspots, report_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.RefRange, commitSHA, branch,
		report.Summary.TotalFiles, blastLevel, blastDetail,
		fragility, report.Summary.HighRiskFiles,
		len(report.TestGaps), len(report.Coupling), len(report.Hotspots),
		string(reportJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("inserting run: %w", err)
	}

	runID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting run ID: %w", err)
	}

	// Store per-file risks.
	for _, f := range report.ChangedFiles {
		bl := ""
		if f.BlastRadius != nil {
			bl = f.BlastRadius.Level
		}
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO file_risks (run_id, file_path, status, blast_level)
			VALUES (?, ?, ?, ?)`,
			runID, f.File, f.Status, bl,
		)
		if err != nil {
			return 0, fmt.Errorf("inserting file risk: %w", err)
		}
	}

	// Store hotspots.
	for _, h := range report.Hotspots {
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO hotspot_history (run_id, file_path)
			VALUES (?, ?)`,
			runID, h,
		)
		if err != nil {
			return 0, fmt.Errorf("inserting hotspot: %w", err)
		}
	}

	return runID, tx.Commit()
}

// BlastRadiusTrend returns blast radius levels over time.
type BlastRadiusTrend struct {
	Date   string `json:"date"`
	Level  string `json:"level"`
	Files  int    `json:"files"`
	Branch string `json:"branch,omitempty"`
}

// GetBlastRadiusTrend returns the last N analysis runs' blast radius.
func (s *Store) GetBlastRadiusTrend(limit int, branch string) ([]BlastRadiusTrend, error) {
	query := `
		SELECT created_at, blast_level, total_files, branch
		FROM analysis_runs
		WHERE (? = '' OR branch = ?)
		ORDER BY created_at DESC
		LIMIT ?`

	rows, err := s.db.Query(query, branch, branch, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []BlastRadiusTrend
	for rows.Next() {
		var t BlastRadiusTrend
		if err := rows.Scan(&t.Date, &t.Level, &t.Files, &t.Branch); err != nil {
			return nil, err
		}
		trends = append(trends, t)
	}
	return trends, rows.Err()
}

// HotspotFrequency represents how often a file appears as a hotspot.
type HotspotFrequency struct {
	File     string `json:"file"`
	Count    int    `json:"count"`
	LastSeen string `json:"last_seen"`
}

// GetHotspotEvolution returns files that appear most frequently as hotspots.
func (s *Store) GetHotspotEvolution(limit int, since time.Time) ([]HotspotFrequency, error) {
	query := `
		SELECT h.file_path, COUNT(*) as cnt, MAX(r.created_at) as last_seen
		FROM hotspot_history h
		JOIN analysis_runs r ON h.run_id = r.id
		WHERE r.created_at >= ?
		GROUP BY h.file_path
		ORDER BY cnt DESC
		LIMIT ?`

	rows, err := s.db.Query(query, since.Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HotspotFrequency
	for rows.Next() {
		var h HotspotFrequency
		if err := rows.Scan(&h.File, &h.Count, &h.LastSeen); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// FileRiskHistory shows how a file's blast radius has changed over time.
type FileRiskHistory struct {
	Date       string `json:"date"`
	BlastLevel string `json:"blast_level"`
	Status     string `json:"status"`
}

// GetFileRiskHistory returns the risk history for a specific file.
func (s *Store) GetFileRiskHistory(filePath string, limit int) ([]FileRiskHistory, error) {
	query := `
		SELECT r.created_at, f.blast_level, f.status
		FROM file_risks f
		JOIN analysis_runs r ON f.run_id = r.id
		WHERE f.file_path = ?
		ORDER BY r.created_at DESC
		LIMIT ?`

	rows, err := s.db.Query(query, filePath, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FileRiskHistory
	for rows.Next() {
		var h FileRiskHistory
		if err := rows.Scan(&h.Date, &h.BlastLevel, &h.Status); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}

// BlastDistribution shows what percentage of runs had each blast level.
type BlastDistribution struct {
	Level   string  `json:"level"`
	Count   int     `json:"count"`
	Percent float64 `json:"percent"`
}

// GetBlastDistribution returns the distribution of blast radius levels.
func (s *Store) GetBlastDistribution(since time.Time) ([]BlastDistribution, error) {
	query := `
		SELECT blast_level, COUNT(*) as cnt
		FROM analysis_runs
		WHERE created_at >= ?
		GROUP BY blast_level
		ORDER BY
			CASE blast_level
				WHEN 'high' THEN 1
				WHEN 'medium' THEN 2
				WHEN 'low' THEN 3
			END`

	rows, err := s.db.Query(query, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BlastDistribution
	total := 0
	for rows.Next() {
		var d BlastDistribution
		if err := rows.Scan(&d.Level, &d.Count); err != nil {
			return nil, err
		}
		total += d.Count
		results = append(results, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range results {
		if total > 0 {
			results[i].Percent = float64(results[i].Count) / float64(total) * 100
		}
	}
	return results, nil
}

// RunCount returns the total number of recorded runs.
func (s *Store) RunCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM analysis_runs").Scan(&count)
	return count, err
}

// UsageEntry represents a single usage tracking record.
type UsageEntry struct {
	Source            string   // "mcp", "cli", "hook"
	Tool              string   // "get_context", "analyze", "analyze_change"
	FilesAnalyzed     []string // file paths analyzed
	MustReadCount     int
	LikelyModifyCount int
	TestCount         int
	BlastLevel        string
	Confidence        float64
	HasConfidence     bool // true for analyze/get_context, false for analyze_change
	ResponseTokens    int
	DurationMs        int64
	Mode              string
	TokenBudget       int
}

// RecordUsage stores a usage log entry.
func (s *Store) RecordUsage(entry *UsageEntry) (int64, error) {
	filesJSON, err := json.Marshal(entry.FilesAnalyzed)
	if err != nil {
		return 0, fmt.Errorf("marshaling files: %w", err)
	}

	// Store NULL confidence for tools that don't produce one (e.g. analyze_change).
	var confidence any
	if entry.HasConfidence {
		confidence = entry.Confidence
	}

	result, err := s.db.Exec(`
		INSERT INTO usage_log
			(source, tool, files_analyzed, file_count, must_read_count,
			 likely_modify_count, test_count, blast_level, confidence,
			 response_tokens, duration_ms, mode, token_budget)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Source, entry.Tool, string(filesJSON), len(entry.FilesAnalyzed),
		entry.MustReadCount, entry.LikelyModifyCount, entry.TestCount,
		entry.BlastLevel, confidence, entry.ResponseTokens,
		entry.DurationMs, entry.Mode, entry.TokenBudget,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting usage: %w", err)
	}
	return result.LastInsertId()
}

// UsageSummary contains aggregate usage statistics.
type UsageSummary struct {
	ContextAnalyses  int     `json:"context_analyses"`  // analyze + get_context
	ChangeAnalyses   int     `json:"change_analyses"`   // analyze_change
	TotalAnalyses    int     `json:"total_analyses"`    // all tools combined
	FilesAnalyzed    int     `json:"files_analyzed"`    // unique files from context analyses
	AvgConfidence    float64 `json:"avg_confidence"`    // from context analyses only (NULLs excluded)
	AvgDurationMs    float64 `json:"avg_duration_ms"`
	BlastLevelCounts map[string]int `json:"blast_level_counts"`
}

// sqliteDatetime formats a time for SQLite comparison (matches datetime('now') format).
const sqliteDatetimeFmt = "2006-01-02 15:04:05"

// GetUsageSummary returns aggregate usage statistics since the given time.
func (s *Store) GetUsageSummary(since time.Time) (*UsageSummary, error) {
	sinceStr := since.UTC().Format(sqliteDatetimeFmt)

	summary := &UsageSummary{
		BlastLevelCounts: make(map[string]int),
	}

	// Overall counts.
	err := s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(AVG(duration_ms), 0)
		FROM usage_log WHERE created_at >= ?`, sinceStr,
	).Scan(&summary.TotalAnalyses, &summary.AvgDurationMs)
	if err != nil {
		return nil, err
	}

	// Context analyses only (analyze + get_context): file counts and confidence.
	// AVG(confidence) naturally skips NULL rows (analyze_change stores NULL).
	err = s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(file_count), 0), COALESCE(AVG(confidence), 0)
		FROM usage_log
		WHERE created_at >= ? AND tool != 'analyze_change'`, sinceStr,
	).Scan(&summary.ContextAnalyses, &summary.FilesAnalyzed, &summary.AvgConfidence)
	if err != nil {
		return nil, err
	}

	summary.ChangeAnalyses = summary.TotalAnalyses - summary.ContextAnalyses

	rows, err := s.db.Query(`
		SELECT COALESCE(blast_level, 'unknown'), COUNT(*)
		FROM usage_log WHERE created_at >= ?
		GROUP BY blast_level`, sinceStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var level string
		var count int
		if err := rows.Scan(&level, &count); err != nil {
			return nil, err
		}
		summary.BlastLevelCounts[level] = count
	}
	return summary, rows.Err()
}

// UsagePeriod represents usage data for a time period.
type UsagePeriod struct {
	Period        string  `json:"period"`
	Analyses      int     `json:"analyses"`
	Files         int     `json:"files"`
	AvgConfidence float64 `json:"avg_confidence"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
}

// GetUsageByPeriod returns usage statistics grouped by the given period.
// period must be "day", "week", or "month".
func (s *Store) GetUsageByPeriod(period string, since time.Time, limit int) ([]UsagePeriod, error) {
	sinceStr := since.UTC().Format(sqliteDatetimeFmt)

	var groupExpr string
	switch period {
	case "week":
		groupExpr = "strftime('%Y-W%W', created_at)"
	case "month":
		groupExpr = "strftime('%Y-%m', created_at)"
	default: // "day"
		groupExpr = "date(created_at)"
	}

	//nolint:gosec // groupExpr is from hardcoded switch, not user input
	query := fmt.Sprintf(`
		SELECT %s as period, COUNT(*), COALESCE(SUM(file_count), 0),
		       COALESCE(AVG(confidence), 0), COALESCE(AVG(duration_ms), 0)
		FROM usage_log
		WHERE created_at >= ?
		GROUP BY period
		ORDER BY period DESC
		LIMIT ?`, groupExpr)

	rows, err := s.db.Query(query, sinceStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UsagePeriod
	for rows.Next() {
		var p UsagePeriod
		if err := rows.Scan(&p.Period, &p.Analyses, &p.Files,
			&p.AvgConfidence, &p.AvgDurationMs); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// TopFile represents a frequently analyzed file.
type TopFile struct {
	File     string `json:"file"`
	Count    int    `json:"count"`
	LastSeen string `json:"last_seen"`
}

// GetTopFiles returns the most frequently analyzed files.
func (s *Store) GetTopFiles(since time.Time, limit int) ([]TopFile, error) {
	sinceStr := since.UTC().Format(sqliteDatetimeFmt)

	rows, err := s.db.Query(`
		SELECT files_analyzed, COUNT(*) as cnt, MAX(created_at) as last_seen
		FROM usage_log
		WHERE created_at >= ? AND tool != 'analyze_change'
		GROUP BY files_analyzed
		ORDER BY cnt DESC
		LIMIT ?`, sinceStr, limit*3) // fetch extra since we need to expand JSON arrays
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Aggregate counts by individual file path (files_analyzed is a JSON array).
	fileCounts := make(map[string]*TopFile)
	for rows.Next() {
		var filesJSON string
		var count int
		var lastSeen string
		if err := rows.Scan(&filesJSON, &count, &lastSeen); err != nil {
			return nil, err
		}

		var files []string
		if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
			continue
		}
		for _, f := range files {
			if existing, ok := fileCounts[f]; ok {
				existing.Count += count
				if lastSeen > existing.LastSeen {
					existing.LastSeen = lastSeen
				}
			} else {
				fileCounts[f] = &TopFile{File: f, Count: count, LastSeen: lastSeen}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by count descending.
	result := make([]TopFile, 0, len(fileCounts))
	for _, tf := range fileCounts {
		result = append(result, *tf)
	}
	sortTopFiles(result)
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// sortTopFiles sorts TopFile entries by count descending, then path ascending for stability.
func sortTopFiles(files []TopFile) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Count != files[j].Count {
			return files[i].Count > files[j].Count
		}
		return files[i].File < files[j].File
	})
}

// UsageCount returns the total number of usage log entries.
func (s *Store) UsageCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM usage_log").Scan(&count)
	return count, err
}

// FilesWithUsage returns the set of files that have usage_log entries since the given time.
// This satisfies the session.UsageLogQuerier interface.
func (s *Store) FilesWithUsage(since time.Time) (map[string]bool, error) {
	sinceStr := since.UTC().Format(sqliteDatetimeFmt)

	rows, err := s.db.Query(`
		SELECT files_analyzed FROM usage_log
		WHERE created_at >= ?`, sinceStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var filesJSON string
		if err := rows.Scan(&filesJSON); err != nil {
			continue
		}
		var files []string
		if err := json.Unmarshal([]byte(filesJSON), &files); err != nil {
			continue
		}
		for _, f := range files {
			result[f] = true
		}
	}
	return result, rows.Err()
}

// UsageEntryFromAnalysis creates a UsageEntry from an analysis output.
func UsageEntryFromAnalysis(source, tool string, files []string, output *model.AnalysisOutput, durationMs int64, mode string, tokenBudget int) *UsageEntry {
	entry := &UsageEntry{
		Source:        source,
		Tool:          tool,
		FilesAnalyzed: files,
		DurationMs:    durationMs,
		Mode:          mode,
		TokenBudget:   tokenBudget,
	}

	if output != nil {
		entry.MustReadCount = len(output.MustRead)
		entry.TestCount = len(output.Tests)
		entry.Confidence = output.Confidence
		entry.HasConfidence = true

		// Count likely_modify entries across all packages.
		for _, entries := range output.LikelyModify {
			entry.LikelyModifyCount += len(entries)
		}

		if output.BlastRadius != nil {
			entry.BlastLevel = output.BlastRadius.Level
		}

		// Estimate response tokens (rough: JSON bytes / 4).
		if data, err := json.Marshal(output); err == nil {
			entry.ResponseTokens = len(data) / 4
		}
	}

	return entry
}

// UsageEntryFromChangeReport creates a UsageEntry from a change report.
func UsageEntryFromChangeReport(source string, files []string, report *model.ChangeReport, durationMs int64, mode string, tokenBudget int) *UsageEntry {
	entry := &UsageEntry{
		Source:        source,
		Tool:          "analyze_change",
		FilesAnalyzed: files,
		DurationMs:    durationMs,
		Mode:          mode,
		TokenBudget:   tokenBudget,
	}

	if report != nil {
		entry.MustReadCount = len(report.MustRead)
		entry.TestCount = len(report.Tests)

		for _, entries := range report.LikelyModify {
			entry.LikelyModifyCount += len(entries)
		}

		if report.BlastRadius != nil {
			entry.BlastLevel = report.BlastRadius.Level
		}

		// Estimate response tokens.
		if data, err := json.Marshal(report); err == nil {
			entry.ResponseTokens = len(data) / 4
		}

		// Extract changed file paths for the files_analyzed field.
		if len(files) == 0 {
			for _, cf := range report.ChangedFiles {
				entry.FilesAnalyzed = append(entry.FilesAnalyzed, cf.File)
			}
		}
	}

	return entry
}

// GainExport represents the full analytics export for JSON/CSV output.
type GainExport struct {
	Summary *UsageSummary `json:"summary"`
	TopFiles []TopFile    `json:"top_files"`
	Daily   []UsagePeriod `json:"daily,omitempty"`
	Weekly  []UsagePeriod `json:"weekly,omitempty"`
	Monthly []UsagePeriod `json:"monthly,omitempty"`
}

// FormatGainSummary produces the human-readable gain dashboard text.
func FormatGainSummary(summary *UsageSummary, topFiles []TopFile, daily []UsagePeriod) string {
	var b strings.Builder

	b.WriteString("Contextception Analytics\n")
	b.WriteString(strings.Repeat("=", 50) + "\n\n")

	// KPIs
	avgBlast := dominantBlastLevel(summary.BlastLevelCounts)
	fmt.Fprintf(&b, "  Context analyses: %-10d Avg confidence:  %.2f\n",
		summary.ContextAnalyses, summary.AvgConfidence)
	fmt.Fprintf(&b, "  Files analyzed:   %-10d Avg blast radius: %s\n",
		summary.FilesAnalyzed, avgBlast)
	if summary.ChangeAnalyses > 0 {
		fmt.Fprintf(&b, "  Change analyses:  %d\n", summary.ChangeAnalyses)
	}
	fmt.Fprintf(&b, "  Avg duration:     %dms\n",
		int(summary.AvgDurationMs))

	// Top files
	if len(topFiles) > 0 {
		b.WriteString("\n  Top analyzed files:\n")
		b.WriteString("  " + strings.Repeat("-", 46) + "\n")
		for _, f := range topFiles {
			name := f.File
			if len(name) > 35 {
				name = "..." + name[len(name)-32:]
			}
			fmt.Fprintf(&b, "  %-37s %3d analyses\n", name, f.Count)
		}
	}

	// Daily trend
	if len(daily) > 0 {
		b.WriteString("\n  Daily activity:\n")
		b.WriteString("  " + strings.Repeat("-", 46) + "\n")
		// Show in chronological order (daily is DESC, reverse it).
		for i := len(daily) - 1; i >= 0; i-- {
			d := daily[i]
			bar := strings.Repeat("|", min(d.Analyses, 30))
			fmt.Fprintf(&b, "  %-12s %3d  %s\n", d.Period, d.Analyses, bar)
		}
	}

	if summary.TotalAnalyses == 0 {
		b.WriteString("\n  No analyses recorded yet. Use contextception analyze or get_context to start tracking.\n")
	}

	return b.String()
}

// FeedbackEntry represents a single feedback record from the LLM.
type FeedbackEntry struct {
	FilePath         string   `json:"file_path"`
	Usefulness       int      `json:"usefulness"`
	UsefulFiles      []string `json:"useful_files,omitempty"`
	UnnecessaryFiles []string `json:"unnecessary_files,omitempty"`
	MissingFiles     []string `json:"missing_files,omitempty"`
	ModifiedFiles    []string `json:"modified_files,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

// RecordFeedback stores an LLM feedback entry, linking to the most recent usage_log for the file.
func (s *Store) RecordFeedback(entry *FeedbackEntry) (int64, error) {
	// Find the most recent usage_log entry for this file.
	// Use JSON-quoted path to avoid false matches (e.g. "db.go" matching "db_test.go").
	var usageID sql.NullInt64
	_ = s.db.QueryRow(`
		SELECT id FROM usage_log
		WHERE files_analyzed LIKE ?
		ORDER BY created_at DESC LIMIT 1`,
		`%"`+entry.FilePath+`"%`,
	).Scan(&usageID)

	usefulJSON, _ := json.Marshal(entry.UsefulFiles)
	unnecessaryJSON, _ := json.Marshal(entry.UnnecessaryFiles)
	missingJSON, _ := json.Marshal(entry.MissingFiles)
	modifiedJSON, _ := json.Marshal(entry.ModifiedFiles)

	result, err := s.db.Exec(`
		INSERT INTO feedback
			(usage_id, file_path, usefulness, useful_files,
			 unnecessary_files, missing_files, modified_files, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		usageID, entry.FilePath, entry.Usefulness,
		string(usefulJSON), string(unnecessaryJSON),
		string(missingJSON), string(modifiedJSON),
		entry.Notes,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting feedback: %w", err)
	}
	return result.LastInsertId()
}

// AccuracyMetrics contains precision/recall metrics computed from feedback.
type AccuracyMetrics struct {
	TotalRatings       int     `json:"total_ratings"`
	AvgUsefulness      float64 `json:"avg_usefulness"`
	MustReadPrecision  float64 `json:"must_read_precision"`
	MustReadRecall     float64 `json:"must_read_recall"`
	LikelyModifyAccuracy float64 `json:"likely_modify_accuracy"`
}

// GetAccuracyMetrics computes precision/recall from feedback data.
func (s *Store) GetAccuracyMetrics(since time.Time) (*AccuracyMetrics, error) {
	sinceStr := since.UTC().Format(sqliteDatetimeFmt)

	metrics := &AccuracyMetrics{}

	// Get basic stats.
	err := s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(AVG(usefulness), 0)
		FROM feedback WHERE created_at >= ?`, sinceStr,
	).Scan(&metrics.TotalRatings, &metrics.AvgUsefulness)
	if err != nil {
		return nil, err
	}

	if metrics.TotalRatings == 0 {
		return metrics, nil
	}

	// Compute precision and recall from file lists.
	rows, err := s.db.Query(`
		SELECT f.useful_files, f.unnecessary_files, f.missing_files, f.modified_files
		FROM feedback f
		WHERE f.created_at >= ?`, sinceStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var totalUseful, totalUnnecessary, totalMissing int
	var totalModified int

	for rows.Next() {
		var usefulJSON, unnecessaryJSON, missingJSON, modifiedJSON sql.NullString
		if err := rows.Scan(&usefulJSON, &unnecessaryJSON, &missingJSON,
			&modifiedJSON); err != nil {
			return nil, err
		}

		totalUseful += countJSONArray(usefulJSON)
		totalUnnecessary += countJSONArray(unnecessaryJSON)
		totalMissing += countJSONArray(missingJSON)
		totalModified += countJSONArray(modifiedJSON)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Likely-modify denominator: sum likely_modify_count from distinct usage entries
	// to avoid inflating the count when multiple feedbacks link to the same analysis.
	var totalLikelyModify int
	_ = s.db.QueryRow(`
		SELECT COALESCE(SUM(u.likely_modify_count), 0)
		FROM usage_log u
		WHERE u.id IN (SELECT DISTINCT usage_id FROM feedback WHERE created_at >= ? AND usage_id IS NOT NULL)`,
		sinceStr,
	).Scan(&totalLikelyModify)

	// Must-read precision: useful / (useful + unnecessary)
	if totalUseful+totalUnnecessary > 0 {
		metrics.MustReadPrecision = float64(totalUseful) / float64(totalUseful+totalUnnecessary)
	}

	// Must-read recall: useful / (useful + missing)
	if totalUseful+totalMissing > 0 {
		metrics.MustReadRecall = float64(totalUseful) / float64(totalUseful+totalMissing)
	}

	// Likely-modify accuracy: modified / predicted
	if totalLikelyModify > 0 {
		metrics.LikelyModifyAccuracy = float64(totalModified) / float64(totalLikelyModify)
	}

	return metrics, nil
}

// countJSONArray returns the number of elements in a JSON array string.
func countJSONArray(s sql.NullString) int {
	if !s.Valid || s.String == "" || s.String == "null" {
		return 0
	}
	var arr []string
	if err := json.Unmarshal([]byte(s.String), &arr); err != nil {
		return 0
	}
	return len(arr)
}

// RatedAnalysis represents a feedback entry with its rating for display.
type RatedAnalysis struct {
	FilePath   string  `json:"file_path"`
	Usefulness int     `json:"usefulness"`
	Notes      string  `json:"notes,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

// GetLowestRated returns the lowest-rated analyses.
func (s *Store) GetLowestRated(since time.Time, limit int) ([]RatedAnalysis, error) {
	sinceStr := since.UTC().Format(sqliteDatetimeFmt)

	rows, err := s.db.Query(`
		SELECT file_path, usefulness, COALESCE(notes, ''), created_at
		FROM feedback
		WHERE created_at >= ?
		ORDER BY usefulness ASC, created_at DESC
		LIMIT ?`, sinceStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RatedAnalysis
	for rows.Next() {
		var r RatedAnalysis
		if err := rows.Scan(&r.FilePath, &r.Usefulness, &r.Notes, &r.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// FeedbackCount returns the total number of feedback entries.
func (s *Store) FeedbackCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM feedback").Scan(&count)
	return count, err
}

// AccuracyExport represents the full accuracy export for JSON output.
type AccuracyExport struct {
	Metrics    *AccuracyMetrics `json:"metrics"`
	LowestRated []RatedAnalysis `json:"lowest_rated,omitempty"`
}

// FormatAccuracySummary produces the human-readable accuracy dashboard text.
func FormatAccuracySummary(metrics *AccuracyMetrics, lowestRated []RatedAnalysis) string {
	var b strings.Builder

	b.WriteString("Recommendation Accuracy")
	fmt.Fprintf(&b, " (%d rated analyses)\n", metrics.TotalRatings)
	b.WriteString(strings.Repeat("=", 50) + "\n\n")

	if metrics.TotalRatings == 0 {
		b.WriteString("  No feedback recorded yet.\n")
		b.WriteString("  Use the rate_context MCP tool after get_context to start tracking accuracy.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "  Must-read precision:    %3.0f%%  (recommended files that were useful)\n",
		metrics.MustReadPrecision*100)
	fmt.Fprintf(&b, "  Must-read recall:       %3.0f%%  (needed files that were recommended)\n",
		metrics.MustReadRecall*100)
	fmt.Fprintf(&b, "  Likely-modify accuracy: %3.0f%%  (predicted modifications that happened)\n",
		metrics.LikelyModifyAccuracy*100)

	stars := strings.Repeat("*", int(metrics.AvgUsefulness+0.5))
	fmt.Fprintf(&b, "  Overall usefulness:     %s %.1f / 5.0\n", stars, metrics.AvgUsefulness)

	if len(lowestRated) > 0 {
		b.WriteString("\n  Lowest-rated analyses:\n")
		b.WriteString("  " + strings.Repeat("-", 46) + "\n")
		for _, r := range lowestRated {
			name := r.FilePath
			if len(name) > 30 {
				name = "..." + name[len(name)-27:]
			}
			line := fmt.Sprintf("  %-32s *%d", name, r.Usefulness)
			if r.Notes != "" {
				notes := r.Notes
				if len(notes) > 40 {
					notes = notes[:37] + "..."
				}
				line += fmt.Sprintf("  %q", notes)
			}
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}

// dominantBlastLevel returns the most common blast level.
func dominantBlastLevel(counts map[string]int) string {
	if len(counts) == 0 {
		return "n/a"
	}
	best := ""
	bestCount := 0
	for level, count := range counts {
		if count > bestCount {
			best = level
			bestCount = count
		}
	}
	return best
}

