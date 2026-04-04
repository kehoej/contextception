package typescript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// workspacePackage represents a single package within a monorepo workspace.
type workspacePackage struct {
	Name    string            // from package.json "name"
	Root    string            // repo-relative dir (e.g. "packages/utils")
	Exports map[string]string // simplified: subpath → relative target
	Types   string            // "types" field
	Module  string            // "module" field
	Main    string            // "main" field
}

// workspaceConfig holds the detected workspace layout.
type workspaceConfig struct {
	root     string                       // abs repo root
	packages map[string]*workspacePackage // name → package
}

// detectWorkspaces checks the repo root for workspace manifests and discovers packages.
// Returns nil if the repo is not a monorepo.
func detectWorkspaces(repoRoot string) *workspaceConfig {
	// 1. pnpm-workspace.yaml
	if globs := parsePnpmWorkspace(filepath.Join(repoRoot, "pnpm-workspace.yaml")); len(globs) > 0 {
		pkgs := discoverPackages(repoRoot, globs)
		if len(pkgs) > 0 {
			return &workspaceConfig{root: repoRoot, packages: pkgs}
		}
	}

	// 2. root package.json with "workspaces"
	if globs := parsePackageJSONWorkspaces(filepath.Join(repoRoot, "package.json")); len(globs) > 0 {
		pkgs := discoverPackages(repoRoot, globs)
		if len(pkgs) > 0 {
			return &workspaceConfig{root: repoRoot, packages: pkgs}
		}
	}

	return nil
}

// pnpmWorkspace represents the structure of pnpm-workspace.yaml.
type pnpmWorkspace struct {
	Packages []string `yaml:"packages"`
}

// parsePnpmWorkspace extracts package glob patterns from pnpm-workspace.yaml.
func parsePnpmWorkspace(absPath string) []string {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	var ws pnpmWorkspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return nil
	}

	return ws.Packages
}

// parsePackageJSONWorkspaces extracts workspace glob patterns from root package.json.
// Handles both array form ["packages/*"] and object form {"packages": ["packages/*"]}.
func parsePackageJSONWorkspaces(absPath string) []string {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	var raw struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Workspaces == nil {
		return nil
	}

	// Try array form first: ["packages/*", "apps/*"]
	var arr []string
	if err := json.Unmarshal(raw.Workspaces, &arr); err == nil {
		return arr
	}

	// Try object form: {"packages": ["packages/*"]}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(raw.Workspaces, &obj); err == nil {
		return obj.Packages
	}

	return nil
}

// discoverPackages expands glob patterns and parses each matching package.json.
func discoverPackages(repoRoot string, globs []string) map[string]*workspacePackage {
	packages := make(map[string]*workspacePackage)

	for _, glob := range globs {
		pattern := filepath.Join(repoRoot, filepath.FromSlash(glob))
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || !info.IsDir() {
				continue
			}

			pkgJSONPath := filepath.Join(match, "package.json")
			pkg := parseWorkspacePackageJSON(pkgJSONPath)
			if pkg == nil {
				continue
			}

			rel, err := filepath.Rel(repoRoot, match)
			if err != nil {
				continue
			}
			pkg.Root = filepath.ToSlash(rel)
			packages[pkg.Name] = pkg
		}
	}

	return packages
}

// parseWorkspacePackageJSON reads a package.json and extracts resolver-relevant fields.
func parseWorkspacePackageJSON(absPath string) *workspacePackage {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	var raw struct {
		Name    string          `json:"name"`
		Types   string          `json:"types"`
		Module  string          `json:"module"`
		Main    string          `json:"main"`
		Exports json.RawMessage `json:"exports"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Name == "" {
		return nil
	}

	pkg := &workspacePackage{
		Name:   raw.Name,
		Types:  raw.Types,
		Module: raw.Module,
		Main:   raw.Main,
	}

	if raw.Exports != nil {
		pkg.Exports = parseExportsField(raw.Exports)
	}

	return pkg
}

// parseExportsField parses the package.json "exports" field into a subpath → target map.
func parseExportsField(raw json.RawMessage) map[string]string {
	result := make(map[string]string)

	// Try string form: "./dist/index.js"
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		result["."] = str
		return result
	}

	// Try object form: {".": "...", "./utils": "..."}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}

	for subpath, val := range obj {
		// Each value can be a string or a conditions object.
		var target string
		if err := json.Unmarshal(val, &target); err == nil {
			result[subpath] = target
			continue
		}

		// Conditions object: {"types": "...", "import": "...", "require": "...", "default": "..."}
		var conditions map[string]json.RawMessage
		if err := json.Unmarshal(val, &conditions); err == nil {
			target = resolveCondition(conditions)
			if target != "" {
				result[subpath] = target
			}
		}
	}

	return result
}

// resolveCondition picks the best target from a conditions object.
// Priority: types > import > require > default.
func resolveCondition(conditions map[string]json.RawMessage) string {
	for _, key := range []string{"types", "import", "require", "default"} {
		val, ok := conditions[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(val, &s); err == nil {
			return s
		}
		// Nested conditions (e.g. {"types": {"import": "..."}}) — try one level deeper.
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(val, &nested); err == nil {
			if s := resolveCondition(nested); s != "" {
				return s
			}
		}
	}
	return ""
}

// matchPackage splits a bare specifier into a workspace package name and subpath.
// Returns (nil, "") if the specifier doesn't match any workspace package.
func matchPackage(packages map[string]*workspacePackage, spec string) (*workspacePackage, string) {
	var name, subpath string

	if strings.HasPrefix(spec, "@") {
		// Scoped package: @scope/pkg or @scope/pkg/subpath
		parts := strings.SplitN(spec, "/", 3)
		if len(parts) < 2 {
			return nil, ""
		}
		name = parts[0] + "/" + parts[1]
		if len(parts) == 3 {
			subpath = "./" + parts[2]
		} else {
			subpath = "."
		}
	} else {
		// Unscoped: pkg or pkg/subpath
		parts := strings.SplitN(spec, "/", 2)
		name = parts[0]
		if len(parts) == 2 {
			subpath = "./" + parts[1]
		} else {
			subpath = "."
		}
	}

	pkg, ok := packages[name]
	if !ok {
		return nil, ""
	}
	return pkg, subpath
}

// resolvePackageEntry finds the entry file for a workspace package and subpath.
// Returns repo-relative path or "" if nothing found.
func resolvePackageEntry(r *Resolver, pkg *workspacePackage, subpath string) string {
	// 1. Exports map.
	if target, ok := pkg.Exports[subpath]; ok {
		if resolved := tryEntryTarget(r, pkg, target); resolved != "" {
			return resolved
		}
	}

	// Fields 2-4 only apply for the root subpath ".".
	if subpath == "." {
		// 2. Types field.
		if pkg.Types != "" {
			if resolved := tryEntryTarget(r, pkg, pkg.Types); resolved != "" {
				return resolved
			}
		}

		// 3. Module field.
		if pkg.Module != "" {
			if resolved := tryEntryTarget(r, pkg, pkg.Module); resolved != "" {
				return resolved
			}
		}

		// 4. Main field.
		if pkg.Main != "" {
			if resolved := tryEntryTarget(r, pkg, pkg.Main); resolved != "" {
				return resolved
			}
		}
	}

	// 5. Conventional fallback (root subpath only).
	if subpath == "." {
		for _, conv := range []string{"src/index.ts", "src/index.tsx", "index.ts", "index.tsx"} {
			candidate := filepath.ToSlash(filepath.Join(pkg.Root, conv))
			if r.fileExists(candidate) {
				return candidate
			}
		}
	}

	// 6. Subpath file fallback (non-root subpaths without exports).
	if subpath != "." {
		relPath := strings.TrimPrefix(subpath, "./")

		// Source-first: try src/<relPath>
		srcCandidate := filepath.ToSlash(filepath.Join(pkg.Root, "src", relPath))
		if resolved := r.resolveFileCandidate(srcCandidate); resolved != "" {
			return resolved
		}

		// Direct: try <relPath> under package root
		candidate := filepath.ToSlash(filepath.Join(pkg.Root, relPath))
		if resolved := r.resolveFileCandidate(candidate); resolved != "" {
			return resolved
		}
	}

	return ""
}

// tryEntryTarget attempts to resolve a package.json entry target to a source file.
// First tries source-first mapping, then falls back to direct resolution.
func tryEntryTarget(r *Resolver, pkg *workspacePackage, target string) string {
	// Try source-first mapping (dist/* → src/*).
	if mapped := mapToSource(target); mapped != "" {
		candidate := filepath.ToSlash(filepath.Join(pkg.Root, mapped))
		if resolved := r.resolveFileCandidate(candidate); resolved != "" {
			return resolved
		}
	}

	// Direct resolution of the target.
	clean := strings.TrimPrefix(target, "./")
	candidate := filepath.ToSlash(filepath.Join(pkg.Root, clean))
	if resolved := r.resolveFileCandidate(candidate); resolved != "" {
		return resolved
	}

	return ""
}

// mapToSource applies the source-first mapping rule from ADR-0003 5.3.
// Maps dist/* or build/* targets to src/* with JS→TS extension swap.
func mapToSource(target string) string {
	clean := strings.TrimPrefix(target, "./")

	var rest string
	if strings.HasPrefix(clean, "dist/") {
		rest = strings.TrimPrefix(clean, "dist/")
	} else if strings.HasPrefix(clean, "build/") {
		rest = strings.TrimPrefix(clean, "build/")
	} else {
		return ""
	}

	// Swap JS/declaration extensions to TS source equivalents.
	// Handle .d.ts/.d.mts/.d.cts first since filepath.Ext only returns
	// the last dot-segment (e.g., ".ts" for "foo.d.ts").
	switch {
	case strings.HasSuffix(rest, ".d.ts"):
		rest = strings.TrimSuffix(rest, ".d.ts") + ".ts"
	case strings.HasSuffix(rest, ".d.mts"):
		rest = strings.TrimSuffix(rest, ".d.mts") + ".mts"
	case strings.HasSuffix(rest, ".d.cts"):
		rest = strings.TrimSuffix(rest, ".d.cts") + ".cts"
	default:
		ext := filepath.Ext(rest)
		stem := strings.TrimSuffix(rest, ext)
		switch ext {
		case ".js":
			rest = stem + ".ts"
		case ".jsx":
			rest = stem + ".tsx"
		case ".mjs":
			rest = stem + ".mts"
		case ".cjs":
			rest = stem + ".cts"
		}
	}

	return "src/" + rest
}
