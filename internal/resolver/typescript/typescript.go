// Package typescript implements import resolution for TypeScript and JavaScript source files.
//
// Resolution follows ADR-0003 priority order:
//  1. Relative specifiers (./ or ../) — extension + index fallback
//  2. tsconfig paths mapping — longest prefix match, baseUrl expansion
//  3. Workspace package resolution — monorepo cross-package imports
//  4. Fallback — classify as external
package typescript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kehoej/contextception/internal/model"
	"github.com/kehoej/contextception/internal/resolver"
)

// tsExtensions lists the file extensions tried during resolution, in priority order per ADR-0003.
var tsExtensions = []string{
	".ts", ".tsx", ".mts", ".cts",
	".js", ".jsx", ".mjs", ".cjs",
}

// Resolver resolves TypeScript/JavaScript import specifiers to repository files.
type Resolver struct {
	repoRoot      string
	tsconfigCache map[string]*tsconfig // dir → parsed tsconfig (nil = no tsconfig found)
	cacheMu       sync.RWMutex         // protects tsconfigCache for concurrent access
	workspace     *workspaceConfig     // nil if not a monorepo
}

// New creates a new TypeScript resolver for the given repository root.
func New(repoRoot string) *Resolver {
	return &Resolver{
		repoRoot:      repoRoot,
		tsconfigCache: make(map[string]*tsconfig),
	}
}

// DetectWorkspaces scans the repo root for workspace manifests (pnpm-workspace.yaml,
// package.json workspaces) and discovers workspace packages. Call once at init time.
func (r *Resolver) DetectWorkspaces() {
	r.workspace = detectWorkspaces(r.repoRoot)
}

// GetWorkspacePackages returns the names of discovered workspace packages (for diagnostics).
func (r *Resolver) GetWorkspacePackages() []string {
	if r.workspace == nil {
		return nil
	}
	names := make([]string, 0, len(r.workspace.packages))
	for name := range r.workspace.packages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Resolve maps a TS/JS import to a repository file.
func (r *Resolver) Resolve(srcFile string, fact model.ImportFact, repoRoot string) (model.ResolveResult, error) {
	spec := fact.Specifier

	// 1. Relative specifiers
	if fact.ImportType == "relative" {
		return r.resolveRelative(srcFile, spec), nil
	}

	// 2. tsconfig paths mapping
	if result, ok := r.resolveTsconfigPaths(srcFile, spec); ok {
		return result, nil
	}

	// 2b. Bare specifier against baseUrl (when paths didn't match)
	if result, ok := r.resolveBaseUrl(srcFile, spec); ok {
		return result, nil
	}

	// 3. Workspace package resolution
	if r.workspace != nil {
		if result, ok := r.resolveWorkspace(spec); ok {
			return result, nil
		}
	}

	// 4. Fallback: external
	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "external",
		Reason:           "external",
	}, nil
}

// resolveWorkspace attempts to resolve a bare specifier as a workspace package import.
func (r *Resolver) resolveWorkspace(spec string) (model.ResolveResult, bool) {
	pkg, subpath := matchPackage(r.workspace.packages, spec)
	if pkg == nil {
		return model.ResolveResult{}, false
	}

	resolved := resolvePackageEntry(r, pkg, subpath)
	if resolved == "" {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "workspace_package",
			Reason:           "workspace_entry_not_found",
		}, true
	}

	return model.ResolveResult{
		ResolvedPath:     resolved,
		ResolutionMethod: "workspace_package",
	}, true
}

// resolveRelative resolves a relative specifier (./ or ../) from the importing file.
func (r *Resolver) resolveRelative(srcFile, spec string) model.ResolveResult {
	srcDir := filepath.Dir(srcFile)
	target := filepath.Join(srcDir, spec)
	// Normalize to slash-separated, repo-relative path.
	target = filepath.ToSlash(filepath.Clean(target))

	// Reject paths that escape the repo.
	if strings.HasPrefix(target, "..") {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "relative",
			Reason:           "escapes_repository",
		}
	}

	if resolved := r.resolveFileCandidate(target); resolved != "" {
		return model.ResolveResult{
			ResolvedPath:     resolved,
			ResolutionMethod: "relative",
		}
	}

	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "relative",
		Reason:           "not_found",
	}
}

// resolveTsconfigPaths attempts to resolve a bare specifier using the nearest tsconfig's paths mapping.
func (r *Resolver) resolveTsconfigPaths(srcFile, spec string) (model.ResolveResult, bool) {
	tc := r.findTsconfig(filepath.Dir(srcFile))
	if tc == nil || len(tc.paths) == 0 {
		return model.ResolveResult{}, false
	}

	// Sort patterns by length descending (longest prefix wins).
	patterns := make([]string, 0, len(tc.paths))
	for p := range tc.paths {
		patterns = append(patterns, p)
	}
	sort.Slice(patterns, func(i, j int) bool {
		return len(patterns[i]) > len(patterns[j])
	})

	for _, pattern := range patterns {
		targets := tc.paths[pattern]

		if strings.HasSuffix(pattern, "/*") {
			// Wildcard pattern: "@lib/*" matches "@lib/foo" → capture "foo"
			prefix := strings.TrimSuffix(pattern, "*")
			if !strings.HasPrefix(spec, prefix) {
				continue
			}
			rest := spec[len(prefix):]

			for _, target := range targets {
				if !strings.HasSuffix(target, "/*") {
					continue
				}
				targetBase := strings.TrimSuffix(target, "*")
				candidate := filepath.ToSlash(filepath.Join(tc.baseDir, targetBase, rest))
				if resolved := r.resolveFileCandidate(candidate); resolved != "" {
					return model.ResolveResult{
						ResolvedPath:     resolved,
						ResolutionMethod: "tsconfig_paths",
					}, true
				}
			}
		} else {
			// Exact pattern: "@lib/foo" matches "@lib/foo"
			if spec != pattern {
				continue
			}
			for _, target := range targets {
				candidate := filepath.ToSlash(filepath.Join(tc.baseDir, target))
				if resolved := r.resolveFileCandidate(candidate); resolved != "" {
					return model.ResolveResult{
						ResolvedPath:     resolved,
						ResolutionMethod: "tsconfig_paths",
					}, true
				}
			}
		}
	}

	return model.ResolveResult{}, false
}

// resolveBaseUrl attempts to resolve a bare specifier directly against the tsconfig baseUrl.
// This handles cases like `import X from "components/Foo"` when baseUrl is ".".
func (r *Resolver) resolveBaseUrl(srcFile, spec string) (model.ResolveResult, bool) {
	tc := r.findTsconfig(filepath.Dir(srcFile))
	if tc == nil || tc.baseDir == "" {
		return model.ResolveResult{}, false
	}
	candidate := filepath.ToSlash(filepath.Join(tc.baseDir, spec))
	if resolved := r.resolveFileCandidate(candidate); resolved != "" {
		return model.ResolveResult{
			ResolvedPath:     resolved,
			ResolutionMethod: "tsconfig_baseUrl",
		}, true
	}
	return model.ResolveResult{}, false
}

// resolveFileCandidate tries to resolve a repo-relative path (without extension) to an actual file.
// Follows the candidate order from ADR-0003 section 2.
func (r *Resolver) resolveFileCandidate(base string) string {
	// If the specifier already has a recognized extension, try remapping it first.
	// e.g., ./foo.js → try ./foo.ts, ./foo.tsx, ... then ./foo.js itself.
	ext := filepath.Ext(base)
	if isJSExtension(ext) {
		stem := strings.TrimSuffix(base, ext)
		// Try TS equivalents of the JS extension first.
		for _, tsExt := range tsExtensions {
			candidate := stem + tsExt
			if r.fileExists(candidate) {
				return candidate
			}
		}
		// The original path with its extension.
		if r.fileExists(base) {
			return base
		}
		// Also try index files using the stem as a directory.
		for _, tsExt := range tsExtensions {
			candidate := filepath.ToSlash(filepath.Join(stem, "index"+tsExt))
			if r.fileExists(candidate) {
				return candidate
			}
		}
		return ""
	}

	// If the specifier already has a TS extension and exists, return it directly.
	if isTSExtension(ext) {
		if r.fileExists(base) {
			return base
		}
		return ""
	}

	// No recognized extension — try each extension, then index files.
	for _, tsExt := range tsExtensions {
		candidate := base + tsExt
		if r.fileExists(candidate) {
			return candidate
		}
	}
	// Try as directory with index file.
	for _, tsExt := range tsExtensions {
		candidate := filepath.ToSlash(filepath.Join(base, "index"+tsExt))
		if r.fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// findTsconfig walks up from dir to repo root looking for tsconfig.json.
// Results are cached per directory. Safe for concurrent access.
func (r *Resolver) findTsconfig(dir string) *tsconfig {
	// Normalize dir to be repo-relative.
	dir = filepath.ToSlash(filepath.Clean(dir))
	if dir == "." {
		dir = ""
	}

	// Read-lock fast path.
	r.cacheMu.RLock()
	tc, ok := r.tsconfigCache[dir]
	r.cacheMu.RUnlock()
	if ok {
		return tc
	}

	// Write-lock slow path with double-check.
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	if tc, ok := r.tsconfigCache[dir]; ok {
		return tc
	}

	// Walk upward from dir to repo root.
	current := dir
	for {
		var absDir string
		if current == "" {
			absDir = r.repoRoot
		} else {
			absDir = filepath.Join(r.repoRoot, current)
		}

		tsconfigPath := filepath.Join(absDir, "tsconfig.json")
		if fileExists(tsconfigPath) {
			tc := parseTsconfig(tsconfigPath, current, r.repoRoot)
			r.tsconfigCache[dir] = tc
			return tc
		}

		if current == "" {
			break
		}
		current = filepath.ToSlash(filepath.Dir(current))
		if current == "." {
			current = ""
		}
	}

	r.tsconfigCache[dir] = nil
	return nil
}

func (r *Resolver) fileExists(repoRelPath string) bool {
	abs := filepath.Join(r.repoRoot, repoRelPath)
	return fileExists(abs)
}

// tsconfig holds the parsed resolver-relevant fields from a tsconfig.json.
type tsconfig struct {
	baseDir    string              // repo-relative directory containing the tsconfig
	paths      map[string][]string // compilerOptions.paths
	hasBaseUrl bool                // true if baseUrl was explicitly set
}

// parseTsconfig reads a tsconfig.json and extracts paths, baseUrl, and handles extends chains.
func parseTsconfig(absPath, repoRelDir, repoRoot string) *tsconfig {
	return parseTsconfigWithSeen(absPath, repoRelDir, repoRoot, make(map[string]bool))
}

// parseTsconfigWithSeen implements parseTsconfig with cycle detection for extends chains.
func parseTsconfigWithSeen(absPath, repoRelDir, repoRoot string, seen map[string]bool) *tsconfig {
	canonical := filepath.Clean(absPath)
	if seen[canonical] {
		return nil
	}
	seen[canonical] = true

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	data = stripJSONComments(data)

	var raw struct {
		Extends         string `json:"extends"`
		CompilerOptions struct {
			BaseUrl string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	// Determine the base directory for path resolution.
	// If baseUrl is set, it's relative to the tsconfig directory.
	// If not set, paths are relative to the tsconfig directory.
	hasBaseUrl := raw.CompilerOptions.BaseUrl != ""
	baseDir := repoRelDir
	if hasBaseUrl {
		bu := filepath.ToSlash(raw.CompilerOptions.BaseUrl)
		if bu == "." {
			baseDir = repoRelDir
		} else {
			if repoRelDir == "" {
				baseDir = bu
			} else {
				baseDir = filepath.ToSlash(filepath.Join(repoRelDir, bu))
			}
		}
	}

	tc := &tsconfig{
		baseDir:    baseDir,
		paths:      raw.CompilerOptions.Paths,
		hasBaseUrl: hasBaseUrl,
	}

	// Handle extends chain.
	if raw.Extends != "" {
		parent := resolveExtendsParent(absPath, repoRoot, raw.Extends, seen)
		if parent != nil {
			// paths: child completely replaces parent (not merged key-by-key).
			if tc.paths == nil {
				tc.paths = parent.paths
				if !hasBaseUrl {
					tc.baseDir = parent.baseDir
				}
			} else if !hasBaseUrl && parent.hasBaseUrl {
				// Child defines own paths but no baseUrl; parent has explicit baseUrl.
				// Inherit parent's baseUrl so child's paths resolve correctly.
				tc.baseDir = parent.baseDir
			}
			// If child has own paths and parent has no baseUrl, keep child's
			// baseDir — the child's paths are relative to its own directory.
		}
	}

	return tc
}

// resolveExtendsParent resolves the parent tsconfig referenced by an extends field.
// Only relative extends paths (./ or ../) are supported; package references are skipped.
func resolveExtendsParent(childAbsPath, repoRoot, extends string, seen map[string]bool) *tsconfig {
	// Skip package references (non-relative extends like @tsconfig/node18).
	if !strings.HasPrefix(extends, "./") && !strings.HasPrefix(extends, "../") {
		return nil
	}

	childDir := filepath.Dir(childAbsPath)
	parentAbsPath := filepath.Clean(filepath.Join(childDir, extends))

	// Auto-append .json if file not found and no .json extension.
	if !fileExists(parentAbsPath) && filepath.Ext(parentAbsPath) != ".json" {
		if candidate := parentAbsPath + ".json"; fileExists(candidate) {
			parentAbsPath = candidate
		}
	}

	// Guard against paths escaping repo root.
	rel, err := filepath.Rel(repoRoot, parentAbsPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil
	}

	// Compute parent's repo-relative directory.
	parentDir := filepath.Dir(parentAbsPath)
	parentRepoRelDir, err := filepath.Rel(repoRoot, parentDir)
	if err != nil {
		return nil
	}
	parentRepoRelDir = filepath.ToSlash(parentRepoRelDir)
	if parentRepoRelDir == "." {
		parentRepoRelDir = ""
	}

	return parseTsconfigWithSeen(parentAbsPath, parentRepoRelDir, repoRoot, seen)
}

// stripJSONComments removes comments and trailing commas from JSONC (tsconfig.json).
// Two-pass: first strip comments, then strip trailing commas.
func stripJSONComments(data []byte) []byte {
	// Pass 1: strip comments.
	data = stripComments(data)
	// Pass 2: strip trailing commas.
	data = stripTrailingCommas(data)
	return data
}

// stripComments removes single-line (//) and multi-line (/* */) comments.
func stripComments(data []byte) []byte {
	var result []byte
	i := 0
	inString := false

	for i < len(data) {
		if data[i] == '"' && (i == 0 || data[i-1] != '\\') {
			inString = !inString
			result = append(result, data[i])
			i++
			continue
		}

		if inString {
			result = append(result, data[i])
			i++
			continue
		}

		// Single-line comment.
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}

		// Multi-line comment.
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) {
				if data[i] == '*' && data[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		result = append(result, data[i])
		i++
	}
	return result
}

// stripTrailingCommas removes commas that appear before } or ].
func stripTrailingCommas(data []byte) []byte {
	var result []byte
	i := 0
	inString := false

	for i < len(data) {
		if data[i] == '"' && (i == 0 || data[i-1] != '\\') {
			inString = !inString
			result = append(result, data[i])
			i++
			continue
		}

		if inString {
			result = append(result, data[i])
			i++
			continue
		}

		if data[i] == ',' {
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				i++
				continue
			}
		}

		result = append(result, data[i])
		i++
	}
	return result
}

var fileExists = resolver.FileExists

func isJSExtension(ext string) bool {
	switch ext {
	case ".js", ".jsx", ".mjs", ".cjs":
		return true
	}
	return false
}

func isTSExtension(ext string) bool {
	switch ext {
	case ".ts", ".tsx", ".mts", ".cts":
		return true
	}
	return false
}
