// Package git extracts historical signals (churn, co-change) from git history.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// commitEntry represents a single parsed commit from git log output.
type commitEntry struct {
	Hash  string
	Date  time.Time
	Files []string
}

// Signals holds aggregated git history signals for a repository.
type Signals struct {
	Churn      map[string]int    // file_path -> commit count
	CoChange   map[[2]string]int // ordered pair (a < b) -> frequency
	WindowDays int
	ComputedAt string // ISO 8601 timestamp of HEAD at extraction time
}

// Config configures git signal extraction.
type Config struct {
	RepoRoot      string
	WindowDays    int // default 90
	MaxCommitSize int // default 100
}

// Extract extracts churn and co-change signals from git history.
// Uses a single git log invocation. Returns nil error on success.
// If git is unavailable or the repo has no history, returns an error.
func Extract(cfg Config) (*Signals, error) {
	if cfg.WindowDays <= 0 {
		cfg.WindowDays = 90
	}
	if cfg.MaxCommitSize <= 0 {
		cfg.MaxCommitSize = 100
	}

	// Get HEAD date for deterministic window computation.
	headDate, err := getHeadDate(cfg.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("getting HEAD date: %w", err)
	}

	cutoff := headDate.AddDate(0, 0, -cfg.WindowDays)
	sinceStr := cutoff.Format("2006-01-02T15:04:05-07:00")

	// Single git log invocation.
	out, err := exec.Command("git", "-C", cfg.RepoRoot,
		"log",
		"--format=%H%x00%aI",
		"--name-only",
		"--no-merges",
		"--since="+sinceStr,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("running git log: %w", err)
	}

	commits := parseGitLog(out)

	// Filter large commits.
	var filtered []commitEntry
	for _, c := range commits {
		if len(c.Files) <= cfg.MaxCommitSize {
			filtered = append(filtered, c)
		}
	}

	// Build churn map.
	churn := make(map[string]int)
	for _, c := range filtered {
		for _, f := range c.Files {
			churn[f]++
		}
	}

	// Build co-change map with ordered pairs (a < b).
	coChange := make(map[[2]string]int)
	for _, c := range filtered {
		if len(c.Files) < 2 {
			continue
		}
		// Sort files for deterministic pair ordering.
		sorted := make([]string, len(c.Files))
		copy(sorted, c.Files)
		sort.Strings(sorted)

		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				pair := [2]string{sorted[i], sorted[j]}
				coChange[pair]++
			}
		}
	}

	return &Signals{
		Churn:      churn,
		CoChange:   coChange,
		WindowDays: cfg.WindowDays,
		ComputedAt: headDate.Format(time.RFC3339),
	}, nil
}

// getHeadDate returns the author date of HEAD.
func getHeadDate(repoRoot string) (time.Time, error) {
	out, err := exec.Command("git", "-C", repoRoot, "log", "-1", "--format=%aI").Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("git log -1: %w", err)
	}
	dateStr := strings.TrimSpace(string(out))
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty HEAD date (no commits?)")
	}
	return time.Parse(time.RFC3339, dateStr)
}

// parseGitLog parses the output of git log --format=%H%x00%aI --name-only.
// Format: each commit block is separated by a blank line.
// First line of block: hash\0date
// Subsequent lines: file paths
func parseGitLog(output []byte) []commitEntry {
	if len(bytes.TrimSpace(output)) == 0 {
		return nil
	}

	var commits []commitEntry

	// Split into blocks separated by double newlines.
	// Git separates commit entries with a blank line.
	blocks := splitCommitBlocks(output)

	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}

		// First line: hash\0date
		header := lines[0]
		parts := strings.SplitN(header, "\x00", 2)
		if len(parts) != 2 {
			continue
		}

		hash := parts[0]
		date, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}

		var files []string
		for _, line := range lines[1:] {
			f := strings.TrimSpace(line)
			if f != "" {
				files = append(files, f)
			}
		}

		commits = append(commits, commitEntry{
			Hash:  hash,
			Date:  date,
			Files: files,
		})
	}

	return commits
}

// splitCommitBlocks splits git log output into individual commit blocks.
// Commits are separated by blank lines. The header line contains a null byte
// which distinguishes it from file path lines.
func splitCommitBlocks(output []byte) []string {
	text := string(output)
	lines := strings.Split(text, "\n")

	var blocks []string
	var current []string

	for _, line := range lines {
		// A header line (contains null byte) starts a new block, unless we're at the first one.
		if strings.Contains(line, "\x00") && len(current) > 0 {
			blocks = append(blocks, strings.Join(current, "\n"))
			current = nil
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}

	return blocks
}
