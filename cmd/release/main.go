// Command release automates the deterministic parts of a contextception release:
// version detection, CHANGELOG insertion, commit, tag, and push.
//
// Usage:
//
//	go run ./cmd/release info                          # print release info as JSON
//	go run ./cmd/release --version 1.0.5 --notes "..." # execute release
//	go run ./cmd/release --version 1.0.5 --dry-run     # preview without changes
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type releaseInfo struct {
	LatestTag   string   `json:"latest_tag"`
	NextPatch   string   `json:"next_patch"`
	Commits     []string `json:"commits"`
	CommitCount int      `json:"commit_count"`
	Branch      string   `json:"branch"`
	CleanTree   bool     `json:"clean_tree"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "info" {
		if err := runInfo(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	version := flag.String("version", "", "release version (e.g. 1.0.5)")
	notes := flag.String("notes", "", "changelog markdown text to insert")
	dryRun := flag.Bool("dry-run", false, "preview changes without modifying anything")
	flag.Parse()

	if *version == "" {
		fmt.Fprintln(os.Stderr, "error: --version is required")
		os.Exit(1)
	}
	if *notes == "" && !*dryRun {
		fmt.Fprintln(os.Stderr, "error: --notes is required (or use --dry-run)")
		os.Exit(1)
	}

	if err := runRelease(*version, *notes, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runInfo() error {
	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("detecting branch: %w", err)
	}

	latestTag, err := gitOutput("describe", "--tags", "--abbrev=0")
	if err != nil {
		latestTag = "v0.0.0"
	}

	// Compute next patch version.
	nextPatch := bumpPatch(latestTag)

	// Collect commits since last tag.
	var commits []string
	logOut, err := gitOutput("log", "--oneline", latestTag+"..HEAD")
	if err == nil && logOut != "" {
		commits = strings.Split(logOut, "\n")
	}

	// Check clean tree.
	statusOut, _ := gitOutput("status", "--porcelain")
	cleanTree := statusOut == ""

	info := releaseInfo{
		LatestTag:   latestTag,
		NextPatch:   nextPatch,
		Commits:     commits,
		CommitCount: len(commits),
		Branch:      branch,
		CleanTree:   cleanTree,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}

func runRelease(version, notes string, dryRun bool) error {
	// Validate branch.
	branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("detecting branch: %w", err)
	}
	if branch != "main" {
		return fmt.Errorf("must be on main branch (currently on %s)", branch)
	}

	// Validate clean tree.
	statusOut, _ := gitOutput("status", "--porcelain")
	if statusOut != "" {
		return fmt.Errorf("working tree is dirty — commit or stash changes first")
	}

	// Validate version tag doesn't exist.
	tag := "v" + version
	if _, err := gitOutput("rev-parse", tag); err == nil {
		return fmt.Errorf("tag %s already exists", tag)
	}

	// Read CHANGELOG.md.
	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		return fmt.Errorf("reading CHANGELOG.md: %w", err)
	}

	// Insert new version after [Unreleased].
	today := time.Now().Format("2006-01-02")
	newSection := fmt.Sprintf("## [%s] - %s\n\n%s", version, today, notes)

	content := string(changelog)
	marker := "## [Unreleased]"
	idx := strings.Index(content, marker)
	if idx == -1 {
		return fmt.Errorf("could not find '## [Unreleased]' in CHANGELOG.md")
	}

	// Insert after the [Unreleased] line (and any blank lines following it).
	afterMarker := idx + len(marker)
	rest := content[afterMarker:]

	// Skip whitespace between [Unreleased] and the next section.
	trimmed := strings.TrimLeft(rest, "\n\r ")
	insertPoint := afterMarker + (len(rest) - len(trimmed))

	updated := content[:afterMarker] + "\n\n" + newSection + "\n\n" + content[insertPoint:]

	if dryRun {
		fmt.Println("=== DRY RUN ===")
		fmt.Printf("Version: %s\n", version)
		fmt.Printf("Tag: %s\n", tag)
		fmt.Printf("Date: %s\n", today)
		fmt.Println("\n--- CHANGELOG entry ---")
		fmt.Println(newSection)
		fmt.Println("\n--- Commands that would run ---")
		fmt.Printf("  write CHANGELOG.md\n")
		fmt.Printf("  git add CHANGELOG.md\n")
		fmt.Printf("  git commit -m \"chore: prep %s release\"\n", tag)
		fmt.Printf("  git tag %s\n", tag)
		fmt.Printf("  git push && git push --tags\n")
		return nil
	}

	// Write CHANGELOG.md.
	if err := os.WriteFile("CHANGELOG.md", []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing CHANGELOG.md: %w", err)
	}
	fmt.Println("Updated CHANGELOG.md")

	// Commit.
	if err := gitRun("add", "CHANGELOG.md"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	commitMsg := fmt.Sprintf("chore: prep %s release", tag)
	if err := gitRun("commit", "-m", commitMsg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	fmt.Printf("Committed: %s\n", commitMsg)

	// Tag.
	if err := gitRun("tag", tag); err != nil {
		return fmt.Errorf("git tag: %w", err)
	}
	fmt.Printf("Tagged: %s\n", tag)

	// Push.
	if err := gitRun("push"); err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	if err := gitRun("push", "--tags"); err != nil {
		return fmt.Errorf("git push --tags: %w", err)
	}
	fmt.Printf("Pushed to origin. Release workflow will run at:\n")
	fmt.Printf("  https://github.com/kehoej/contextception/actions\n")

	return nil
}

// bumpPatch takes a tag like "v1.0.4" and returns "1.0.5".
func bumpPatch(tag string) string {
	v := strings.TrimPrefix(tag, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return v + ".1"
	}
	// Parse and increment patch.
	patch := 0
	_, _ = fmt.Sscanf(parts[2], "%d", &patch)
	return fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
