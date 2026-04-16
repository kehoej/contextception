// Package csharp implements import resolution for C# source files.
package csharp

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"

	csharppkg "github.com/kehoej/contextception/internal/extractor/csharp"
	"github.com/kehoej/contextception/internal/model"
)

// Resolver resolves C# import paths to repository files.
type Resolver struct {
	repoRoot    string
	sourceRoots []string // repo-relative directories containing .csproj files (or ".")

	// projectNamespaceMap maps dotted project directory names to their repo-relative paths.
	// e.g., "MediaBrowser.Controller" → "MediaBrowser.Controller"
	//       "Jellyfin.Database.Implementations" → "src/Jellyfin.Database/Jellyfin.Database.Implementations"
	// This handles the C# convention where project directories use dots in their names.
	projectNamespaceMap map[string]string
}

// New creates a new C# resolver for the given repository root.
// It auto-detects source roots from .csproj project files.
func New(repoRoot string) *Resolver {
	r := &Resolver{
		repoRoot:            repoRoot,
		projectNamespaceMap: make(map[string]string),
	}
	r.detectSourceRoots()
	return r
}

// detectSourceRoots finds C# project directories by scanning for .csproj files.
// Each directory containing a .csproj is treated as a source root since C# source
// files live alongside their project files (no src/main/java convention).
func (r *Resolver) detectSourceRoots() {
	r.walkForProjects(r.repoRoot, "", 0, 5)

	// Also check simple layouts.
	for _, c := range []string{"src", "."} {
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

	// Also register directory basenames as project namespace keys (handles the common
	// case where the directory name IS the project name, e.g., "MediaBrowser.Controller/").
	for _, root := range r.sourceRoots {
		if root == "." || root == "src" {
			continue
		}
		dirName := filepath.Base(root)
		if _, exists := r.projectNamespaceMap[dirName]; !exists {
			r.projectNamespaceMap[dirName] = root
		}
	}
}

// walkForProjects recursively walks directories looking for .csproj files.
func (r *Resolver) walkForProjects(absDir, relDir string, depth, maxDepth int) {
	if depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}

	var csprojNames []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, ext := range []string{".csproj", ".shproj"} {
			if strings.HasSuffix(name, ext) {
				csprojNames = append(csprojNames, strings.TrimSuffix(name, ext))
			}
		}
	}

	if len(csprojNames) > 0 {
		root := relDir
		if root == "" {
			root = "."
		}
		r.sourceRoots = append(r.sourceRoots, root)
		// Register the csproj name(s) as project namespace keys.
		// This handles cases where the csproj name differs from the directory name
		// (e.g., src/Compilers/Core/Portable/Microsoft.CodeAnalysis.csproj).
		for _, name := range csprojNames {
			r.projectNamespaceMap[name] = root
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden dirs and common non-source dirs.
		if strings.HasPrefix(name, ".") || name == "bin" || name == "obj" || name == "packages" || name == "TestResults" {
			continue
		}

		childAbs := filepath.Join(absDir, name)
		var childRel string
		if relDir == "" {
			childRel = name
		} else {
			childRel = relDir + "/" + name
		}

		r.walkForProjects(childAbs, childRel, depth+1, maxDepth)
	}
}

// Resolve maps a C# using directive to a repository file.
func (r *Resolver) Resolve(srcFile string, fact model.ImportFact, repoRoot string) (model.ResolveResult, error) {
	spec := fact.Specifier

	// Same-namespace imports are handled by ResolveAll.
	if fact.ImportType == "same_namespace" {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "same_namespace_handled_by_resolve_all",
		}, nil
	}

	// Stdlib check — but skip it if the namespace matches a local project or
	// can be found as a subdirectory within a source root. This handles repos
	// like Roslyn (Microsoft.CodeAnalysis.*) and EF Core (Microsoft.EntityFrameworkCore.*)
	// where the namespace uses a stdlib prefix but the code is in-repo.
	if csharppkg.IsStdlib(spec) && !r.matchesLocalProject(spec) && !r.canResolveLocally(spec) {
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

	// Strategy 1: Project namespace mapping.
	// C# projects use dotted directory names (e.g., "MediaBrowser.Controller/").
	// A using like "MediaBrowser.Controller.Entities" means:
	//   - The project root is "MediaBrowser.Controller"
	//   - The sub-path within the project is "Entities"
	// We try matching the longest project namespace prefix first.
	if result, found := r.resolveViaProjectNamespace(resolveSpec, srcFile); found {
		return result, nil
	}

	// Strategy 2: Convert namespace to path directly: Foo.Bar.Baz → Foo/Bar/Baz.cs
	// Works for simple projects where namespace components match directory segments.
	filePath := strings.ReplaceAll(resolveSpec, ".", "/") + ".cs"
	for _, root := range r.sourceRoots {
		candidate := r.joinRoot(root, filePath)
		abs := filepath.Join(r.repoRoot, candidate)
		if _, err := os.Stat(abs); err == nil {
			return model.ResolveResult{
				ResolvedPath:     candidate,
				ResolutionMethod: "namespace_to_file",
			}, nil
		}
	}

	// Strategy 3: Try namespace suffix as subdirectory within each source root.
	// Handles cases where the project name differs from the namespace
	// (e.g., project "Avalonia.Base" contains namespace "Avalonia.Media" at Media/).
	parts := strings.Split(resolveSpec, ".")
	for i := 1; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], "/")
		for _, root := range r.sourceRoots {
			if root == "." || root == "src" {
				continue
			}
			dirPath := filepath.ToSlash(filepath.Join(root, suffix))
			absDirPath := filepath.Join(r.repoRoot, dirPath)
			if info, err := os.Stat(absDirPath); err == nil && info.IsDir() {
				csFiles := collectCSFiles(absDirPath, dirPath)
				if len(csFiles) > 0 {
					p := pickRepresentativeFile(csFiles, srcFile)
					return model.ResolveResult{
						ResolvedPath:     p,
						ResolutionMethod: "namespace_subdir",
					}, nil
				}
			}
			// Also try as a file: e.g., Avalonia.Threading → root/Threading.cs
			fileSuffix := suffix + ".cs"
			filePath := filepath.ToSlash(filepath.Join(root, fileSuffix))
			abs := filepath.Join(r.repoRoot, filePath)
			if _, err := os.Stat(abs); err == nil {
				return model.ResolveResult{
					ResolvedPath:     filePath,
					ResolutionMethod: "namespace_subdir",
				}, nil
			}
		}
	}

	// Strategy 4: Try last component as filename (C# namespaces don't map 1:1 to paths).
	// e.g., MyApp.Services.UserService → try UserService.cs in all source roots.
	if len(parts) >= 2 {
		lastComponent := parts[len(parts)-1] + ".cs"
		for _, root := range r.sourceRoots {
			if result, found := r.findFileRecursive(root, lastComponent); found {
				return result, nil
			}
		}
	}

	// Not found locally → external (NuGet package).
	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "external",
		Reason:           "third_party",
	}, nil
}

// resolveViaProjectNamespace tries to resolve a namespace specifier by matching
// its prefix against known project directory names (dotted names like "MediaBrowser.Controller").
//
// For example, with source root "MediaBrowser.Controller" and spec "MediaBrowser.Controller.Entities":
//   - Strips prefix → remainder is "Entities"
//   - Looks for "MediaBrowser.Controller/Entities.cs" (file) or
//     "MediaBrowser.Controller/Entities/" (directory with .cs files)
func (r *Resolver) resolveViaProjectNamespace(spec, srcFile string) (model.ResolveResult, bool) {
	// Try longest prefix match first (more specific project names first).
	// Sort by length descending for correctness.
	var bestMatch string
	var bestRoot string
	for projName, projRoot := range r.projectNamespaceMap {
		if !strings.HasPrefix(spec, projName) {
			continue
		}
		// Must be exact prefix: either spec == projName, or next char is '.'.
		if len(spec) > len(projName) && spec[len(projName)] != '.' {
			continue
		}
		if len(projName) > len(bestMatch) {
			bestMatch = projName
			bestRoot = projRoot
		}
	}

	if bestMatch == "" {
		return model.ResolveResult{}, false
	}

	// Strip the matched prefix to get the remainder.
	var remainder string
	if len(spec) > len(bestMatch) {
		remainder = spec[len(bestMatch)+1:] // skip the dot
	}

	// Determine the target path within the project.
	var targetDir string
	if remainder == "" {
		// The spec exactly matches the project namespace (e.g., "Orleans.Runtime").
		// The target is the project root directory itself.
		targetDir = bestRoot
	} else {
		// Convert remainder to subpath: "Entities.Genre" → "Entities/Genre"
		subPath := strings.ReplaceAll(remainder, ".", "/")

		// Try as a file: project_root/sub/path.cs
		filePath := subPath + ".cs"
		candidate := filepath.ToSlash(filepath.Join(bestRoot, filePath))
		abs := filepath.Join(r.repoRoot, candidate)
		if _, err := os.Stat(abs); err == nil {
			return model.ResolveResult{
				ResolvedPath:     candidate,
				ResolutionMethod: "project_namespace",
			}, true
		}

		targetDir = filepath.ToSlash(filepath.Join(bestRoot, subPath))
	}

	// Try as a directory: the using imports a namespace, not a file.
	// e.g., "using MediaBrowser.Controller.Entities;" → MediaBrowser.Controller/Entities/
	// Pick a representative .cs file using context-aware selection.
	dirPath := targetDir
	absDirPath := filepath.Join(r.repoRoot, dirPath)
	if info, err := os.Stat(absDirPath); err == nil && info.IsDir() {
		csFiles := collectCSFiles(absDirPath, dirPath)
		if len(csFiles) > 0 {
			p := pickRepresentativeFile(csFiles, srcFile)
			return model.ResolveResult{
				ResolvedPath:     p,
				ResolutionMethod: "project_namespace",
			}, true
		}
		// No .cs files at this level — try first subdirectory that has .cs files.
		entries, err := os.ReadDir(absDirPath)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				subDirAbs := filepath.Join(absDirPath, e.Name())
				subDirRel := filepath.ToSlash(filepath.Join(dirPath, e.Name()))
				csFiles := collectCSFiles(subDirAbs, subDirRel)
				if len(csFiles) > 0 {
					p := pickRepresentativeFile(csFiles, srcFile)
					return model.ResolveResult{
						ResolvedPath:     p,
						ResolutionMethod: "project_namespace",
					}, true
				}
			}
		}
	}

	return model.ResolveResult{}, false
}

// ResolveAll implements MultiResolver for C#. Handles same_namespace imports
// by resolving to sibling .cs files in the same directory. Also expands
// namespace-level using directives to all .cs files in the target directory.
func (r *Resolver) ResolveAll(srcFile string, fact model.ImportFact, repoRoot string) ([]model.ResolveResult, error) {
	if fact.ImportType == "same_namespace" {
		return r.resolveSameNamespace(srcFile)
	}

	// For absolute/alias/static using directives, delegate to single Resolve.
	// Namespace-directory expansion is NOT done here — the same_namespace synthetic
	// fact handles sibling discovery (like Java's same_package). This prevents
	// namespace-level using directives from flooding must_read with all files
	// in the target directory.
	result, err := r.Resolve(srcFile, fact, repoRoot)
	if err != nil {
		return nil, err
	}
	return []model.ResolveResult{result}, nil
}

// resolveSameNamespace resolves same_namespace facts to sibling .cs files.
func (r *Resolver) resolveSameNamespace(srcFile string) ([]model.ResolveResult, error) {
	srcDir := filepath.Dir(srcFile)
	absDir := filepath.Join(r.repoRoot, srcDir)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return []model.ResolveResult{{
			External:         true,
			ResolutionMethod: "same_namespace",
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
		if !strings.HasSuffix(name, ".cs") || name == srcBase {
			continue
		}
		// Skip test files as siblings.
		stem := strings.TrimSuffix(name, ".cs")
		if strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests") || strings.HasPrefix(stem, "Test") {
			continue
		}
		p := filepath.Join(srcDir, name)
		p = filepath.ToSlash(p)
		results = append(results, model.ResolveResult{
			ResolvedPath:     p,
			ResolutionMethod: "same_namespace",
		})
	}

	if len(results) == 0 {
		return []model.ResolveResult{{
			External:         true,
			ResolutionMethod: "same_namespace",
			Reason:           "no_siblings",
		}}, nil
	}

	// Cap to prevent explosion in large namespaces.
	const maxSameNamespace = 10
	if len(results) > maxSameNamespace {
		results = results[:maxSameNamespace]
	}

	return results, nil
}

// pickRepresentativeFile selects a .cs file from a directory to represent a
// namespace-level import edge. Instead of always picking the alphabetically first
// file (which creates artificial hub nodes), this uses two strategies:
//  1. Prefix match: prefer files whose name matches the source file's stem
//     (e.g., SessionManager.cs importing Controller.Session → pick ISessionManager.cs)
//  2. Hash-based rotation: if no prefix match, hash the source file path to pick
//     a deterministic but distributed file from the directory.
//
// This prevents any single file from accumulating all namespace-level edges.
func pickRepresentativeFile(csFiles []string, srcFile string) string {
	if len(csFiles) == 0 {
		return ""
	}

	// Strategy 1: Find a file whose stem matches or is prefixed by the source stem.
	srcStem := strings.TrimSuffix(filepath.Base(srcFile), ".cs")
	if srcStem != "" {
		// Try exact stem match first (e.g., SessionManager → ISessionManager.cs or SessionManager.cs)
		for _, f := range csFiles {
			fStem := strings.TrimSuffix(filepath.Base(f), ".cs")
			// Match: same stem, or I+stem (interface), or stem without I prefix
			if strings.EqualFold(fStem, srcStem) ||
				strings.EqualFold(fStem, "I"+srcStem) ||
				(len(srcStem) > 1 && srcStem[0] == 'I' && strings.EqualFold(fStem, srcStem[1:])) {
				return f
			}
		}
		// Try prefix match (e.g., BaseItem → BaseItemExtensions.cs)
		for _, f := range csFiles {
			fStem := strings.TrimSuffix(filepath.Base(f), ".cs")
			if len(fStem) > len(srcStem) && strings.HasPrefix(fStem, srcStem) {
				return f
			}
		}
	}

	// Strategy 2: Prefer interface files (I*.cs) as representatives — they're
	// more stable and informative than implementations.
	var interfaces []string
	for _, f := range csFiles {
		fBase := filepath.Base(f)
		if len(fBase) > 2 && fBase[0] == 'I' && fBase[1] >= 'A' && fBase[1] <= 'Z' {
			interfaces = append(interfaces, f)
		}
	}

	// Strategy 3: Hash-based rotation to distribute edges.
	pool := csFiles
	if len(interfaces) > 0 {
		pool = interfaces
	}
	h := fnv.New32a()
	h.Write([]byte(srcFile))
	idx := int(h.Sum32()) % len(pool)
	if idx < 0 {
		idx = -idx
	}
	return pool[idx]
}

// collectCSFiles returns all .cs filenames (repo-relative) in a directory.
func collectCSFiles(absDir, relDir string) []string {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cs") {
			files = append(files, filepath.ToSlash(filepath.Join(relDir, e.Name())))
		}
	}
	return files
}

// canResolveLocally does a quick check if the namespace suffix exists as a subdirectory
// within any source root. This is cheaper than full resolution and used to skip stdlib
// classification for in-repo namespaces whose project name doesn't match the namespace.
func (r *Resolver) canResolveLocally(spec string) bool {
	parts := strings.Split(spec, ".")
	for i := 1; i < len(parts); i++ {
		suffix := strings.Join(parts[i:], "/")
		for _, root := range r.sourceRoots {
			if root == "." || root == "src" {
				continue
			}
			dirPath := filepath.Join(r.repoRoot, root, suffix)
			if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
				return true
			}
			// Also try as a file.
			filePath := dirPath + ".cs"
			if _, err := os.Stat(filePath); err == nil {
				return true
			}
		}
	}
	return false
}

// matchesLocalProject returns true if the namespace spec could resolve to a local project.
// This is used to override stdlib classification for repos that contain Microsoft.* or System.*
// projects (e.g., Roslyn contains Microsoft.CodeAnalysis.*).
func (r *Resolver) matchesLocalProject(spec string) bool {
	for projName := range r.projectNamespaceMap {
		if strings.HasPrefix(spec, projName) {
			if len(spec) == len(projName) || spec[len(projName)] == '.' {
				return true
			}
		}
	}
	return false
}

// findFileRecursive searches a source root for a file by name (non-recursive walk limited to depth 5).
func (r *Resolver) findFileRecursive(root, fileName string) (model.ResolveResult, bool) {
	absRoot := filepath.Join(r.repoRoot, root)
	if root == "." {
		absRoot = r.repoRoot
	}

	var found string
	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "bin" || name == "obj" || name == "node_modules" || name == "packages" {
				return filepath.SkipDir
			}
			// Limit depth.
			rel, _ := filepath.Rel(absRoot, path)
			if strings.Count(filepath.ToSlash(rel), "/") >= 5 {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == fileName {
			rel, _ := filepath.Rel(r.repoRoot, path)
			found = filepath.ToSlash(rel)
			return filepath.SkipAll
		}
		return nil
	})

	if found != "" {
		return model.ResolveResult{
			ResolvedPath:     found,
			ResolutionMethod: "filename_search",
		}, true
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
