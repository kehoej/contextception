// Package session parses Claude Code session JSONL files to extract
// tool usage patterns for adoption and discovery analytics.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionDir returns the Claude Code projects directory.
func SessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// EncodedPath converts a repo path to Claude Code's encoded directory name.
// "/Users/kehoe/Repositories/myproject" -> "-Users-kehoe-Repositories-myproject"
func EncodedPath(repoRoot string) string {
	return strings.ReplaceAll(repoRoot, "/", "-")
}

// SessionInfo contains metadata about a parsed session.
type SessionInfo struct {
	ID        string    `json:"id"`
	Date      time.Time `json:"date"`
	Edits     int       `json:"edits"`
	Contexts  int       `json:"contexts"`
	Coverage  float64   `json:"coverage"` // contexts / editable files edited
}

// EditEvent represents a file edit extracted from a session.
type EditEvent struct {
	FilePath  string
	Tool      string // "Edit" or "Write"
	Timestamp time.Time
}

// ContextEvent represents a get_context call from a session.
type ContextEvent struct {
	FilePath  string
	Timestamp time.Time
}

// sessionEntry is the top-level JSON structure of a Claude Code JSONL line.
type sessionEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   *sessionMessage `json:"message"`
}

type sessionMessage struct {
	Content []json.RawMessage `json:"content"`
}

type toolUseContent struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// DiscoverResult contains the output of the discover analysis.
type DiscoverResult struct {
	SessionsScanned int              `json:"sessions_scanned"`
	FilesEdited     int              `json:"files_edited"`
	ContextRequested int             `json:"context_requested"`
	Coverage        float64          `json:"coverage"`
	MissedFiles     []MissedFile     `json:"missed_files,omitempty"`
}

// MissedFile represents a file edited without get_context being called.
type MissedFile struct {
	File        string `json:"file"`
	EditCount   int    `json:"edit_count"`
	ContextCount int   `json:"context_count"`
	IsTest      bool   `json:"is_test,omitempty"`
}

// ListSessions returns session JSONL files for the given repo, sorted by modification time (newest first).
func ListSessions(repoRoot string, since time.Time, limit int) ([]string, error) {
	dir := filepath.Join(SessionDir(), EncodedPath(repoRoot))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading session directory: %w", err)
	}

	type fileWithTime struct {
		path    string
		modTime time.Time
	}

	var files []fileWithTime
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(since) {
			continue
		}
		files = append(files, fileWithTime{
			path:    filepath.Join(dir, e.Name()),
			modTime: info.ModTime(),
		})
	}

	// Sort by modification time, newest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}

	result := make([]string, len(files))
	for i, f := range files {
		result[i] = f.path
	}
	return result, nil
}

// ParseSession extracts edit and context events from a session JSONL file.
func ParseSession(path string, repoRoot string) ([]EditEvent, []ContextEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var edits []EditEvent
	var contexts []ContextEvent

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var entry sessionEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Message == nil {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)

		for _, raw := range entry.Message.Content {
			var tc toolUseContent
			if err := json.Unmarshal(raw, &tc); err != nil {
				continue
			}
			if tc.Type != "tool_use" {
				continue
			}

			switch tc.Name {
			case "Edit", "Write":
				filePath, _ := tc.Input["file_path"].(string)
				if filePath == "" {
					continue
				}
				// Only count files within the repo.
				if !isWithinRepo(filePath, repoRoot) {
					continue
				}
				edits = append(edits, EditEvent{
					FilePath:  makeRelative(filePath, repoRoot),
					Tool:      tc.Name,
					Timestamp: ts,
				})
			}

			// Check for contextception MCP calls.
			if strings.Contains(tc.Name, "contextception") && strings.Contains(tc.Name, "get_context") {
				file, _ := tc.Input["file"].(string)
				if file != "" {
					contexts = append(contexts, ContextEvent{
						FilePath:  makeRelative(file, repoRoot),
						Timestamp: ts,
					})
				}
			}
		}
	}

	return edits, contexts, scanner.Err()
}

// Discover analyzes sessions to find files edited without get_context being called.
func Discover(repoRoot string, since time.Time, includeTests bool, isSupportedExt func(string) bool) (*DiscoverResult, error) {
	sessions, err := ListSessions(repoRoot, since, 0)
	if err != nil {
		return nil, err
	}

	// Aggregate across all sessions.
	editCounts := make(map[string]int)   // file -> edit count
	contextFiles := make(map[string]int) // file -> context count

	for _, sessionPath := range sessions {
		edits, contexts, err := ParseSession(sessionPath, repoRoot)
		if err != nil {
			continue
		}

		for _, e := range edits {
			ext := filepath.Ext(e.FilePath)
			if isSupportedExt != nil && !isSupportedExt(ext) {
				continue
			}
			editCounts[e.FilePath]++
		}

		for _, c := range contexts {
			contextFiles[c.FilePath]++
		}
	}

	// Build missed files list.
	var missed []MissedFile
	filesWithContext := 0
	for file, editCount := range editCounts {
		ctxCount := contextFiles[file]
		if ctxCount > 0 {
			filesWithContext++
		}

		isTest := isTestFile(file)
		if !includeTests && isTest && ctxCount == 0 {
			continue // skip test files from missed list unless --all
		}

		if ctxCount < editCount {
			missed = append(missed, MissedFile{
				File:         file,
				EditCount:    editCount,
				ContextCount: ctxCount,
				IsTest:       isTest,
			})
		}
	}

	// Sort missed files: most edits first.
	sort.Slice(missed, func(i, j int) bool {
		return missed[i].EditCount > missed[j].EditCount
	})

	totalEdited := len(editCounts)
	coverage := 0.0
	if totalEdited > 0 {
		coverage = float64(filesWithContext) / float64(totalEdited) * 100
	}

	return &DiscoverResult{
		SessionsScanned:  len(sessions),
		FilesEdited:      totalEdited,
		ContextRequested: filesWithContext,
		Coverage:         coverage,
		MissedFiles:      missed,
	}, nil
}

// GetSessionStats returns adoption stats per session.
func GetSessionStats(repoRoot string, since time.Time, limit int, isSupportedExt func(string) bool) ([]SessionInfo, error) {
	sessions, err := ListSessions(repoRoot, since, limit)
	if err != nil {
		return nil, err
	}

	var results []SessionInfo
	for _, sessionPath := range sessions {
		edits, contexts, err := ParseSession(sessionPath, repoRoot)
		if err != nil {
			continue
		}

		// Count unique files edited (supported extensions only).
		editedFiles := make(map[string]bool)
		for _, e := range edits {
			ext := filepath.Ext(e.FilePath)
			if isSupportedExt != nil && !isSupportedExt(ext) {
				continue
			}
			editedFiles[e.FilePath] = true
		}

		contextFiles := make(map[string]bool)
		for _, c := range contexts {
			contextFiles[c.FilePath] = true
		}

		editCount := len(editedFiles)
		ctxCount := 0
		for f := range editedFiles {
			if contextFiles[f] {
				ctxCount++
			}
		}

		coverage := 0.0
		if editCount > 0 {
			coverage = float64(ctxCount) / float64(editCount) * 100
		}

		// Extract session ID from filename.
		id := strings.TrimSuffix(filepath.Base(sessionPath), ".jsonl")

		// Get session date from file mod time.
		info, _ := os.Stat(sessionPath)
		date := time.Now()
		if info != nil {
			date = info.ModTime()
		}

		if editCount > 0 { // only show sessions with edits
			results = append(results, SessionInfo{
				ID:       id,
				Date:     date,
				Edits:    editCount,
				Contexts: ctxCount,
				Coverage: coverage,
			})
		}
	}

	return results, nil
}

// isWithinRepo checks if a file path is within the repo root.
func isWithinRepo(filePath, repoRoot string) bool {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	return strings.HasPrefix(abs, repoRoot+"/") || abs == repoRoot
}

// makeRelative converts an absolute path to a repo-relative path.
func makeRelative(filePath, repoRoot string) string {
	if filepath.IsAbs(filePath) {
		if rel, err := filepath.Rel(repoRoot, filePath); err == nil {
			return rel
		}
	}
	return filePath
}

// isTestFile checks if a file path looks like a test file.
func isTestFile(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	return strings.HasPrefix(lower, "test_") ||
		strings.HasSuffix(lower, "_test.go") ||
		strings.HasSuffix(lower, "_test.py") ||
		strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".test.tsx") ||
		strings.HasSuffix(lower, ".test.js") ||
		strings.HasSuffix(lower, ".test.jsx") ||
		strings.HasSuffix(lower, ".spec.ts") ||
		strings.HasSuffix(lower, ".spec.js") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/__tests__/")
}

// FormatDiscoverSummary produces the human-readable discover output.
func FormatDiscoverSummary(result *DiscoverResult) string {
	var b strings.Builder

	b.WriteString("Missed Context Opportunities\n")
	b.WriteString(strings.Repeat("=", 50) + "\n\n")

	fmt.Fprintf(&b, "  Sessions scanned:  %d\n", result.SessionsScanned)
	fmt.Fprintf(&b, "  Files edited:      %d\n", result.FilesEdited)
	fmt.Fprintf(&b, "  Context requested: %d (%.0f%% coverage)\n",
		result.ContextRequested, result.Coverage)

	if len(result.MissedFiles) > 0 {
		b.WriteString("\n  Edits without context:\n")
		b.WriteString("  " + strings.Repeat("-", 46) + "\n")
		for _, f := range result.MissedFiles {
			name := f.File
			if len(name) > 35 {
				name = "..." + name[len(name)-32:]
			}
			status := fmt.Sprintf("Modified %dx", f.EditCount)
			if f.ContextCount > 0 {
				status += fmt.Sprintf(", analyzed %dx", f.ContextCount)
			} else {
				status += ", never analyzed"
			}
			tag := "!!"
			if f.IsTest {
				tag = "  "
			} else if f.ContextCount > 0 {
				tag = "! "
			}
			fmt.Fprintf(&b, "  %-35s %s [%s]\n", name, status, tag)
		}
	}

	if result.SessionsScanned == 0 {
		b.WriteString("\n  No Claude Code sessions found for this repository.\n")
	} else if len(result.MissedFiles) == 0 && result.FilesEdited > 0 {
		b.WriteString("\n  All edited files had context requested. Great coverage!\n")
	} else if result.FilesEdited == 0 {
		b.WriteString("\n  No supported files were edited in recent sessions.\n")
	}

	return b.String()
}

// FormatSessionSummary produces the human-readable session adoption output.
func FormatSessionSummary(sessions []SessionInfo) string {
	var b strings.Builder

	b.WriteString("Contextception Adoption\n")
	b.WriteString(strings.Repeat("=", 50) + "\n\n")

	if len(sessions) == 0 {
		b.WriteString("  No sessions with edits found.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "  %-14s %-12s %5s  %5s  %s\n",
		"Session", "Date", "Edits", "Ctx", "Coverage")
	b.WriteString("  " + strings.Repeat("-", 56) + "\n")

	for _, s := range sessions {
		id := s.ID
		if len(id) > 12 {
			id = id[:8] + "..."
		}

		dateStr := formatRelativeDate(s.Date)

		bar := progressBar(s.Coverage, 10)
		fmt.Fprintf(&b, "  %-14s %-12s %5d  %5d  %3.0f%% %s\n",
			id, dateStr, s.Edits, s.Contexts, s.Coverage, bar)
	}

	return b.String()
}

// formatRelativeDate returns a human-friendly relative date.
func formatRelativeDate(t time.Time) string {
	now := time.Now()
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days == 0:
		return "Today"
	case days == 1:
		return "Yesterday"
	case days < 7:
		return fmt.Sprintf("%dd ago", days)
	default:
		return t.Format("2006-01-02")
	}
}

// progressBar generates a simple progress bar.
func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("|", filled) + strings.Repeat(".", width-filled)
}
