// Package indexer orchestrates the scan → extract → resolve → store pipeline.
package indexer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kehoej/contextception/internal/classify"
	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/extractor"
	goextractor "github.com/kehoej/contextception/internal/extractor/golang"
	javaextractor "github.com/kehoej/contextception/internal/extractor/java"
	gitpkg "github.com/kehoej/contextception/internal/git"
	pyextractor "github.com/kehoej/contextception/internal/extractor/python"
	rustextractor "github.com/kehoej/contextception/internal/extractor/rust"
	tsextractor "github.com/kehoej/contextception/internal/extractor/typescript"
	"github.com/kehoej/contextception/internal/resolver"
	goresolver "github.com/kehoej/contextception/internal/resolver/golang"
	javaresolver "github.com/kehoej/contextception/internal/resolver/java"
	pyresolver "github.com/kehoej/contextception/internal/resolver/python"
	rustresolver "github.com/kehoej/contextception/internal/resolver/rust"
	tsresolver "github.com/kehoej/contextception/internal/resolver/typescript"
)

// Config configures the indexer.
type Config struct {
	RepoRoot   string
	Index      *db.Index
	Verbose    bool
	Output     io.Writer
	Config     *config.Config
	OnProgress func(phase string, current, total int) // optional streaming callback
}

// Indexer orchestrates the indexing pipeline.
type Indexer struct {
	repoRoot     string
	idx          *db.Index
	verbose      bool
	output       io.Writer
	cfg          *config.Config
	onProgress   func(phase string, current, total int)
	extractors   map[string]extractor.Extractor
	resolvers    map[string]resolver.Resolver
	packageRoots []pyresolver.PackageRoot
}

// NewIndexer creates an Indexer with all language extractors and resolvers.
func NewIndexer(cfg Config) (*Indexer, error) {
	if cfg.Output == nil {
		cfg.Output = os.Stderr
	}
	repoCfg := cfg.Config
	if repoCfg == nil {
		repoCfg = config.Empty()
	}

	pyExt := pyextractor.New()
	tsExt := tsextractor.New()
	goExt := goextractor.New()
	javaExt := javaextractor.New()
	rustExt := rustextractor.New()
	pyRes := pyresolver.New(cfg.RepoRoot)

	roots, err := pyRes.DetectPackageRoots()
	if err != nil {
		return nil, fmt.Errorf("detecting package roots: %w", err)
	}

	extractors := map[string]extractor.Extractor{}
	for _, ext := range pyExt.Extensions() {
		extractors[ext] = pyExt
	}
	for _, ext := range tsExt.Extensions() {
		extractors[ext] = tsExt
	}
	for _, ext := range goExt.Extensions() {
		extractors[ext] = goExt
	}
	for _, ext := range javaExt.Extensions() {
		extractors[ext] = javaExt
	}
	for _, ext := range rustExt.Extensions() {
		extractors[ext] = rustExt
	}

	tsRes := tsresolver.New(cfg.RepoRoot)
	tsRes.DetectWorkspaces()
	goRes := goresolver.New(cfg.RepoRoot)
	javaRes := javaresolver.New(cfg.RepoRoot)
	rustRes := rustresolver.New(cfg.RepoRoot)

	resolvers := map[string]resolver.Resolver{
		"python":     pyRes,
		"typescript": tsRes,
		"javascript": tsRes,
		"go":         goRes,
		"java":       javaRes,
		"rust":       rustRes,
	}

	return &Indexer{
		repoRoot:     cfg.RepoRoot,
		idx:          cfg.Index,
		verbose:      cfg.Verbose,
		output:       cfg.Output,
		cfg:          repoCfg,
		onProgress:   cfg.OnProgress,
		extractors:   extractors,
		resolvers:    resolvers,
		packageRoots: roots,
	}, nil
}

// Run executes the indexing pipeline, choosing full or incremental.
func (ix *Indexer) Run() error {
	lastCommit, _ := ix.idx.GetMeta("last_indexed_commit")
	headCommit := gitHeadCommit(ix.repoRoot)

	// Force full reindex when schema has changed since last full index.
	// This ensures data added by new migrations (e.g. imported_names)
	// is populated for all files, not just incrementally-changed ones.
	currentSchema, _ := ix.idx.SchemaVersion()
	indexedSchema, _ := ix.idx.GetMeta("indexed_at_schema")
	if indexedSchema != strconv.Itoa(currentSchema) {
		ix.log("Schema version changed (%s → %d), running full reindex", indexedSchema, currentSchema)
		return ix.fullIndex()
	}

	if lastCommit == "" || headCommit == "" {
		return ix.fullIndex()
	}

	// Check for changes since last index.
	ix.progress("Checking for changes", 0, 0)
	changed, removed, err := ix.detectChanges(lastCommit)
	if err != nil {
		ix.log("Could not detect incremental changes, running full index")
		return ix.fullIndex()
	}

	if len(changed) == 0 && len(removed) == 0 {
		ix.log("No changes detected since last index")
		ix.progress("No changes detected", 0, 0)
		return nil
	}

	return ix.incrementalIndex(changed, removed, headCommit)
}

// gitListFiles returns all tracked and untracked-but-not-ignored files using git.
func gitListFiles(repoRoot string) ([]string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "ls-files").Output()
	if err != nil {
		return nil, err
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" && !isIgnoredPath(line) {
			files = append(files, line)
		}
	}

	// Include untracked but not ignored files (new files not yet committed).
	out, err = exec.Command("git", "-C", repoRoot, "ls-files", "--others", "--exclude-standard").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" && !isIgnoredPath(line) {
				files = append(files, line)
			}
		}
	}

	return files, nil
}

// fullIndex scans the entire repository from scratch.
func (ix *Indexer) fullIndex() error {
	ix.log("Running full index...")
	totalStart := time.Now()

	phaseStart := time.Now()
	scanner := NewScanner(ix.repoRoot, ix.cfg.Ignore)

	var files []ScannedFile
	var err error

	// Prefer git-aware scanning to respect .gitignore.
	if gitFiles, gitErr := gitListFiles(ix.repoRoot); gitErr == nil {
		// Filter config-ignored files.
		var filtered []string
		for _, f := range gitFiles {
			if !ix.cfg.IsIgnored(f) {
				filtered = append(filtered, f)
			}
		}
		ix.log("Using git-aware file discovery (%d files, %d after config filter)", len(gitFiles), len(filtered))
		files, err = scanner.ScanPaths(filtered)
	} else {
		files, err = scanner.ScanAll()
	}
	if err != nil {
		return fmt.Errorf("scanning repository: %w", err)
	}

	// Sort by path for determinism.
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	ix.log("Found %d files", len(files))
	ix.progress("Scanning files", len(files), 0)
	ix.log("  [timing] file discovery + scanning: %v", time.Since(phaseStart))

	tx, err := ix.idx.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear all existing data.
	if err := db.ClearAllDataTx(tx); err != nil {
		return fmt.Errorf("clearing data: %w", err)
	}

	// Phase 1: Insert all file records (sequential, in transaction).
	indexedFiles := make(map[string]bool, len(files))
	for _, f := range files {
		if err := db.InsertFileTx(tx, f.Path, f.ContentHash, f.LastModified, f.Language, f.SizeBytes); err != nil {
			return fmt.Errorf("inserting file %q: %w", f.Path, err)
		}
		indexedFiles[f.Path] = true
	}

	// Phase 2: Process files (parallel extraction + resolution).
	phaseStart = time.Now()
	ix.progress("Extracting imports", 0, len(files))
	results := ix.processFilesParallel(files)
	ix.log("  [timing] extraction + resolution: %v", time.Since(phaseStart))

	// Phase 3: Insert edges and unresolved (sequential, in transaction).
	for _, r := range results {
		merged := mergeEdgeSymbols(r.edges)
		for _, e := range merged {
			if indexedFiles[e.dst] {
				if err := db.InsertEdgeTx(tx, e.src, e.dst, "import", e.specifier, e.method, e.line, e.importedNames); err != nil {
					return fmt.Errorf("inserting edge: %w", err)
				}
			} else {
				if err := db.InsertUnresolvedTx(tx, e.src, e.specifier, "resolved file not indexed", e.line); err != nil {
					return fmt.Errorf("inserting unresolved: %w", err)
				}
			}
		}
		for _, u := range r.unresolved {
			if err := db.InsertUnresolvedTx(tx, u.src, u.specifier, u.reason, u.line); err != nil {
				return fmt.Errorf("inserting unresolved: %w", err)
			}
		}
	}

	// Set metadata.
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.SetMetaTx(tx, "last_indexed_at", now); err != nil {
		return fmt.Errorf("setting last_indexed_at: %w", err)
	}
	head := gitHeadCommit(ix.repoRoot)
	if head != "" {
		if err := db.SetMetaTx(tx, "last_indexed_commit", head); err != nil {
			return fmt.Errorf("setting last_indexed_commit: %w", err)
		}
	}
	if err := db.SetMetaTx(tx, "repo_root", ix.repoRoot); err != nil {
		return fmt.Errorf("setting repo_root: %w", err)
	}
	// Auto-detect repo profile.
	profile := config.DetectRepoProfile(ix.repoRoot)
	if err := db.SetMetaTx(tx, "repo_profile", profile.Type); err != nil {
		return fmt.Errorf("setting repo_profile: %w", err)
	}
	if len(profile.Signals) > 0 {
		if err := db.SetMetaTx(tx, "repo_profile_signals", strings.Join(profile.Signals, ", ")); err != nil {
			return fmt.Errorf("setting repo_profile_signals: %w", err)
		}
	}
	schemaVer, _ := ix.idx.SchemaVersion()
	if err := db.SetMetaTx(tx, "indexed_at_schema", strconv.Itoa(schemaVer)); err != nil {
		return fmt.Errorf("setting indexed_at_schema: %w", err)
	}

	phaseStart = time.Now()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	ix.log("  [timing] DB writes (transaction commit): %v", time.Since(phaseStart))

	// Store package roots.
	var rootRecords []db.PackageRootRecord
	for _, r := range ix.packageRoots {
		rootRecords = append(rootRecords, db.PackageRootRecord{
			Path:            r.Path,
			DetectionMethod: r.DetectionMethod,
			Language:        "python",
		})
	}
	if err := ix.idx.UpsertPackageRoots(rootRecords); err != nil {
		return fmt.Errorf("storing package roots: %w", err)
	}

	// Compute signals in a separate transaction.
	ix.progress("Computing signals", 0, 0)
	phaseStart = time.Now()
	if err := ix.computeAllSignals(); err != nil {
		return fmt.Errorf("computing signals: %w", err)
	}

	// Apply config entrypoints.
	if err := ix.applyConfigEntrypoints(); err != nil {
		return fmt.Errorf("applying config entrypoints: %w", err)
	}
	ix.log("  [timing] signal computation: %v", time.Since(phaseStart))

	// Compute git history signals (non-fatal on failure).
	ix.progress("Git history analysis", 0, 0)
	phaseStart = time.Now()
	if err := ix.computeGitSignals(); err != nil {
		return fmt.Errorf("computing git signals: %w", err)
	}
	ix.log("  [timing] git signal computation: %v", time.Since(phaseStart))

	ix.progress("Writing index", 0, 0)
	ix.log("Indexed %d files (total: %v)", len(files), time.Since(totalStart))
	return nil
}

// incrementalIndex processes only changed and removed files.
func (ix *Indexer) incrementalIndex(changed, removed []string, headCommit string) error {
	// Filter config-ignored files from changed list.
	var filteredChanged []string
	for _, f := range changed {
		if !ix.cfg.IsIgnored(f) {
			filteredChanged = append(filteredChanged, f)
		}
	}
	changed = filteredChanged

	ix.log("Running incremental index (%d changed, %d removed)", len(changed), len(removed))
	ix.progress("Incremental index", len(changed), 0)

	tx, err := ix.idx.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Remove deleted files (CASCADE handles edges/signals/unresolved).
	for _, path := range removed {
		if err := db.DeleteFileTx(tx, path); err != nil {
			return fmt.Errorf("deleting file %q: %w", path, err)
		}
	}

	// Process changed/added files.
	scanner := NewScanner(ix.repoRoot, ix.cfg.Ignore)
	scanned, err := scanner.ScanPaths(changed)
	if err != nil {
		return fmt.Errorf("scanning changed files: %w", err)
	}

	// Phase 1: Delete old data and upsert file records (sequential, in transaction).
	for _, f := range scanned {
		if err := db.DeleteFileEdgesTx(tx, f.Path); err != nil {
			return fmt.Errorf("deleting edges for %q: %w", f.Path, err)
		}
		if err := db.DeleteFileUnresolvedTx(tx, f.Path); err != nil {
			return fmt.Errorf("deleting unresolved for %q: %w", f.Path, err)
		}
		if err := db.DeleteFileSignalsTx(tx, f.Path); err != nil {
			return fmt.Errorf("deleting signals for %q: %w", f.Path, err)
		}
		if err := db.UpsertFileTx(tx, f.Path, f.ContentHash, f.LastModified, f.Language, f.SizeBytes); err != nil {
			return fmt.Errorf("upserting file %q: %w", f.Path, err)
		}
	}

	// Phase 2: Process files (parallel extraction + resolution).
	ix.progress("Extracting imports", 0, len(scanned))
	results := ix.processFilesParallel(scanned)

	// Phase 3: Insert edges and unresolved (sequential, in transaction).
	for _, r := range results {
		merged := mergeEdgeSymbols(r.edges)
		for _, e := range merged {
			exists, err := db.FileExistsTx(tx, e.dst)
			if err != nil {
				return fmt.Errorf("checking file existence: %w", err)
			}
			if exists {
				if err := db.InsertEdgeTx(tx, e.src, e.dst, "import", e.specifier, e.method, e.line, e.importedNames); err != nil {
					return fmt.Errorf("inserting edge: %w", err)
				}
			} else {
				if err := db.InsertUnresolvedTx(tx, e.src, e.specifier, "resolved file not indexed", e.line); err != nil {
					return fmt.Errorf("inserting unresolved: %w", err)
				}
			}
		}
		for _, u := range r.unresolved {
			if err := db.InsertUnresolvedTx(tx, u.src, u.specifier, u.reason, u.line); err != nil {
				return fmt.Errorf("inserting unresolved: %w", err)
			}
		}
	}

	// Update metadata.
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.SetMetaTx(tx, "last_indexed_at", now); err != nil {
		return fmt.Errorf("setting last_indexed_at: %w", err)
	}
	if headCommit != "" {
		if err := db.SetMetaTx(tx, "last_indexed_commit", headCommit); err != nil {
			return fmt.Errorf("setting last_indexed_commit: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	// Recompute all signals (simpler than tracking affected neighbors).
	ix.progress("Computing signals", 0, 0)
	if err := ix.computeAllSignals(); err != nil {
		return fmt.Errorf("computing signals: %w", err)
	}

	// Apply config entrypoints.
	if err := ix.applyConfigEntrypoints(); err != nil {
		return fmt.Errorf("applying config entrypoints: %w", err)
	}

	// Compute git history signals (non-fatal on failure).
	ix.progress("Git history analysis", 0, 0)
	if err := ix.computeGitSignals(); err != nil {
		return fmt.Errorf("computing git signals: %w", err)
	}

	ix.log("Incremental index complete")
	return nil
}

type pendingEdge struct {
	src, dst, specifier, method string
	line                        int
	importedNames               []string
}

// mergeEdgeSymbols groups edges by (src, dst, specifier) and merges their
// importedNames slices, deduplicating names. This handles the case where a
// file has multiple import statements from the same module (e.g., one
// `import type { MatchData }` and one `import { createRouter }`).
// The first edge's other fields (line_number, method) are preserved.
func mergeEdgeSymbols(edges []pendingEdge) []pendingEdge {
	if len(edges) <= 1 {
		return edges
	}

	type edgeKey struct{ src, dst, specifier string }
	seen := make(map[edgeKey]int, len(edges)) // key → index in result
	result := make([]pendingEdge, 0, len(edges))

	for _, e := range edges {
		key := edgeKey{e.src, e.dst, e.specifier}
		if idx, ok := seen[key]; ok {
			// Merge importedNames into the existing edge.
			existing := &result[idx]
			if len(e.importedNames) > 0 {
				nameSet := make(map[string]bool, len(existing.importedNames)+len(e.importedNames))
				for _, n := range existing.importedNames {
					nameSet[n] = true
				}
				for _, n := range e.importedNames {
					if !nameSet[n] {
						existing.importedNames = append(existing.importedNames, n)
						nameSet[n] = true
					}
				}
			}
		} else {
			seen[key] = len(result)
			// Copy the edge so we don't mutate the input slice.
			eCopy := e
			if len(e.importedNames) > 0 {
				eCopy.importedNames = make([]string, len(e.importedNames))
				copy(eCopy.importedNames, e.importedNames)
			}
			result = append(result, eCopy)
		}
	}

	return result
}

type pendingUnresolved struct {
	src, specifier, reason string
	line                   int
}

// processFile extracts imports and resolves them, returning edges and unresolved records.
func (ix *Indexer) processFile(f ScannedFile) ([]pendingEdge, []pendingUnresolved) {
	return processFileWith(f, ix.extractors, ix.resolvers, ix.repoRoot)
}

// processFileWith is the standalone implementation of processFile that accepts
// explicit extractors and resolvers, enabling safe concurrent use with cloned extractors.
func processFileWith(f ScannedFile, extractors map[string]extractor.Extractor, resolvers map[string]resolver.Resolver, repoRoot string) ([]pendingEdge, []pendingUnresolved) {
	ext := filepath.Ext(f.Path)
	extr, ok := extractors[ext]
	if !ok {
		return nil, nil
	}

	content, err := os.ReadFile(f.AbsPath)
	if err != nil {
		return nil, nil
	}

	facts, err := extr.Extract(f.Path, content)
	if err != nil {
		return nil, nil
	}

	// Select resolver by language, fall back to NullResolver.
	res, ok := resolvers[f.Language]
	if !ok {
		res = &resolver.NullResolver{}
	}

	var edges []pendingEdge
	var unresolved []pendingUnresolved

	// Check if the resolver supports multi-resolution (e.g., Go package imports).
	multiRes, isMulti := res.(resolver.MultiResolver)

	for _, fact := range facts {
		if isMulti {
			results, err := multiRes.ResolveAll(f.Path, fact, repoRoot)
			if err != nil {
				unresolved = append(unresolved, pendingUnresolved{
					src: f.Path, specifier: fact.Specifier, reason: "resolution_error", line: fact.LineNumber,
				})
				continue
			}
			for _, result := range results {
				if result.External {
					unresolved = append(unresolved, pendingUnresolved{
						src: f.Path, specifier: fact.Specifier, reason: result.Reason, line: fact.LineNumber,
					})
				} else {
					edges = append(edges, pendingEdge{
						src: f.Path, dst: result.ResolvedPath, specifier: fact.Specifier,
						method: result.ResolutionMethod, line: fact.LineNumber,
						importedNames: fact.ImportedNames,
					})
				}
			}
		} else {
			result, err := res.Resolve(f.Path, fact, repoRoot)
			if err != nil {
				unresolved = append(unresolved, pendingUnresolved{
					src: f.Path, specifier: fact.Specifier, reason: "resolution_error", line: fact.LineNumber,
				})
				continue
			}
			if result.External {
				unresolved = append(unresolved, pendingUnresolved{
					src: f.Path, specifier: fact.Specifier, reason: result.Reason, line: fact.LineNumber,
				})
			} else {
				edges = append(edges, pendingEdge{
					src: f.Path, dst: result.ResolvedPath, specifier: fact.Specifier,
					method: result.ResolutionMethod, line: fact.LineNumber,
					importedNames: fact.ImportedNames,
				})
			}
		}
	}

	// Same-package edges: for languages like Go where files in the same directory
	// implicitly depend on each other.
	if spRes, ok := res.(resolver.SamePackageResolver); ok {
		for _, result := range spRes.ResolveSamePackageEdges(f.Path, repoRoot) {
			if !result.External {
				edges = append(edges, pendingEdge{
					src: f.Path, dst: result.ResolvedPath, specifier: "<same_package>",
					method: result.ResolutionMethod, line: 0,
				})
			}
		}
	}

	return edges, unresolved
}

// fileResults holds the processing output for a single file.
type fileResults struct {
	edges      []pendingEdge
	unresolved []pendingUnresolved
}

// processFilesParallel runs processFileWith concurrently across files using a worker pool.
// Results are stored in a pre-allocated slice indexed by file position for deterministic order.
func (ix *Indexer) processFilesParallel(files []ScannedFile) []fileResults {
	results := make([]fileResults, len(files))

	// Skip goroutines for small file counts (overhead not worth it).
	if len(files) <= 16 {
		for i, f := range files {
			results[i].edges, results[i].unresolved = ix.processFile(f)
		}
		return results
	}

	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers > len(files) {
		numWorkers = len(files)
	}

	fileCh := make(chan int, len(files))
	for i := range files {
		fileCh <- i
	}
	close(fileCh)

	var processed atomic.Int64
	total := len(files)

	// Report progress periodically from a separate goroutine.
	done := make(chan struct{})
	if ix.onProgress != nil {
		go func() {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					cur := int(processed.Load())
					ix.progress("Extracting imports", cur, total)
				}
			}
		}()
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		clonedExtractors := ix.cloneExtractors()
		go func(extractors map[string]extractor.Extractor) {
			defer wg.Done()
			for i := range fileCh {
				edges, unresolved := processFileWith(files[i], extractors, ix.resolvers, ix.repoRoot)
				results[i] = fileResults{edges: edges, unresolved: unresolved}
				processed.Add(1)
			}
		}(clonedExtractors)
	}
	wg.Wait()
	close(done)
	// Final progress update with exact count.
	ix.progress("Extracting imports", total, total)

	return results
}

// cloneExtractors creates independent copies of all extractors for concurrent use.
func (ix *Indexer) cloneExtractors() map[string]extractor.Extractor {
	cloned := make(map[string]extractor.Extractor, len(ix.extractors))
	for ext, e := range ix.extractors {
		cloned[ext] = e.Clone()
	}
	return cloned
}

// computeAllSignals recomputes structural signals for all files.
func (ix *Indexer) computeAllSignals() error {
	tx, err := ix.idx.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing signals.
	if _, err := tx.Exec("DELETE FROM signals"); err != nil {
		return err
	}

	// Build a temp table of non-source files (tests, docs, examples) so that
	// indegree only counts imports from real source files. This prevents test
	// files from inflating indegree of heavily-tested modules.
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS non_source_files (path TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM non_source_files`); err != nil {
		return err
	}

	rows, err := tx.Query("SELECT path FROM files")
	if err != nil {
		return err
	}
	var nonSource []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		if classify.IsNonSource(p) {
			nonSource = append(nonSource, p)
		}
	}
	rows.Close()

	for _, p := range nonSource {
		if _, err := tx.Exec("INSERT OR IGNORE INTO non_source_files (path) VALUES (?)", p); err != nil {
			return err
		}
	}

	// Compute indegree (source-only), outdegree, entrypoint, and utility flags.
	_, err = tx.Exec(`
		INSERT INTO signals (file_path, indegree, outdegree, is_entrypoint, is_utility)
		SELECT
			f.path,
			COALESCE(ind.cnt, 0),
			COALESCE(outd.cnt, 0),
			CASE
				WHEN COALESCE(ind.cnt, 0) = 0
				     AND f.path NOT LIKE '%/__init__.py'
				     AND f.path != '__init__.py'
				THEN 1 ELSE 0
			END,
			CASE
				WHEN COALESCE(outd.cnt, 0) >= 5 AND COALESCE(ind.cnt, 0) <= 2
				THEN 1 ELSE 0
			END
		FROM files f
		LEFT JOIN (
			SELECT e.dst_file, COUNT(DISTINCT e.src_file) AS cnt
			FROM edges e
			LEFT JOIN non_source_files ns ON ns.path = e.src_file
			WHERE ns.path IS NULL
			GROUP BY e.dst_file
		) ind ON ind.dst_file = f.path
		LEFT JOIN (
			SELECT src_file, COUNT(DISTINCT dst_file) AS cnt
			FROM edges GROUP BY src_file
		) outd ON outd.src_file = f.path
	`)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// applyConfigEntrypoints marks config-specified entrypoints in the signals table.
func (ix *Indexer) applyConfigEntrypoints() error {
	if len(ix.cfg.Entrypoints) == 0 {
		return nil
	}

	tx, err := ix.idx.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, ep := range ix.cfg.Entrypoints {
		_, err := tx.Exec("UPDATE signals SET is_entrypoint = 1 WHERE file_path = ?", ep)
		if err != nil {
			return fmt.Errorf("setting config entrypoint %q: %w", ep, err)
		}
	}

	return tx.Commit()
}

// detectChanges uses git to find files changed since lastCommit.
func (ix *Indexer) detectChanges(lastCommit string) (changed, removed []string, err error) {
	head := gitHeadCommit(ix.repoRoot)
	if head == "" {
		return nil, nil, fmt.Errorf("could not determine HEAD commit")
	}
	if head == lastCommit {
		// Check working tree changes.
		return ix.detectWorkingTreeChanges()
	}

	// Get committed changes.
	out, err := exec.Command("git", "-C", ix.repoRoot, "diff", "--name-status", lastCommit+".."+head).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("git diff: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		status, file := parts[0], parts[len(parts)-1]
		if isIgnoredPath(file) {
			continue
		}
		if status == "D" {
			removed = append(removed, file)
		} else {
			changed = append(changed, file)
		}
	}

	// Also check working tree.
	wtChanged, wtRemoved, err := ix.detectWorkingTreeChanges()
	if err == nil {
		changed = append(changed, wtChanged...)
		removed = append(removed, wtRemoved...)
	}

	return changed, removed, nil
}

func (ix *Indexer) detectWorkingTreeChanges() (changed, removed []string, err error) {
	// Unstaged changes.
	out, err := exec.Command("git", "-C", ix.repoRoot, "diff", "--name-status").Output()
	if err != nil {
		return nil, nil, err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		file := parts[len(parts)-1]
		if isIgnoredPath(file) {
			continue
		}
		if parts[0] == "D" {
			removed = append(removed, file)
		} else {
			changed = append(changed, file)
		}
	}

	// Untracked files.
	out, err = exec.Command("git", "-C", ix.repoRoot, "ls-files", "--others", "--exclude-standard").Output()
	if err != nil {
		return changed, removed, nil // non-fatal
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" && !isIgnoredPath(line) {
			changed = append(changed, line)
		}
	}

	return changed, removed, nil
}

func gitHeadCommit(repoRoot string) string {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// computeGitSignals extracts churn and co-change data from git history
// and stores it in the index. Non-fatal: if extraction fails, the indexer
// logs a warning and continues with structural-only signals.
func (ix *Indexer) computeGitSignals() error {
	signals, err := gitpkg.Extract(gitpkg.Config{
		RepoRoot:      ix.repoRoot,
		WindowDays:    90,
		MaxCommitSize: 100,
	})
	if err != nil {
		ix.log("Warning: git signal extraction failed: %v", err)
		return nil // non-fatal
	}

	tx, err := ix.idx.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := db.ClearGitDataTx(tx); err != nil {
		return err
	}

	// Only insert churn for files that exist in the index (FK constraint).
	indexedFiles, err := ix.idx.GetAllFileHashes()
	if err != nil {
		return fmt.Errorf("getting indexed files: %w", err)
	}

	for path, count := range signals.Churn {
		if _, ok := indexedFiles[path]; !ok {
			continue // skip files not in the index
		}
		if err := db.InsertGitSignalTx(tx, db.GitSignalRecord{
			FilePath:   path,
			SignalType: "churn",
			Value:      float64(count),
			WindowDays: signals.WindowDays,
			ComputedAt: signals.ComputedAt,
		}); err != nil {
			return fmt.Errorf("inserting churn signal for %q: %w", path, err)
		}
	}

	for pair, freq := range signals.CoChange {
		if err := db.InsertCoChangePairTx(tx, db.CoChangePairRecord{
			FileA:      pair[0],
			FileB:      pair[1],
			Frequency:  freq,
			WindowDays: signals.WindowDays,
			ComputedAt: signals.ComputedAt,
		}); err != nil {
			return fmt.Errorf("inserting co-change pair: %w", err)
		}
	}

	ix.log("Stored git signals: %d churn entries, %d co-change pairs", len(signals.Churn), len(signals.CoChange))
	return tx.Commit()
}

func (ix *Indexer) progress(phase string, current, total int) {
	if ix.onProgress != nil {
		ix.onProgress(phase, current, total)
	}
}

func (ix *Indexer) log(format string, args ...any) {
	if ix.verbose {
		fmt.Fprintf(ix.output, format+"\n", args...)
	}
}
