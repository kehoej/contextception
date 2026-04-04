// Package java implements import resolution for Java source files.
package java

import (
	"os"
	"path/filepath"
	"strings"

	javapkg "github.com/kehoej/contextception/internal/extractor/java"
	"github.com/kehoej/contextception/internal/model"
)

// Resolver resolves Java import paths to repository files.
type Resolver struct {
	repoRoot    string
	sourceRoots []string // repo-relative source root directories
}

// New creates a new Java resolver for the given repository root.
// It auto-detects source roots from project structure.
func New(repoRoot string) *Resolver {
	r := &Resolver{repoRoot: repoRoot}
	r.detectSourceRoots()
	return r
}

// detectSourceRoots finds Java source root directories.
// For multi-module projects (Maven/Gradle), it walks subdirectories to find
// all module-level source roots.
func (r *Resolver) detectSourceRoots() {
	// Standard suffixes to look for within each module.
	suffixes := []string{
		"src/main/java",
		"src/test/java",
	}

	// Check repo root first.
	for _, s := range suffixes {
		abs := filepath.Join(r.repoRoot, s)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			r.sourceRoots = append(r.sourceRoots, s)
		}
	}

	// Walk subdirectories looking for Java source root layouts.
	// Limit depth to 4 levels to avoid scanning deep into source trees.
	r.walkModules(r.repoRoot, "", 0, 4, suffixes)

	// Also check simple layouts.
	simpleCandidates := []string{"src", "."}
	for _, c := range simpleCandidates {
		abs := filepath.Join(r.repoRoot, c)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			r.sourceRoots = append(r.sourceRoots, c)
		}
	}

	// Deduplicate.
	seen := make(map[string]bool)
	unique := r.sourceRoots[:0]
	for _, root := range r.sourceRoots {
		if !seen[root] {
			seen[root] = true
			unique = append(unique, root)
		}
	}
	r.sourceRoots = unique

	// If nothing found, use root.
	if len(r.sourceRoots) == 0 {
		r.sourceRoots = []string{"."}
	}
}

// walkModules recursively walks directories looking for Java source root layouts
// (e.g., src/main/java). Works for both Maven/Gradle projects with per-module
// build files and Gradle projects where all configuration is in the root build.gradle.
func (r *Resolver) walkModules(absDir, relDir string, depth, maxDepth int, suffixes []string) {
	if depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden dirs and common non-module dirs.
		if strings.HasPrefix(name, ".") || name == "target" || name == "build" || name == "out" {
			continue
		}

		childAbs := filepath.Join(absDir, name)
		var childRel string
		if relDir == "" {
			childRel = name
		} else {
			childRel = relDir + "/" + name
		}

		// Check for source roots in this directory (with or without build file).
		// Gradle multi-module projects like Kafka may not have per-module build files,
		// so we scan for src/main/java directories regardless of build file presence.
		for _, s := range suffixes {
			candidate := childRel + "/" + s
			abs := filepath.Join(r.repoRoot, candidate)
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				r.sourceRoots = append(r.sourceRoots, candidate)
			}
		}

		// Continue walking deeper.
		r.walkModules(childAbs, childRel, depth+1, maxDepth, suffixes)
	}
}

// Resolve maps a Java import to a repository file.
func (r *Resolver) Resolve(srcFile string, fact model.ImportFact, repoRoot string) (model.ResolveResult, error) {
	spec := fact.Specifier

	// Same-package imports are handled by ResolveAll.
	if fact.ImportType == "same_package" {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "same_package_handled_by_resolve_all",
		}, nil
	}

	// Stdlib check.
	if javapkg.IsStdlib(spec) {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "stdlib",
		}, nil
	}

	// Strip wildcard for path resolution.
	resolveSpec := spec
	if strings.HasSuffix(resolveSpec, ".*") {
		resolveSpec = strings.TrimSuffix(resolveSpec, ".*")
	}

	// Convert dot-separated package to path: com.example.Foo → com/example/Foo.java
	filePath := strings.ReplaceAll(resolveSpec, ".", "/") + ".java"

	// Try each source root for direct file match.
	for _, root := range r.sourceRoots {
		candidate := r.joinRoot(root, filePath)
		abs := filepath.Join(r.repoRoot, candidate)
		if _, err := os.Stat(abs); err == nil {
			return model.ResolveResult{
				ResolvedPath:     candidate,
				ResolutionMethod: "package_to_file",
			}, nil
		}
	}

	// Static imports: always strip last component (member name) regardless of case.
	if fact.ImportType == "static" && !strings.HasSuffix(spec, ".*") {
		parts := strings.Split(resolveSpec, ".")
		if len(parts) >= 2 {
			classSpec := strings.Join(parts[:len(parts)-1], ".")
			if result, found := r.tryResolveSpec(classSpec, "static_import_class"); found {
				return result, nil
			}
		}
	}

	// For non-static imports, try stripping the last component when it looks
	// like a member name (lowercase) — handles static imports without the flag
	// and similar patterns.
	if fact.ImportType != "static" {
		if parts := strings.Split(resolveSpec, "."); len(parts) >= 3 {
			lastPart := parts[len(parts)-1]
			if len(lastPart) > 0 && lastPart[0] >= 'a' && lastPart[0] <= 'z' {
				classSpec := strings.Join(parts[:len(parts)-1], ".")
				if result, found := r.tryResolveSpec(classSpec, "static_import_class"); found {
					return result, nil
				}
			}
		}
	}

	// Inner type fallback: progressively strip uppercase trailing components.
	// e.g., Banner.Mode → strip .Mode → try Banner.java
	parts := strings.Split(resolveSpec, ".")
	for depth := 1; depth <= 2 && len(parts)-depth >= 2; depth++ {
		stripped := parts[len(parts)-depth]
		if len(stripped) == 0 || stripped[0] < 'A' || stripped[0] > 'Z' {
			break // stop on non-uppercase component
		}
		classSpec := strings.Join(parts[:len(parts)-depth], ".")
		if result, found := r.tryResolveSpec(classSpec, "inner_type"); found {
			return result, nil
		}
	}

	// Not found locally → external third-party.
	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "external",
		Reason:           "third_party",
	}, nil
}

// ResolveAll implements MultiResolver for Java. Handles same_package imports
// by resolving to sibling .java files in the same directory.
func (r *Resolver) ResolveAll(srcFile string, fact model.ImportFact, repoRoot string) ([]model.ResolveResult, error) {
	if fact.ImportType != "same_package" {
		// Delegate non-same-package to single Resolve.
		result, err := r.Resolve(srcFile, fact, repoRoot)
		if err != nil {
			return nil, err
		}
		return []model.ResolveResult{result}, nil
	}

	// Same-package: resolve to sibling .java files in same directory.
	srcDir := filepath.Dir(srcFile)
	absDir := filepath.Join(r.repoRoot, srcDir)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return []model.ResolveResult{{
			External:         true,
			ResolutionMethod: "same_package",
			Reason:           "not_found",
		}}, nil
	}

	srcBase := filepath.Base(srcFile)
	var results []model.ResolveResult
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".java") || name == srcBase {
			continue
		}
		// Skip test files as siblings.
		stem := strings.TrimSuffix(name, ".java")
		if strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests") || strings.HasPrefix(stem, "Test") {
			continue
		}
		p := filepath.Join(srcDir, name)
		p = filepath.ToSlash(p)
		results = append(results, model.ResolveResult{
			ResolvedPath:     p,
			ResolutionMethod: "same_package",
		})
	}

	if len(results) == 0 {
		return []model.ResolveResult{{
			External:         true,
			ResolutionMethod: "same_package",
			Reason:           "no_siblings",
		}}, nil
	}

	// Cap to prevent explosion in large packages.
	const maxSamePackage = 10
	if len(results) > maxSamePackage {
		results = results[:maxSamePackage]
	}

	return results, nil
}

// tryResolveSpec attempts to resolve a dot-separated specifier to a file.
// Returns (result, true) if found, (zero, false) otherwise.
func (r *Resolver) tryResolveSpec(spec, method string) (model.ResolveResult, bool) {
	filePath := strings.ReplaceAll(spec, ".", "/") + ".java"
	for _, root := range r.sourceRoots {
		candidate := r.joinRoot(root, filePath)
		abs := filepath.Join(r.repoRoot, candidate)
		if _, err := os.Stat(abs); err == nil {
			return model.ResolveResult{
				ResolvedPath:     candidate,
				ResolutionMethod: method,
			}, true
		}
	}
	return model.ResolveResult{}, false
}

// joinRoot joins a source root with a file path, handling the "." root case.
func (r *Resolver) joinRoot(root, filePath string) string {
	var candidate string
	if root == "." {
		candidate = filePath
	} else {
		candidate = filepath.Join(root, filePath)
	}
	return filepath.ToSlash(candidate)
}
