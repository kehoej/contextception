// Package golang implements import resolution for Go source files.
package golang

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kehoej/contextception/internal/model"
)

const maxFilesPerPackage = 1
const maxSamePackageFiles = 1

// Resolver resolves Go import paths to repository files using go.mod.
type Resolver struct {
	repoRoot   string
	modulePath string        // from go.mod (e.g., "github.com/kehoej/contextception")
	modules    []moduleEntry // all modules (primary + go.work use directives)
}

// moduleEntry represents a Go module found in the repo.
type moduleEntry struct {
	path string // module path from go.mod
	dir  string // repo-relative directory
}

// goModInfo holds parsed information from a go.mod file.
type goModInfo struct {
	modulePath string            // module path (e.g., "github.com/user/repo")
	replaces   map[string]string // old module path → local replacement directory (relative to go.mod dir)
}

// New creates a new Go resolver for the given repository root.
// It parses go.mod at the root and optionally go.work for multi-module repos.
func New(repoRoot string) *Resolver {
	r := &Resolver{repoRoot: repoRoot}
	r.detectModules()
	return r
}

// detectModules reads go.mod and optionally go.work to find all modules.
func (r *Resolver) detectModules() {
	// Try go.work first for multi-module repos.
	if workModules := r.parseGoWork(); len(workModules) > 0 {
		r.modules = workModules
		if len(workModules) > 0 {
			r.modulePath = workModules[0].path
		}
		return
	}

	// Single module: parse go.mod at root.
	info := r.parseGoMod(r.repoRoot)
	if info.modulePath != "" {
		r.modulePath = info.modulePath
		r.modules = []moduleEntry{{path: info.modulePath, dir: "."}}

		// Add local replace directives as additional module entries.
		for oldPath, localDir := range info.replaces {
			// Read the replaced module's go.mod to get its module path.
			absDir := filepath.Join(r.repoRoot, localDir)
			replacedInfo := r.parseGoMod(absDir)
			if replacedInfo.modulePath != "" {
				r.modules = append(r.modules, moduleEntry{path: replacedInfo.modulePath, dir: localDir})
			} else {
				// Use the old module path for matching (common for in-repo replaces).
				r.modules = append(r.modules, moduleEntry{path: oldPath, dir: localDir})
			}
		}
	}
}

// parseGoMod reads the module path and replace directives from a go.mod file.
func (r *Resolver) parseGoMod(dir string) goModInfo {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return goModInfo{}
	}

	info := goModInfo{replaces: make(map[string]string)}
	inReplace := false

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "module ") {
			info.modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module"))
			continue
		}

		// Block replace: replace ( ... )
		if line == "replace (" || strings.HasPrefix(line, "replace (") {
			inReplace = true
			continue
		}
		if inReplace {
			if line == ")" {
				inReplace = false
				continue
			}
			if old, local, ok := parseReplaceLine(line); ok {
				info.replaces[old] = local
			}
			continue
		}

		// Single-line replace: replace old => ./local
		if strings.HasPrefix(line, "replace ") {
			rest := strings.TrimPrefix(line, "replace ")
			if old, local, ok := parseReplaceLine(rest); ok {
				info.replaces[old] = local
			}
		}
	}

	return info
}

// parseReplaceLine parses a replace directive line like "old/mod v1.0.0 => ./local/dir".
// Returns (oldModPath, localDir, ok). Only returns ok=true for local replacements (starting with ./ or ../).
func parseReplaceLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "//") {
		return "", "", false
	}

	parts := strings.SplitN(line, "=>", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	// Left side: "old/module" or "old/module v1.0.0"
	leftFields := strings.Fields(strings.TrimSpace(parts[0]))
	if len(leftFields) == 0 {
		return "", "", false
	}
	oldPath := leftFields[0]

	// Right side: "./local/dir" or "../local/dir" (optionally with version)
	rightFields := strings.Fields(strings.TrimSpace(parts[1]))
	if len(rightFields) == 0 {
		return "", "", false
	}
	newPath := rightFields[0]

	// Only track local replacements.
	if !strings.HasPrefix(newPath, "./") && !strings.HasPrefix(newPath, "../") {
		return "", "", false
	}

	return oldPath, newPath, true
}

// parseGoWork reads go.work and returns module entries from `use` directives.
func (r *Resolver) parseGoWork() []moduleEntry {
	data, err := os.ReadFile(filepath.Join(r.repoRoot, "go.work"))
	if err != nil {
		return nil
	}

	var entries []moduleEntry
	inUse := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "use (") || line == "use (" {
			inUse = true
			continue
		}
		if inUse {
			if line == ")" {
				inUse = false
				continue
			}
			dir := strings.TrimSpace(line)
			if dir == "" || strings.HasPrefix(dir, "//") {
				continue
			}
			absDir := filepath.Join(r.repoRoot, dir)
			info := r.parseGoMod(absDir)
			if info.modulePath != "" {
				entries = append(entries, moduleEntry{path: info.modulePath, dir: dir})
				// Add local replace directives from this module.
				for oldPath, localDir := range info.replaces {
					replaceAbs := filepath.Join(absDir, localDir)
					replacedInfo := r.parseGoMod(replaceAbs)
					if replacedInfo.modulePath != "" {
						relDir := filepath.ToSlash(filepath.Join(dir, localDir))
						entries = append(entries, moduleEntry{path: replacedInfo.modulePath, dir: relDir})
					} else {
						relDir := filepath.ToSlash(filepath.Join(dir, localDir))
						entries = append(entries, moduleEntry{path: oldPath, dir: relDir})
					}
				}
			}
			continue
		}

		// Single-line use directive: use ./subdir
		if strings.HasPrefix(line, "use ") {
			dir := strings.TrimSpace(strings.TrimPrefix(line, "use"))
			absDir := filepath.Join(r.repoRoot, dir)
			info := r.parseGoMod(absDir)
			if info.modulePath != "" {
				entries = append(entries, moduleEntry{path: info.modulePath, dir: dir})
				for oldPath, localDir := range info.replaces {
					replaceAbs := filepath.Join(absDir, localDir)
					replacedInfo := r.parseGoMod(replaceAbs)
					if replacedInfo.modulePath != "" {
						relDir := filepath.ToSlash(filepath.Join(dir, localDir))
						entries = append(entries, moduleEntry{path: replacedInfo.modulePath, dir: relDir})
					} else {
						relDir := filepath.ToSlash(filepath.Join(dir, localDir))
						entries = append(entries, moduleEntry{path: oldPath, dir: relDir})
					}
				}
			}
		}
	}

	return entries
}

// Resolve maps a Go import to a single repository file (the first result from ResolveAll).
func (r *Resolver) Resolve(srcFile string, fact model.ImportFact, repoRoot string) (model.ResolveResult, error) {
	results, err := r.ResolveAll(srcFile, fact, repoRoot)
	if err != nil {
		return model.ResolveResult{}, err
	}
	return results[0], nil
}

// ResolveAll resolves a Go import to all target files in the package directory.
// Go imports target directories, not individual files, so a single import can
// resolve to multiple .go files.
func (r *Resolver) ResolveAll(srcFile string, fact model.ImportFact, repoRoot string) ([]model.ResolveResult, error) {
	spec := fact.Specifier

	// Cgo pseudo-package.
	if spec == "C" {
		return []model.ResolveResult{{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "cgo",
		}}, nil
	}

	// Stdlib.
	firstElem := spec
	if idx := strings.Index(spec, "/"); idx >= 0 {
		firstElem = spec[:idx]
	}
	if !strings.Contains(firstElem, ".") {
		return []model.ResolveResult{{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "stdlib",
		}}, nil
	}

	// Try each module.
	for _, mod := range r.modules {
		if strings.HasPrefix(spec, mod.path) {
			relPath := strings.TrimPrefix(spec, mod.path)
			relPath = strings.TrimPrefix(relPath, "/")

			var pkgDir string
			if mod.dir == "." {
				pkgDir = relPath
			} else {
				pkgDir = filepath.Join(mod.dir, relPath)
			}

			absDir := filepath.Join(r.repoRoot, pkgDir)
			goFiles := findGoFiles(absDir)

			if len(goFiles) == 0 {
				return []model.ResolveResult{{
					External:         true,
					ResolutionMethod: "module_local",
					Reason:           "not_found",
				}}, nil
			}

			// Cap per-package file explosion.
			goFiles = prioritizeGoFiles(path.Base(spec), goFiles)

			var results []model.ResolveResult
			for _, f := range goFiles {
				p := filepath.Join(pkgDir, f)
				p = filepath.ToSlash(p)
				results = append(results, model.ResolveResult{
					ResolvedPath:     p,
					ResolutionMethod: "module_local",
				})
			}
			return results, nil
		}
	}

	return []model.ResolveResult{{
		External:         true,
		ResolutionMethod: "external",
		Reason:           "third_party",
	}}, nil
}

// ResolveSamePackageEdges returns sibling .go files in the same directory as srcFile.
// These represent Go's implicit same-package visibility.
func (r *Resolver) ResolveSamePackageEdges(srcFile, repoRoot string) []model.ResolveResult {
	srcDir := filepath.Dir(srcFile)
	absDir := filepath.Join(r.repoRoot, srcDir)
	goFiles := findGoFiles(absDir)

	srcBase := filepath.Base(srcFile)
	var siblings []string
	for _, f := range goFiles {
		if f != srcBase {
			siblings = append(siblings, f)
		}
	}

	// Apply prioritization/cap (separate cap for same-package edges).
	pkgName := filepath.Base(srcDir)
	siblings = prioritizeGoFilesWithCap(pkgName, siblings, maxSamePackageFiles)

	var results []model.ResolveResult
	for _, f := range siblings {
		p := filepath.Join(srcDir, f)
		p = filepath.ToSlash(p)
		results = append(results, model.ResolveResult{
			ResolvedPath:     p,
			ResolutionMethod: "same_package",
		})
	}
	return results
}

// prioritizeGoFiles selects at most maxFilesPerPackage files using platform-aware tiers.
func prioritizeGoFiles(pkgName string, files []string) []string {
	return prioritizeGoFilesWithCap(pkgName, files, maxFilesPerPackage)
}

// prioritizeGoFilesWithCap selects at most cap files, prioritizing:
// 1. Canonical (<pkgName>.go), non-platform
// 2. Non-platform, non-doc files (alphabetical)
// 3. Platform-specific files (alphabetical)
// Excluded: doc.go
func prioritizeGoFilesWithCap(pkgName string, files []string, cap int) []string {
	if len(files) <= cap {
		return files
	}

	canonical := pkgName + ".go"
	var tier1, tier2, tier3 []string
	for _, f := range files {
		if f == "doc.go" {
			continue
		}
		if f == canonical && !isPlatformSpecific(f) {
			tier1 = append(tier1, f)
		} else if !isPlatformSpecific(f) {
			tier2 = append(tier2, f)
		} else {
			tier3 = append(tier3, f)
		}
	}

	var result []string
	result = append(result, tier1...)
	result = append(result, tier2...)
	result = append(result, tier3...)
	if len(result) > cap {
		result = result[:cap]
	}
	return result
}

// isPlatformSpecific returns true if the filename matches Go build-constraint
// naming conventions (*_GOOS.go, *_GOARCH.go, *_GOOS_GOARCH.go).
func isPlatformSpecific(name string) bool {
	// Strip .go suffix and check trailing segments.
	base := strings.TrimSuffix(name, ".go")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return false
	}
	// Check last 1-2 segments against known GOOS/GOARCH values.
	last := parts[len(parts)-1]
	if goosValues[last] || goarchValues[last] {
		return true
	}
	if len(parts) >= 3 {
		secondLast := parts[len(parts)-2]
		if goosValues[secondLast] && goarchValues[last] {
			return true
		}
	}
	return false
}

var goosValues = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true,
	"freebsd": true, "hurd": true, "illumos": true, "ios": true,
	"js": true, "linux": true, "nacl": true, "netbsd": true,
	"openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
	"windows": true, "zos": true,
}

var goarchValues = map[string]bool{
	"386": true, "amd64": true, "arm": true, "arm64": true,
	"loong64": true, "mips": true, "mips64": true, "mips64le": true,
	"mipsle": true, "ppc64": true, "ppc64le": true, "riscv64": true,
	"s390x": true, "wasm": true,
}

// findGoFiles returns sorted non-test .go filenames in a directory.
func findGoFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		// Skip test files.
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Skip files starting with _ or . (build-ignored by Go convention).
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		files = append(files, name)
	}
	// Already sorted by os.ReadDir.
	return files
}
