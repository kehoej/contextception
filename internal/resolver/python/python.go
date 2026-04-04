// Package python implements import resolution for Python source files.
package python

import (
	"os"
	"path/filepath"
	"strings"

	pyext "github.com/kehoej/contextception/internal/extractor/python"
	"github.com/kehoej/contextception/internal/model"
	"github.com/kehoej/contextception/internal/resolver"
)

// PackageRoot represents a detected Python package root directory.
type PackageRoot struct {
	Path            string // repo-relative path (e.g., "src", ".")
	DetectionMethod string // "pyproject_toml", "setup_py", "setup_cfg", "src_layout", "repo_root"
}

// Resolver resolves Python import specifiers to repository files.
type Resolver struct {
	repoRoot     string
	packageRoots []PackageRoot
}

// New creates a new Python resolver for the given repository root.
func New(repoRoot string) *Resolver {
	return &Resolver{
		repoRoot: repoRoot,
	}
}

// DetectPackageRoots scans the repository for Python package root directories.
func (r *Resolver) DetectPackageRoots() ([]PackageRoot, error) {
	var roots []PackageRoot

	// Check repo root for pyproject.toml, setup.py, setup.cfg.
	if fileExists(filepath.Join(r.repoRoot, "pyproject.toml")) {
		roots = append(roots, PackageRoot{Path: ".", DetectionMethod: "pyproject_toml"})
		// Check for src/ layout next to pyproject.toml.
		if dirExists(filepath.Join(r.repoRoot, "src")) {
			roots = append(roots, PackageRoot{Path: "src", DetectionMethod: "src_layout"})
		}
	}
	if fileExists(filepath.Join(r.repoRoot, "setup.py")) {
		roots = appendIfNewPath(roots, PackageRoot{Path: ".", DetectionMethod: "setup_py"})
		if dirExists(filepath.Join(r.repoRoot, "src")) {
			roots = appendIfNewPath(roots, PackageRoot{Path: "src", DetectionMethod: "src_layout"})
		}
	}
	if fileExists(filepath.Join(r.repoRoot, "setup.cfg")) {
		roots = appendIfNewPath(roots, PackageRoot{Path: ".", DetectionMethod: "setup_cfg"})
		if dirExists(filepath.Join(r.repoRoot, "src")) {
			roots = appendIfNewPath(roots, PackageRoot{Path: "src", DetectionMethod: "src_layout"})
		}
	}

	// Scan first-level subdirs for monorepo support.
	entries, err := os.ReadDir(r.repoRoot)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		subdir := entry.Name()
		absSubdir := filepath.Join(r.repoRoot, subdir)
		if fileExists(filepath.Join(absSubdir, "pyproject.toml")) || fileExists(filepath.Join(absSubdir, "setup.py")) {
			roots = appendIfNewPath(roots, PackageRoot{Path: subdir, DetectionMethod: "monorepo_subdir"})
			if dirExists(filepath.Join(absSubdir, "src")) {
				roots = appendIfNewPath(roots, PackageRoot{Path: filepath.Join(subdir, "src"), DetectionMethod: "src_layout"})
			}
		}
	}

	// Fallback: repo root always present.
	roots = appendIfNewPath(roots, PackageRoot{Path: ".", DetectionMethod: "repo_root"})

	r.packageRoots = roots
	return roots, nil
}

// SetPackageRoots allows setting package roots directly (used after loading from DB).
func (r *Resolver) SetPackageRoots(roots []PackageRoot) {
	r.packageRoots = roots
}

// Resolve maps a Python import to a repository file.
func (r *Resolver) Resolve(srcFile string, fact model.ImportFact, repoRoot string) (model.ResolveResult, error) {
	spec := fact.Specifier

	// Relative imports.
	if fact.ImportType == "relative" {
		return r.resolveRelative(srcFile, fact), nil
	}

	// Absolute imports: check stdlib first.
	topLevel := strings.SplitN(spec, ".", 2)[0]
	if pyext.IsStdlib(topLevel) {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "stdlib",
			Reason:           "stdlib",
		}, nil
	}

	// Try each package root.
	return r.resolveAbsolute(fact), nil
}

// resolveRelative handles relative import resolution.
func (r *Resolver) resolveRelative(srcFile string, fact model.ImportFact) model.ResolveResult {
	spec := fact.Specifier

	// Count leading dots.
	dots := 0
	for _, ch := range spec {
		if ch == '.' {
			dots++
		} else {
			break
		}
	}
	remainder := spec[dots:]

	// Find the package directory of the importing file.
	srcDir := filepath.Dir(srcFile)

	// Navigate up (dots - 1) levels.
	baseDir := srcDir
	for i := 1; i < dots; i++ {
		baseDir = filepath.Dir(baseDir)
		if baseDir == "." || baseDir == "" {
			return model.ResolveResult{
				External:         true,
				ResolutionMethod: "relative",
				Reason:           "relative import escapes repository",
			}
		}
	}

	// If there's a remainder after the dots, resolve it.
	if remainder != "" {
		// First try: the imported names might be submodules.
		if len(fact.ImportedNames) > 0 && fact.ImportedNames[0] != "*" {
			for _, name := range fact.ImportedNames {
				subSpec := remainder + "." + name
				if resolved := resolveModulePath(r.repoRoot, baseDir, subSpec); resolved != "" {
					return model.ResolveResult{
						ResolvedPath:     resolved,
						ResolutionMethod: "relative",
					}
				}
			}
		}
		// Then try the specifier itself.
		if resolved := resolveModulePath(r.repoRoot, baseDir, remainder); resolved != "" {
			return model.ResolveResult{
				ResolvedPath:     resolved,
				ResolutionMethod: "relative",
			}
		}
	} else {
		// "from . import X" — X might be a module in the same package.
		if len(fact.ImportedNames) > 0 {
			for _, name := range fact.ImportedNames {
				if name == "*" {
					continue
				}
				if resolved := resolveModulePath(r.repoRoot, baseDir, name); resolved != "" {
					return model.ResolveResult{
						ResolvedPath:     resolved,
						ResolutionMethod: "relative",
					}
				}
			}
		}
		// Check for __init__.py in the base dir itself.
		initPath := filepath.Join(baseDir, "__init__.py")
		if fileExists(filepath.Join(r.repoRoot, initPath)) {
			return model.ResolveResult{
				ResolvedPath:     initPath,
				ResolutionMethod: "relative",
			}
		}
	}

	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "relative",
		Reason:           "not_found",
	}
}

// resolveAbsolute handles absolute import resolution across package roots.
func (r *Resolver) resolveAbsolute(fact model.ImportFact) model.ResolveResult {
	spec := fact.Specifier

	for _, root := range r.packageRoots {
		rootAbs := root.Path
		if rootAbs == "." {
			rootAbs = ""
		}

		// When ImportedNames are present, check if they resolve as submodules.
		// This determines whether the import targets the package or its children.
		if len(fact.ImportedNames) > 0 && fact.ImportedNames[0] != "*" {
			allSubmodules := true
			var firstSubmodule string
			for _, name := range fact.ImportedNames {
				subSpec := spec + "." + name
				if resolved := resolveModulePath(r.repoRoot, rootAbs, subSpec); resolved != "" {
					if firstSubmodule == "" {
						firstSubmodule = resolved
					}
				} else {
					allSubmodules = false
				}
			}

			// If ALL imported names resolve as submodules, return the first one.
			// e.g., "from webapp import settings" → webapp/settings.py
			if allSubmodules && firstSubmodule != "" {
				return model.ResolveResult{
					ResolvedPath:     firstSubmodule,
					ResolutionMethod: "package_local",
				}
			}

			// If some names DON'T resolve as submodules, they're attributes
			// defined in __init__.py. Prefer the package as primary dependency.
			// e.g., "from django.template import Context, Engine, loader"
			// where Context/Engine are in __init__.py and loader is a submodule.
			if !allSubmodules {
				if resolved := resolveModulePath(r.repoRoot, rootAbs, spec); resolved != "" {
					return model.ResolveResult{
						ResolvedPath:     resolved,
						ResolutionMethod: "package_local",
					}
				}
				// Package doesn't resolve; return first submodule match if any.
				if firstSubmodule != "" {
					return model.ResolveResult{
						ResolvedPath:     firstSubmodule,
						ResolutionMethod: "package_local",
					}
				}
			}
		}

		// No ImportedNames (or all "*"): try the specifier itself.
		if resolved := resolveModulePath(r.repoRoot, rootAbs, spec); resolved != "" {
			return model.ResolveResult{
				ResolvedPath:     resolved,
				ResolutionMethod: "package_local",
			}
		}
	}

	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "external",
		Reason:           "external",
	}
}

// resolveModulePath converts a dotted module path to a file path and checks existence.
// base is a repo-relative directory (e.g., "src" or "").
// Returns the repo-relative path if found, empty string otherwise.
func resolveModulePath(repoRoot, base, modulePath string) string {
	// Convert dots to path separators: foo.bar.baz → foo/bar/baz
	relPath := strings.ReplaceAll(modulePath, ".", string(filepath.Separator))

	// Try as .py file.
	pyFile := filepath.Join(base, relPath+".py")
	if fileExists(filepath.Join(repoRoot, pyFile)) {
		return pyFile
	}

	// Try as package __init__.py.
	initFile := filepath.Join(base, relPath, "__init__.py")
	if fileExists(filepath.Join(repoRoot, initFile)) {
		return initFile
	}

	return ""
}

// fileExists and dirExists delegate to the shared resolver package helpers.
var (
	fileExists = resolver.FileExists
	dirExists  = resolver.DirExists
)

func appendIfNewPath(roots []PackageRoot, root PackageRoot) []PackageRoot {
	for _, r := range roots {
		if r.Path == root.Path {
			return roots
		}
	}
	return append(roots, root)
}
