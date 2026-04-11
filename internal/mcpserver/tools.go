package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kehoej/contextception/internal/analyzer"
	"github.com/kehoej/contextception/internal/change"
	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/grader"
	"github.com/kehoej/contextception/internal/history"
	"github.com/kehoej/contextception/internal/indexer"
	"github.com/kehoej/contextception/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// getContextInput is the input schema for the get_context tool.
// File accepts a single path (string) or multiple paths ([]string) for multi-file analysis.
type getContextInput struct {
	File              any    `json:"file" jsonschema:"Repo-relative or absolute path to the file to analyze"`
	OmitExternal      bool   `json:"omit_external,omitempty" jsonschema:"Omit external dependencies from output"`
	IncludeSignatures bool   `json:"include_signatures,omitempty" jsonschema:"Include code signatures for must_read symbols"`
	MaxMustRead       int    `json:"max_must_read,omitempty" jsonschema:"Max must_read entries (default 10)"`
	MaxRelated        int    `json:"max_related,omitempty" jsonschema:"Max related entries (default 10)"`
	MaxLikelyModify   int    `json:"max_likely_modify,omitempty" jsonschema:"Max likely_modify entries (default 15)"`
	MaxTests          int    `json:"max_tests,omitempty" jsonschema:"Max test entries (default 5)"`
	Mode              string `json:"mode,omitempty" jsonschema:"Workflow mode: plan, implement, review (adjusts caps)"`
	TokenBudget       int    `json:"token_budget,omitempty" jsonschema:"Target token budget for output (adjusts caps automatically)"`
}

// parseFiles extracts file paths from the File field, which may be a string or []string.
func (in *getContextInput) parseFiles() ([]string, error) {
	switch v := in.File.(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("file parameter is required")
		}
		return []string{v}, nil
	case []any:
		if len(v) == 0 {
			return nil, fmt.Errorf("file parameter is required")
		}
		files := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("file array element %d is not a string", i)
			}
			if s == "" {
				return nil, fmt.Errorf("file array element %d is empty", i)
			}
			files[i] = s
		}
		return files, nil
	case nil:
		return nil, fmt.Errorf("file parameter is required")
	default:
		return nil, fmt.Errorf("file parameter must be a string or array of strings")
	}
}

// indexInput is the input schema for the index tool (no parameters).
type indexInput struct{}

// statusInput is the input schema for the status tool (no parameters).
type statusInput struct{}

// indexResult is the structured output of the index tool.
type indexResult struct {
	Files      int     `json:"files"`
	Edges      int     `json:"edges"`
	DurationMs float64 `json:"duration_ms"`
}

// configSummary is the config portion of the status result.
type configSummary struct {
	Entrypoints int `json:"entrypoints"`
	Ignore      int `json:"ignore"`
	Generated   int `json:"generated"`
}

// statusResult is the structured output of the status tool.
type statusResult struct {
	Indexed            bool           `json:"indexed"`
	Files              int            `json:"files"`
	Edges              int            `json:"edges"`
	Unresolved         int            `json:"unresolved"`
	UnresolvedByReason map[string]int `json:"unresolved_by_reason"`
	LastIndexedAt      string         `json:"last_indexed_at"`
	LastCommit         string         `json:"last_commit"`
	RepoProfile        string         `json:"repo_profile,omitempty"`
	Config             *configSummary `json:"config,omitempty"`
}

// handleGetContext analyzes one or more files and returns the context bundle.
// Auto-indexes the repository if no index exists.
func (cs *ContextServer) handleGetContext(ctx context.Context, req *mcp.CallToolRequest, input getContextInput) (*mcp.CallToolResult, any, error) {
	files, err := input.parseFiles()
	if err != nil {
		return errorResult(err.Error()), nil, nil
	}

	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	// Always run incremental index to pick up any changes (committed or working tree).
	// This is fast (no-op when nothing changed, incremental when files changed).
	if err := cs.runIndex(); err != nil {
		return errorResult(fmt.Sprintf("auto-indexing failed: %v", err)), nil, nil
	}

	// Resolve all paths to repo-relative.
	targets := make([]string, len(files))
	for i, f := range files {
		target := f
		if filepath.IsAbs(target) {
			rel, err := filepath.Rel(cs.repoRoot, target)
			if err != nil {
				return errorResult(fmt.Sprintf("resolving path: %v", err)), nil, nil
			}
			target = rel
		}
		targets[i] = strings.ReplaceAll(filepath.Clean(target), string(os.PathSeparator), "/")
	}

	// Run analysis.
	a := analyzer.New(idx, analyzer.Config{
		RepoConfig:   cs.ensureConfig(),
		OmitExternal: input.OmitExternal,
		Signatures:   input.IncludeSignatures,
		RepoRoot:     cs.repoRoot,
		Caps: analyzer.Caps{
			MaxMustRead:     input.MaxMustRead,
			MaxRelated:      input.MaxRelated,
			MaxLikelyModify: input.MaxLikelyModify,
			MaxTests:        input.MaxTests,
			Mode:            input.Mode,
			TokenBudget:     input.TokenBudget,
		},
	})

	start := time.Now()
	var output *model.AnalysisOutput
	if len(targets) == 1 {
		output, err = a.Analyze(targets[0])
	} else {
		output, err = a.AnalyzeMulti(targets)
	}
	if err != nil {
		return errorResult(fmt.Sprintf("analysis failed: %v", err)), nil, nil
	}
	durationMs := time.Since(start).Milliseconds()

	// Record usage analytics (best-effort).
	if hist, hErr := history.Open(cs.repoRoot); hErr == nil {
		entry := history.UsageEntryFromAnalysis("mcp", "get_context", targets, output, durationMs, input.Mode, input.TokenBudget)
		_, _ = hist.RecordUsage(entry)
		hist.Close()
	}

	return jsonResult(output), nil, nil
}

// handleIndex builds or updates the repository index.
func (cs *ContextServer) handleIndex(ctx context.Context, req *mcp.CallToolRequest, input indexInput) (*mcp.CallToolResult, any, error) {
	start := time.Now()

	if err := cs.runIndex(); err != nil {
		return errorResult(fmt.Sprintf("indexing failed: %v", err)), nil, nil
	}

	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	fileCount, _ := idx.FileCount()
	edgeCount, _ := idx.EdgeCount()
	elapsed := time.Since(start)

	result := indexResult{
		Files:      fileCount,
		Edges:      edgeCount,
		DurationMs: float64(elapsed.Milliseconds()),
	}

	return jsonResult(result), nil, nil
}

// handleStatus returns index diagnostics.
func (cs *ContextServer) handleStatus(ctx context.Context, req *mcp.CallToolRequest, input statusInput) (*mcp.CallToolResult, any, error) {
	idxPath := db.IndexPath(cs.repoRoot)

	// Check if index exists at all.
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		return jsonResult(statusResult{Indexed: false}), nil, nil
	}

	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	fileCount, _ := idx.FileCount()
	edgeCount, _ := idx.EdgeCount()
	unresolvedSummary, _ := idx.GetUnresolvedSummary()
	lastIndexed, _ := idx.GetMeta("last_indexed_at")
	lastCommit, _ := idx.GetMeta("last_indexed_commit")

	if len(lastCommit) > 12 {
		lastCommit = lastCommit[:12]
	}

	repoProfile, _ := idx.GetMeta("repo_profile")

	result := statusResult{
		Indexed:            fileCount > 0,
		Files:              fileCount,
		Edges:              edgeCount,
		Unresolved:         unresolvedSummary.Total,
		UnresolvedByReason: unresolvedSummary.ByReason,
		LastIndexedAt:      lastIndexed,
		LastCommit:         lastCommit,
		RepoProfile:        repoProfile,
	}

	cfg := cs.ensureConfig()
	if cfg.Version > 0 {
		result.Config = &configSummary{
			Entrypoints: len(cfg.Entrypoints),
			Ignore:      len(cfg.Ignore),
			Generated:   len(cfg.Generated),
		}
	}

	return jsonResult(result), nil, nil
}

// runIndex creates and runs the indexer.
func (cs *ContextServer) runIndex() error {
	idx, err := cs.ensureIndex()
	if err != nil {
		return err
	}

	ix, err := indexer.NewIndexer(indexer.Config{
		RepoRoot: cs.repoRoot,
		Index:    idx,
		Verbose:  false,
		Output:   logWriter(),
		Config:   cs.ensureConfig(),
	})
	if err != nil {
		return fmt.Errorf("creating indexer: %w", err)
	}

	return ix.Run()
}

// --- Search/Discovery Tools ---

// searchInput is the input schema for the search tool.
type searchInput struct {
	Query string `json:"query" jsonschema:"Search query string"`
	Type  string `json:"type,omitempty" jsonschema:"Search type: path or symbol (default: path)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Max results to return (default 50, max 100)"`
}

// searchResultEntry is a single file in search results.
type searchResultEntry struct {
	File      string `json:"file"`
	Language  string `json:"language"`
	SizeBytes int64  `json:"size_bytes"`
	Indegree  int    `json:"indegree"`
	Outdegree int    `json:"outdegree"`
	Role      string `json:"role,omitempty"`
}

// searchOutput is the JSON output of the search tool.
type searchOutput struct {
	Query   string              `json:"query"`
	Type    string              `json:"type"`
	Results []searchResultEntry `json:"results"`
	Count   int                 `json:"count"`
}

// handleSearch searches the index for files by path pattern or symbol name.
func (cs *ContextServer) handleSearch(ctx context.Context, req *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, any, error) {
	if input.Query == "" {
		return errorResult("query parameter is required"), nil, nil
	}

	searchType := input.Type
	if searchType == "" {
		searchType = "path"
	}
	if searchType != "path" && searchType != "symbol" {
		return errorResult("type must be 'path' or 'symbol'"), nil, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	fileCount, err := idx.FileCount()
	if err != nil {
		return errorResult(fmt.Sprintf("checking index: %v", err)), nil, nil
	}
	if fileCount == 0 {
		if err := cs.runIndex(); err != nil {
			return errorResult(fmt.Sprintf("auto-indexing failed: %v", err)), nil, nil
		}
	}

	var dbResults []db.SearchResult
	if searchType == "path" {
		dbResults, err = idx.SearchByPath(input.Query, limit)
	} else {
		dbResults, err = idx.SearchBySymbol(input.Query, limit)
	}
	if err != nil {
		return errorResult(fmt.Sprintf("search failed: %v", err)), nil, nil
	}

	// Enrich with signals and role.
	paths := make([]string, len(dbResults))
	for i, r := range dbResults {
		paths[i] = r.Path
	}

	signals, err := idx.GetSignals(paths)
	if err != nil {
		signals = map[string]db.SignalRow{} // non-fatal
	}

	maxIndegree, _ := idx.GetMaxIndegree()
	stableThresh := maxIndegree / 2
	if stableThresh < 5 {
		stableThresh = 5
	}

	entries := make([]searchResultEntry, len(dbResults))
	for i, r := range dbResults {
		sig := signals[r.Path]
		role := classify.ClassifyRole(r.Path, sig.Indegree, sig.Outdegree, sig.IsEntrypoint, sig.IsUtility, stableThresh)
		entries[i] = searchResultEntry{
			File:      r.Path,
			Language:  r.Language,
			SizeBytes: r.SizeBytes,
			Indegree:  sig.Indegree,
			Outdegree: sig.Outdegree,
			Role:      role,
		}
	}

	output := searchOutput{
		Query:   input.Query,
		Type:    searchType,
		Results: entries,
		Count:   len(entries),
	}

	return jsonResult(output), nil, nil
}

// getEntrypointsInput is the input schema for the get_entrypoints tool.
type getEntrypointsInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Max foundation files to return (default 10)"`
}

// entrypointEntry is a file in the entrypoints/foundations response.
type entrypointEntry struct {
	File      string `json:"file"`
	Language  string `json:"language"`
	Indegree  int    `json:"indegree"`
	Outdegree int    `json:"outdegree"`
}

// getEntrypointsOutput is the JSON output of the get_entrypoints tool.
type getEntrypointsOutput struct {
	Entrypoints []entrypointEntry `json:"entrypoints"`
	Foundations []entrypointEntry `json:"foundations"`
	TotalFiles  int               `json:"total_files"`
}

// handleGetEntrypoints returns the project's entrypoint and foundation files.
func (cs *ContextServer) handleGetEntrypoints(ctx context.Context, req *mcp.CallToolRequest, input getEntrypointsInput) (*mcp.CallToolResult, any, error) {
	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	fileCount, err := idx.FileCount()
	if err != nil {
		return errorResult(fmt.Sprintf("checking index: %v", err)), nil, nil
	}
	if fileCount == 0 {
		if err := cs.runIndex(); err != nil {
			return errorResult(fmt.Sprintf("auto-indexing failed: %v", err)), nil, nil
		}
		fileCount, _ = idx.FileCount()
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	entrypoints, err := idx.GetEntrypoints()
	if err != nil {
		return errorResult(fmt.Sprintf("getting entrypoints: %v", err)), nil, nil
	}

	foundations, err := idx.GetFoundations(limit)
	if err != nil {
		return errorResult(fmt.Sprintf("getting foundations: %v", err)), nil, nil
	}

	epEntries := make([]entrypointEntry, len(entrypoints))
	for i, e := range entrypoints {
		epEntries[i] = entrypointEntry{File: e.Path, Language: e.Language, Indegree: e.Indegree, Outdegree: e.Outdegree}
	}

	fEntries := make([]entrypointEntry, len(foundations))
	for i, f := range foundations {
		fEntries[i] = entrypointEntry{File: f.Path, Language: f.Language, Indegree: f.Indegree, Outdegree: f.Outdegree}
	}

	output := getEntrypointsOutput{
		Entrypoints: epEntries,
		Foundations: fEntries,
		TotalFiles:  fileCount,
	}

	return jsonResult(output), nil, nil
}

// getStructureInput is the input schema for the get_structure tool.
type getStructureInput struct{}

// dirEntry is a directory in the structure response.
type dirEntry struct {
	Path      string         `json:"path"`
	FileCount int            `json:"file_count"`
	Languages map[string]int `json:"languages"`
}

// getStructureOutput is the JSON output of the get_structure tool.
type getStructureOutput struct {
	TotalFiles  int            `json:"total_files"`
	Languages   map[string]int `json:"languages"`
	Directories []dirEntry     `json:"directories"`
}

// handleGetStructure returns directory structure with file counts and language distribution.
func (cs *ContextServer) handleGetStructure(ctx context.Context, req *mcp.CallToolRequest, input getStructureInput) (*mcp.CallToolResult, any, error) {
	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	fileCount, err := idx.FileCount()
	if err != nil {
		return errorResult(fmt.Sprintf("checking index: %v", err)), nil, nil
	}
	if fileCount == 0 {
		if err := cs.runIndex(); err != nil {
			return errorResult(fmt.Sprintf("auto-indexing failed: %v", err)), nil, nil
		}
		fileCount, _ = idx.FileCount()
	}

	dirLangs, err := idx.GetDirectoryStructure()
	if err != nil {
		return errorResult(fmt.Sprintf("getting directory structure: %v", err)), nil, nil
	}

	// Aggregate into directory entries and global language counts.
	globalLangs := make(map[string]int)
	dirMap := make(map[string]*dirEntry)

	for _, dl := range dirLangs {
		globalLangs[dl.Language] += dl.FileCount

		de, ok := dirMap[dl.Dir]
		if !ok {
			de = &dirEntry{Path: dl.Dir, Languages: make(map[string]int)}
			dirMap[dl.Dir] = de
		}
		de.Languages[dl.Language] = dl.FileCount
		de.FileCount += dl.FileCount
	}

	// Sort directories by file count desc, then path asc.
	dirs := make([]dirEntry, 0, len(dirMap))
	for _, de := range dirMap {
		dirs = append(dirs, *de)
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].FileCount != dirs[j].FileCount {
			return dirs[i].FileCount > dirs[j].FileCount
		}
		return dirs[i].Path < dirs[j].Path
	})

	output := getStructureOutput{
		TotalFiles:  fileCount,
		Languages:   globalLangs,
		Directories: dirs,
	}

	return jsonResult(output), nil, nil
}

// --- Archetype Tool ---

// getArchetypesInput is the input schema for the get_archetypes tool.
type getArchetypesInput struct {
	Categories []string `json:"categories,omitempty" jsonschema:"Optional list of archetype categories to detect. If omitted, detects all. Available: Service/Controller, Model/Schema, Middleware/Plugin, High Fan-in Utility, Page/Route/Endpoint, Auth/Security, Leaf Component, Config/Constants, Barrel/Index, Test File, Database/Migration, Serialization/Validation, Error Handling, CLI/Command, Event/Message, Interface/Contract, Orchestrator, Hotspot"`
}

// archetypeEntry is a file in the archetypes response.
type archetypeEntry struct {
	Archetype string `json:"archetype"`
	File      string `json:"file"`
	Indegree  int    `json:"indegree"`
	Outdegree int    `json:"outdegree"`
	Role      string `json:"role,omitempty"`
}

// getArchetypesOutput is the JSON output of the get_archetypes tool.
type getArchetypesOutput struct {
	Archetypes []archetypeEntry `json:"archetypes"`
	Count      int              `json:"count"`
}

// handleGetArchetypes detects representative files across architectural layers.
func (cs *ContextServer) handleGetArchetypes(ctx context.Context, req *mcp.CallToolRequest, input getArchetypesInput) (*mcp.CallToolResult, any, error) {
	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	fileCount, err := idx.FileCount()
	if err != nil {
		return errorResult(fmt.Sprintf("checking index: %v", err)), nil, nil
	}
	if fileCount == 0 {
		if err := cs.runIndex(); err != nil {
			return errorResult(fmt.Sprintf("auto-indexing failed: %v", err)), nil, nil
		}
	}

	candidates, err := grader.DetectArchetypes(idx, input.Categories)
	if err != nil {
		return errorResult(fmt.Sprintf("detecting archetypes: %v", err)), nil, nil
	}

	entries := make([]archetypeEntry, len(candidates))
	for i, c := range candidates {
		entries[i] = archetypeEntry{
			Archetype: c.Archetype,
			File:      c.File,
			Indegree:  c.Indegree,
			Outdegree: c.Outdegree,
			Role:      c.Role,
		}
	}

	output := getArchetypesOutput{
		Archetypes: entries,
		Count:      len(entries),
	}

	return jsonResult(output), nil, nil
}

// --- Analyze Change Tool ---

// analyzeChangeInput is the input schema for the analyze_change tool.
type analyzeChangeInput struct {
	Base string `json:"base,omitempty" jsonschema:"Base git ref (commit SHA, branch name, or tag). If omitted, auto-detects merge-base against default branch."`
	Head string `json:"head,omitempty" jsonschema:"Head git ref. Defaults to HEAD."`
}

// handleAnalyzeChange analyzes the impact of a git diff.
func (cs *ContextServer) handleAnalyzeChange(ctx context.Context, req *mcp.CallToolRequest, input analyzeChangeInput) (*mcp.CallToolResult, any, error) {
	idx, err := cs.ensureIndex()
	if err != nil {
		return errorResult(fmt.Sprintf("opening index: %v", err)), nil, nil
	}

	// Ensure index is fresh.
	if err := cs.runIndex(); err != nil {
		return errorResult(fmt.Sprintf("auto-indexing failed: %v", err)), nil, nil
	}

	head := input.Head
	if head == "" {
		head = "HEAD"
	}

	base := input.Base
	if base == "" {
		// Auto-detect default branch and merge-base.
		defaultBranch, err := cs.detectDefaultBranch()
		if err != nil {
			return errorResult(fmt.Sprintf("could not detect base ref: %v — specify base explicitly", err)), nil, nil
		}
		mergeBase, err := cs.gitMergeBase(defaultBranch, head)
		if err != nil {
			// Fall back to direct diff against the default branch.
			base = defaultBranch
		} else {
			base = mergeBase
		}
	}

	cfg := change.Config{
		RepoRoot: cs.repoRoot,
		RepoCfg:  cs.ensureConfig(),
		AnalyzerCfg: analyzer.Config{
			RepoRoot: cs.repoRoot,
		},
	}

	start := time.Now()
	report, err := change.BuildReport(idx, cfg, base, head)
	if err != nil {
		return errorResult(fmt.Sprintf("analyzing change: %v", err)), nil, nil
	}
	durationMs := time.Since(start).Milliseconds()

	// Record usage analytics (best-effort).
	if hist, hErr := history.Open(cs.repoRoot); hErr == nil {
		entry := history.UsageEntryFromChangeReport("mcp", nil, report, durationMs, "", 0)
		_, _ = hist.RecordUsage(entry)
		hist.Close()
	}

	return jsonResult(report), nil, nil
}

// rateContextInput is the input schema for the rate_context tool.
type rateContextInput struct {
	File             string   `json:"file"              jsonschema:"File that was analyzed"`
	Usefulness       int      `json:"usefulness"        jsonschema:"1-5 rating: 1=not useful, 5=essential"`
	UsefulFiles      []string `json:"useful_files,omitempty" jsonschema:"Which must_read/related files were actually useful"`
	UnnecessaryFiles []string `json:"unnecessary_files,omitempty" jsonschema:"Files in must_read that were NOT needed"`
	MissingFiles     []string `json:"missing_files,omitempty" jsonschema:"Files you needed that were NOT suggested"`
	ModifiedFiles    []string `json:"modified_files,omitempty" jsonschema:"Files you actually modified"`
	Notes            string   `json:"notes,omitempty"   jsonschema:"Brief explanation of rating"`
}

// handleRateContext records LLM feedback about a previous get_context result.
func (cs *ContextServer) handleRateContext(ctx context.Context, req *mcp.CallToolRequest, input rateContextInput) (*mcp.CallToolResult, any, error) {
	if input.File == "" {
		return errorResult("file parameter is required"), nil, nil
	}
	if input.Usefulness < 1 || input.Usefulness > 5 {
		return errorResult("usefulness must be between 1 and 5"), nil, nil
	}

	hist, err := history.Open(cs.repoRoot)
	if err != nil {
		return errorResult(fmt.Sprintf("opening history: %v", err)), nil, nil
	}
	defer hist.Close()

	entry := &history.FeedbackEntry{
		FilePath:         input.File,
		Usefulness:       input.Usefulness,
		UsefulFiles:      input.UsefulFiles,
		UnnecessaryFiles: input.UnnecessaryFiles,
		MissingFiles:     input.MissingFiles,
		ModifiedFiles:    input.ModifiedFiles,
		Notes:            input.Notes,
	}

	_, err = hist.RecordFeedback(entry)
	if err != nil {
		return errorResult(fmt.Sprintf("recording feedback: %v", err)), nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Feedback recorded. Thank you."},
		},
	}, nil, nil
}

// detectDefaultBranch finds the default branch (main or master).
func (cs *ContextServer) detectDefaultBranch() (string, error) {
	// Try git symbolic-ref for remote HEAD.
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = cs.repoRoot
	out, err := cmd.Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// Fall back: check if main or master exist.
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "rev-parse", "--verify", branch)
		cmd.Dir = cs.repoRoot
		if err := cmd.Run(); err == nil {
			return branch, nil
		}
	}

	return "", fmt.Errorf("could not detect default branch (tried origin/HEAD, main, master)")
}

// gitMergeBase returns the merge base of two refs.
func (cs *ContextServer) gitMergeBase(a, b string) (string, error) {
	cmd := exec.Command("git", "merge-base", a, b)
	cmd.Dir = cs.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// jsonResult marshals v to JSON and wraps it in a CallToolResult.
// Returns an error result if marshalling fails.
func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return errorResult(fmt.Sprintf("marshalling result: %v", err))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}

// errorResult creates a CallToolResult with IsError set.
func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}
