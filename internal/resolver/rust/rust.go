// Package rust implements import resolution for Rust source files.
package rust

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	rustpkg "github.com/kehoej/contextception/internal/extractor/rust"
	"github.com/kehoej/contextception/internal/model"
	"github.com/kehoej/contextception/internal/resolver"
)

// parseSingleLineArray parses a TOML single-line array from a key = ["a", "b"] line.
// Returns nil if the line doesn't contain a complete single-line array.
func parseSingleLineArray(line string) []string {
	openIdx := strings.Index(line, "[")
	closeIdx := strings.LastIndex(line, "]")
	if openIdx < 0 || closeIdx <= openIdx {
		return nil
	}
	inner := line[openIdx+1 : closeIdx]
	var result []string
	for _, part := range strings.Split(inner, ",") {
		val := strings.TrimSpace(part)
		val = strings.Trim(val, `"'`)
		if val != "" {
			result = append(result, val)
		}
	}
	return result
}

// Resolver resolves Rust use/mod paths to repository files.
type Resolver struct {
	repoRoot    string
	crateName   string            // from Cargo.toml [package] name
	crateRoot   string            // repo-relative directory containing Cargo.toml
	deps        map[string]bool   // external dependencies from Cargo.toml
	workMembers []workspaceMember // workspace members
}

type workspaceMember struct {
	name string          // package name
	dir  string          // repo-relative directory
	deps map[string]bool // member-level dependencies
}

// normalizeRustCrateName replaces hyphens with underscores, matching Rust's
// convention where crate names with hyphens are accessed with underscores.
func normalizeRustCrateName(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// New creates a new Rust resolver for the given repository root.
func New(repoRoot string) *Resolver {
	r := &Resolver{
		repoRoot: repoRoot,
		deps:     make(map[string]bool),
	}
	r.detectCrate()
	return r
}

// detectCrate reads Cargo.toml to find the crate name and dependencies.
func (r *Resolver) detectCrate() {
	data, err := os.ReadFile(filepath.Join(r.repoRoot, "Cargo.toml"))
	if err != nil {
		return
	}

	r.crateRoot = "."
	lines := strings.Split(string(data), "\n")
	inPackage := false
	inDeps := false
	inWorkspace := false
	inMembers := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Section headers.
		if strings.HasPrefix(trimmed, "[") {
			inPackage = trimmed == "[package]"
			inDeps = trimmed == "[dependencies]" || trimmed == "[dev-dependencies]" || trimmed == "[build-dependencies]"
			inWorkspace = trimmed == "[workspace]"
			inMembers = false
			continue
		}

		if inPackage {
			if strings.HasPrefix(trimmed, "name") {
				if eqIdx := strings.Index(trimmed, "="); eqIdx >= 0 {
					val := strings.TrimSpace(trimmed[eqIdx+1:])
					val = strings.Trim(val, `"'`)
					r.crateName = val
				}
			}
		}

		if inDeps {
			// Parse dependency names (e.g., "serde = ..." or "serde.workspace = true").
			if eqIdx := strings.Index(trimmed, "="); eqIdx > 0 {
				depName := strings.TrimSpace(trimmed[:eqIdx])
				// Remove dotted keys like "serde.workspace".
				if dotIdx := strings.Index(depName, "."); dotIdx > 0 {
					depName = depName[:dotIdx]
				}
				if depName != "" {
					r.deps[normalizeRustCrateName(depName)] = true
				}
			}
		}

		if inWorkspace {
			if strings.HasPrefix(trimmed, "members") {
				// Handle single-line array: members = ["a", "b/*"]
				if items := parseSingleLineArray(trimmed); len(items) > 0 {
					for _, member := range items {
						if strings.Contains(member, "*") {
							matches, err := filepath.Glob(filepath.Join(r.repoRoot, member))
							if err == nil {
								for _, m := range matches {
									rel, _ := filepath.Rel(r.repoRoot, m)
									rel = filepath.ToSlash(rel)
									if _, err := os.Stat(filepath.Join(m, "Cargo.toml")); err == nil {
										r.workMembers = append(r.workMembers, workspaceMember{dir: rel})
									}
								}
							}
						} else {
							r.workMembers = append(r.workMembers, workspaceMember{dir: member})
						}
					}
					continue
				}
				inMembers = true
				continue
			}
		}

		if inMembers {
			if trimmed == "]" {
				inMembers = false
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			member := strings.Trim(trimmed, `"', `)
			if member != "" && member != "[" {
				// Expand glob patterns (e.g., "crates/*").
				if strings.Contains(member, "*") {
					matches, err := filepath.Glob(filepath.Join(r.repoRoot, member))
					if err == nil {
						for _, m := range matches {
							rel, _ := filepath.Rel(r.repoRoot, m)
							rel = filepath.ToSlash(rel)
							// Only include if it contains a Cargo.toml.
							if _, err := os.Stat(filepath.Join(m, "Cargo.toml")); err == nil {
								r.workMembers = append(r.workMembers, workspaceMember{dir: rel})
							}
						}
					}
				} else {
					r.workMembers = append(r.workMembers, workspaceMember{dir: member})
				}
			}
		}
	}

	// Populate names and deps for workspace members, then discover implicit members.
	r.populateMemberInfo()
	r.discoverImplicitMembers()
	r.discoverNestedWorkspaces()
}

// populateMemberInfo reads each workspace member's Cargo.toml to fill in name and deps.
// Only processes members that haven't been populated yet (name == "").
func (r *Resolver) populateMemberInfo() {
	for i := range r.workMembers {
		if r.workMembers[i].name != "" {
			continue // already populated
		}
		memberData, err := os.ReadFile(filepath.Join(r.repoRoot, r.workMembers[i].dir, "Cargo.toml"))
		if err != nil {
			continue
		}
		r.workMembers[i].deps = make(map[string]bool)
		inPkg := false
		inMemberDeps := false
		for _, line := range strings.Split(string(memberData), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "[") {
				inPkg = t == "[package]"
				inMemberDeps = t == "[dependencies]" || t == "[dev-dependencies]" || t == "[build-dependencies]"
				continue
			}
			if inPkg && strings.HasPrefix(t, "name") {
				if eqIdx := strings.Index(t, "="); eqIdx >= 0 {
					val := strings.TrimSpace(t[eqIdx+1:])
					val = strings.Trim(val, `"'`)
					r.workMembers[i].name = val
				}
			}
			if inMemberDeps {
				if eqIdx := strings.Index(t, "="); eqIdx > 0 {
					depName := strings.TrimSpace(t[:eqIdx])
					if dotIdx := strings.Index(depName, "."); dotIdx > 0 {
						depName = depName[:dotIdx]
					}
					if depName != "" {
						r.workMembers[i].deps[normalizeRustCrateName(depName)] = true
					}
				}
			}
		}
	}
}

// parsePackageName reads a Cargo.toml file and returns the [package] name value,
// or "" if the file doesn't exist or has no package name.
func parsePackageName(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	inPkg := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			inPkg = t == "[package]"
			continue
		}
		if inPkg && strings.HasPrefix(t, "name") {
			if eqIdx := strings.Index(t, "="); eqIdx >= 0 {
				val := strings.TrimSpace(t[eqIdx+1:])
				val = strings.Trim(val, `"'`)
				return val
			}
		}
	}
	return ""
}

// discoverImplicitMembers scans parent directories of existing workspace members
// for sibling directories that contain a Cargo.toml with a [package] section.
// This discovers crates that aren't explicitly listed as workspace members but
// exist alongside listed members (e.g., compiler/rustc_abi next to compiler/rustc).
func (r *Resolver) discoverImplicitMembers() {
	if len(r.workMembers) == 0 {
		return
	}
	// Collect parent directories of existing members.
	parentDirs := make(map[string]bool)
	for _, m := range r.workMembers {
		parent := path.Dir(m.dir)
		if parent != "" && parent != "." {
			parentDirs[parent] = true
		}
	}
	existing := make(map[string]bool, len(r.workMembers))
	for _, m := range r.workMembers {
		existing[m.dir] = true
	}
	// Scan each parent for sibling crates.
	for parentDir := range parentDirs {
		entries, err := os.ReadDir(filepath.Join(r.repoRoot, parentDir))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidateDir := filepath.ToSlash(filepath.Join(parentDir, entry.Name()))
			if existing[candidateDir] {
				continue
			}
			cargoPath := filepath.Join(r.repoRoot, candidateDir, "Cargo.toml")
			if name := parsePackageName(cargoPath); name != "" {
				r.workMembers = append(r.workMembers, workspaceMember{dir: candidateDir})
				existing[candidateDir] = true
			}
		}
	}
	// Populate info for newly discovered members.
	r.populateMemberInfo()
}

// cargoTomlInfo holds parsed information from a Cargo.toml file.
type cargoTomlInfo struct {
	packageName string   // [package] name value
	isWorkspace bool     // has [workspace] section
	members     []string // workspace members list (raw, may contain globs)
}

// parseCargoTomlInfo parses a Cargo.toml and returns both package and workspace info.
func parseCargoTomlInfo(filePath string) cargoTomlInfo {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return cargoTomlInfo{}
	}
	var info cargoTomlInfo
	inPkg := false
	inWorkspace := false
	inMembers := false
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") {
			inPkg = t == "[package]"
			inWorkspace = t == "[workspace]"
			inMembers = false
			continue
		}
		if inPkg && strings.HasPrefix(t, "name") {
			if eqIdx := strings.Index(t, "="); eqIdx >= 0 {
				val := strings.TrimSpace(t[eqIdx+1:])
				val = strings.Trim(val, `"'`)
				info.packageName = val
			}
		}
		if inWorkspace {
			info.isWorkspace = true
			if strings.HasPrefix(t, "members") {
				// Handle single-line array: members = ["a", "b/*"]
				if items := parseSingleLineArray(t); len(items) > 0 {
					info.members = append(info.members, items...)
					continue
				}
				inMembers = true
				continue
			}
		}
		if inMembers {
			if t == "]" {
				inMembers = false
				continue
			}
			if strings.HasPrefix(t, "#") {
				continue
			}
			member := strings.Trim(t, `"', `)
			if member != "" && member != "[" {
				info.members = append(info.members, member)
			}
		}
	}
	return info
}

// expandAndAddMembers expands a workspace member pattern (possibly a glob) relative
// to baseDir and adds discovered crates to r.workMembers.
func (r *Resolver) expandAndAddMembers(baseDir, memberPattern string, existing map[string]bool) {
	fullPattern := filepath.Join(baseDir, memberPattern)
	if strings.Contains(fullPattern, "*") {
		matches, err := filepath.Glob(filepath.Join(r.repoRoot, fullPattern))
		if err != nil {
			return
		}
		for _, m := range matches {
			rel, _ := filepath.Rel(r.repoRoot, m)
			rel = filepath.ToSlash(rel)
			if existing[rel] {
				continue
			}
			if _, err := os.Stat(filepath.Join(m, "Cargo.toml")); err == nil {
				r.workMembers = append(r.workMembers, workspaceMember{dir: rel})
				existing[rel] = true
			}
		}
	} else {
		dir := filepath.ToSlash(filepath.Join(baseDir, memberPattern))
		if existing[dir] {
			return
		}
		cargoPath := filepath.Join(r.repoRoot, dir, "Cargo.toml")
		if _, err := os.Stat(cargoPath); err == nil {
			r.workMembers = append(r.workMembers, workspaceMember{dir: dir})
			existing[dir] = true
		}
	}
}

// discoverNestedWorkspaces scans for subdirectories that contain their own
// workspace Cargo.toml files (e.g., src/tools/rust-analyzer/Cargo.toml with
// its own [workspace] and members list). Expands their members and adds them.
func (r *Resolver) discoverNestedWorkspaces() {
	if len(r.workMembers) == 0 {
		return
	}
	// Collect parent dirs of existing members as scan candidates.
	// We look at siblings of parent dirs to find nested workspace roots.
	candidates := make(map[string]bool)
	for _, m := range r.workMembers {
		parent := path.Dir(m.dir)
		if parent != "" && parent != "." {
			candidates[parent] = true
		}
	}
	existing := make(map[string]bool, len(r.workMembers))
	for _, m := range r.workMembers {
		existing[m.dir] = true
	}
	for parentDir := range candidates {
		entries, err := os.ReadDir(filepath.Join(r.repoRoot, parentDir))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.ToSlash(filepath.Join(parentDir, entry.Name()))
			cargoPath := filepath.Join(r.repoRoot, dir, "Cargo.toml")
			info := parseCargoTomlInfo(cargoPath)
			if !info.isWorkspace || len(info.members) == 0 {
				continue
			}
			// Expand nested workspace members relative to their base directory.
			for _, member := range info.members {
				r.expandAndAddMembers(dir, member, existing)
			}
		}
	}
	// Populate info for newly discovered members.
	r.populateMemberInfo()
}

// Resolve maps a Rust use/mod path to a repository file.
func (r *Resolver) Resolve(srcFile string, fact model.ImportFact, repoRoot string) (model.ResolveResult, error) {
	spec := fact.Specifier

	// Module declarations: mod foo; → look for foo.rs or foo/mod.rs.
	if strings.HasPrefix(spec, "mod:") {
		modName := strings.TrimPrefix(spec, "mod:")
		return r.resolveModDecl(srcFile, modName)
	}

	// Extern crate declarations.
	if strings.HasPrefix(spec, "extern:") {
		crateName := strings.TrimPrefix(spec, "extern:")
		if r.deps[crateName] {
			return model.ResolveResult{
				External:         true,
				ResolutionMethod: "external",
				Reason:           "dependency",
			}, nil
		}
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "extern_crate",
		}, nil
	}

	// Stdlib imports.
	if rustpkg.IsStdlib(spec) {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "stdlib",
		}, nil
	}

	// Strip grouped imports for resolution (use base path).
	resolveSpec := spec
	if braceIdx := strings.Index(resolveSpec, "::{"); braceIdx >= 0 {
		resolveSpec = resolveSpec[:braceIdx]
	}

	// crate:: relative paths — resolve relative to the crate the source file belongs to.
	// Also handles bare "crate" (from stripped grouped imports like crate::{A, B}).
	if resolveSpec == "crate" || strings.HasPrefix(resolveSpec, "crate::") {
		relPath := strings.TrimPrefix(resolveSpec, "crate")
		relPath = strings.TrimPrefix(relPath, "::")
		return r.resolveCratePath(srcFile, relPath)
	}

	// super:: relative paths.
	if resolveSpec == "super" || strings.HasPrefix(resolveSpec, "super::") {
		return r.resolveSuperPath(srcFile, resolveSpec)
	}

	// self:: relative paths.
	// For named files (foo.rs): self::bar resolves in foo/ subdirectory.
	// For mod.rs/lib.rs/main.rs: self::bar resolves in the containing directory.
	if resolveSpec == "self" || strings.HasPrefix(resolveSpec, "self::") {
		relPath := strings.TrimPrefix(resolveSpec, "self")
		relPath = strings.TrimPrefix(relPath, "::")
		srcBase := path.Base(srcFile)
		srcDir := path.Dir(srcFile)
		if srcBase != "mod.rs" && srcBase != "lib.rs" && srcBase != "main.rs" {
			// Named file: self refers to the module namespace, which is a subdirectory.
			stem := strings.TrimSuffix(srcBase, ".rs")
			srcDir = path.Join(srcDir, stem)
		}
		if relPath == "" {
			// Bare "self" — resolve to current module file.
			return model.ResolveResult{
				ResolvedPath:     srcFile,
				ResolutionMethod: "self_module",
			}, nil
		}
		result, err := r.resolveModulePath(srcDir, relPath)
		if err != nil {
			return result, err
		}
		// Single-segment fallback: self::TypeName where TypeName is defined
		// in the current module file, not a child module.
		if result.External && !strings.Contains(relPath, "::") {
			return model.ResolveResult{
				ResolvedPath:     srcFile,
				ResolutionMethod: "self_module",
			}, nil
		}
		return result, nil
	}

	// External crate (starts with a crate name).
	firstPart := resolveSpec
	if colonIdx := strings.Index(resolveSpec, "::"); colonIdx >= 0 {
		firstPart = resolveSpec[:colonIdx]
	}

	// Check if it's a workspace member crate (check before deps, since workspace
	// members may also appear in [dependencies] as path dependencies).
	normalizedFirst := normalizeRustCrateName(firstPart)
	for _, m := range r.workMembers {
		memberName := normalizeRustCrateName(m.name)
		if memberName == "" {
			// Fallback: check directory basename.
			memberName = normalizeRustCrateName(path.Base(m.dir))
		}
		if memberName == normalizedFirst {
			// If specifier is just the crate name, resolve to its entry point.
			if resolveSpec == firstPart {
				srcDir := filepath.Join(m.dir, "src")
				libRs := filepath.ToSlash(filepath.Join(srcDir, "lib.rs"))
				if r.fileExists(libRs) {
					return model.ResolveResult{
						ResolvedPath:     libRs,
						ResolutionMethod: "workspace_member",
					}, nil
				}
				mainRs := filepath.ToSlash(filepath.Join(srcDir, "main.rs"))
				if r.fileExists(mainRs) {
					return model.ResolveResult{
						ResolvedPath:     mainRs,
						ResolutionMethod: "workspace_member",
					}, nil
				}
			}
			relPath := strings.TrimPrefix(resolveSpec, firstPart+"::")
			result, err := r.resolveInDir(filepath.Join(m.dir, "src"), relPath)
			if err != nil {
				return result, err
			}
			// Fallback: rustc_span::Span or rustc_hir::def_id::DefId where the
			// path doesn't resolve to a module file. Fall back to the member's
			// entry point so we create an edge instead of not_found.
			if result.External {
				srcDir := filepath.Join(m.dir, "src")
				libRs := filepath.ToSlash(filepath.Join(srcDir, "lib.rs"))
				if r.fileExists(libRs) {
					return model.ResolveResult{
						ResolvedPath:     libRs,
						ResolutionMethod: "workspace_member",
					}, nil
				}
				mainRs := filepath.ToSlash(filepath.Join(srcDir, "main.rs"))
				if r.fileExists(mainRs) {
					return model.ResolveResult{
						ResolvedPath:     mainRs,
						ResolutionMethod: "workspace_member",
					}, nil
				}
			}
			return result, nil
		}
	}

	// Check if it's a known dependency (workspace-level or member-level).
	// This is checked AFTER workspace members so local path deps don't shadow them.
	if r.deps[normalizedFirst] || r.isMemberDep(srcFile, normalizedFirst) {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "external",
			Reason:           "dependency",
		}, nil
	}

	// Before marking as third_party, check if this could be a workspace member
	// that we failed to match (e.g., due to naming mismatch). If the workspace
	// has members and the import's first segment matches a member directory basename,
	// mark as not_found instead of third_party so confidence scoring counts it.
	if len(r.workMembers) > 0 {
		for _, m := range r.workMembers {
			dirBase := normalizeRustCrateName(path.Base(m.dir))
			if dirBase == normalizedFirst {
				return model.ResolveResult{
					External:         true,
					ResolutionMethod: "external",
					Reason:           "not_found",
				}, nil
			}
		}
	}

	// Unknown external.
	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "external",
		Reason:           "third_party",
	}, nil
}

// resolveModDecl resolves a `mod foo;` declaration to foo.rs or foo/mod.rs.
func (r *Resolver) resolveModDecl(srcFile, modName string) (model.ResolveResult, error) {
	srcDir := path.Dir(srcFile)
	srcBase := path.Base(srcFile)

	// If source is lib.rs or main.rs, look in the same directory.
	// If source is foo/mod.rs, look in the same directory.
	// If source is foo.rs, look in foo/ directory.
	var searchDir string
	if srcBase == "lib.rs" || srcBase == "main.rs" || srcBase == "mod.rs" {
		searchDir = srcDir
	} else {
		// foo.rs declaring mod bar → look in foo/bar.rs
		stem := strings.TrimSuffix(srcBase, ".rs")
		searchDir = path.Join(srcDir, stem)
	}

	// Try modName.rs first.
	candidate := path.Join(searchDir, modName+".rs")
	candidate = filepath.ToSlash(candidate)
	if r.fileExists(candidate) {
		return model.ResolveResult{
			ResolvedPath:     candidate,
			ResolutionMethod: "mod_declaration",
		}, nil
	}

	// Try modName/mod.rs.
	candidate = path.Join(searchDir, modName, "mod.rs")
	candidate = filepath.ToSlash(candidate)
	if r.fileExists(candidate) {
		return model.ResolveResult{
			ResolvedPath:     candidate,
			ResolutionMethod: "mod_declaration",
		}, nil
	}

	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "mod_declaration",
		Reason:           "not_found",
	}, nil
}

// resolveCratePath resolves a crate:: relative path.
// In a workspace, it determines which member crate the source file belongs to.
// When relPath is empty (bare "crate"), resolves to the crate entry point.
func (r *Resolver) resolveCratePath(srcFile, relPath string) (model.ResolveResult, error) {
	// In a workspace, find which member crate the source file belongs to.
	if len(r.workMembers) > 0 {
		srcNorm := filepath.ToSlash(srcFile)
		for _, m := range r.workMembers {
			prefix := filepath.ToSlash(m.dir) + "/"
			if strings.HasPrefix(srcNorm, prefix) {
				srcDir := filepath.Join(m.dir, "src")
				if relPath == "" {
					return r.resolveCrateRoot(srcDir)
				}
				result, err := r.resolveModulePath(srcDir, relPath)
				if err != nil {
					return result, err
				}
				// Fallback: crate::TypeName or crate::a::b::C where the path doesn't
				// resolve to a module file — fall back to crate root (lib.rs/main.rs).
				if result.External {
					return r.resolveCrateRoot(srcDir)
				}
				return result, nil
			}
		}
	}

	// Non-workspace or file not in any member: use crateRoot.
	var srcDir string
	if r.crateRoot == "." {
		srcDir = "src"
	} else {
		srcDir = filepath.Join(r.crateRoot, "src")
	}
	if relPath == "" {
		return r.resolveCrateRoot(srcDir)
	}
	result, err := r.resolveModulePath(srcDir, relPath)
	if err != nil {
		return result, err
	}
	// Fallback for non-workspace crates: any unresolvable crate:: path.
	if result.External {
		return r.resolveCrateRoot(srcDir)
	}
	return result, nil
}

// resolveCrateRoot resolves to the crate entry point (lib.rs or main.rs) in the given src dir.
func (r *Resolver) resolveCrateRoot(srcDir string) (model.ResolveResult, error) {
	libRs := filepath.ToSlash(filepath.Join(srcDir, "lib.rs"))
	if r.fileExists(libRs) {
		return model.ResolveResult{
			ResolvedPath:     libRs,
			ResolutionMethod: "crate_root",
		}, nil
	}
	mainRs := filepath.ToSlash(filepath.Join(srcDir, "main.rs"))
	if r.fileExists(mainRs) {
		return model.ResolveResult{
			ResolvedPath:     mainRs,
			ResolutionMethod: "crate_root",
		}, nil
	}
	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "crate_root",
		Reason:           "not_found",
	}, nil
}

// resolveSuperPath resolves super:: paths by walking up the module tree.
// For mod.rs/lib.rs/main.rs: super:: walks up one directory (standard behavior).
// For named files (foo.rs): the first super:: stays in the containing directory
// because foo.rs is a child module — super means "parent module" = same dir.
// Also handles bare "super" (from stripped grouped imports).
func (r *Resolver) resolveSuperPath(srcFile, spec string) (model.ResolveResult, error) {
	dir := path.Dir(srcFile)
	srcBase := path.Base(srcFile)

	// Determine if source is a named file (not mod.rs/lib.rs/main.rs).
	isNamedFile := srcBase != "mod.rs" && srcBase != "lib.rs" && srcBase != "main.rs"

	// Handle bare "super" keyword.
	if spec == "super" {
		if !isNamedFile {
			dir = path.Dir(dir)
		}
		// Resolve to parent module entry point.
		return r.resolveModuleEntry(dir)
	}

	// Count and strip super:: prefixes.
	remaining := spec
	firstSuper := true
	for strings.HasPrefix(remaining, "super::") {
		remaining = strings.TrimPrefix(remaining, "super::")
		if firstSuper && isNamedFile {
			// For named files, first super:: stays in containing directory
			// (foo.rs is child module, super = parent = same dir).
			firstSuper = false
			continue
		}
		firstSuper = false
		// Walk up one directory.
		dir = path.Dir(dir)
	}

	if remaining == "" {
		return model.ResolveResult{
			External:         true,
			ResolutionMethod: "super_path",
			Reason:           "empty_path",
		}, nil
	}

	result, err := r.resolveModulePath(dir, remaining)
	if err != nil {
		return result, err
	}
	// Single-segment fallback: super::TypeName where TypeName is a type
	// defined in the parent module's entry file, not a child module.
	if result.External && !strings.Contains(remaining, "::") {
		return r.resolveModuleEntry(dir)
	}
	return result, nil
}

// resolveModuleEntry resolves to the module entry point (mod.rs, lib.rs, or main.rs) in a directory.
// Also handles Rust 2018 edition modules where foo/ maps to foo.rs in the parent directory.
func (r *Resolver) resolveModuleEntry(dir string) (model.ResolveResult, error) {
	for _, name := range []string{"mod.rs", "lib.rs", "main.rs"} {
		candidate := filepath.ToSlash(filepath.Join(dir, name))
		if r.fileExists(candidate) {
			return model.ResolveResult{
				ResolvedPath:     candidate,
				ResolutionMethod: "module_entry",
			}, nil
		}
	}
	// Rust 2018 edition fallback: directory foo/ may correspond to foo.rs in the parent.
	dirBase := path.Base(dir)
	parentDir := path.Dir(dir)
	parentFile := filepath.ToSlash(filepath.Join(parentDir, dirBase+".rs"))
	if r.fileExists(parentFile) {
		return model.ResolveResult{
			ResolvedPath:     parentFile,
			ResolutionMethod: "module_entry",
		}, nil
	}
	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "module_entry",
		Reason:           "not_found",
	}, nil
}

// resolveModulePath tries to find a Rust module file from a directory + module path.
func (r *Resolver) resolveModulePath(dir, modPath string) (model.ResolveResult, error) {
	parts := strings.Split(modPath, "::")

	// Build file path from module path parts.
	fileParts := make([]string, len(parts))
	copy(fileParts, parts)

	// Try direct file: dir/a/b/c.rs
	filePath := path.Join(dir, strings.Join(fileParts, "/")) + ".rs"
	filePath = filepath.ToSlash(filePath)
	if r.fileExists(filePath) {
		return model.ResolveResult{
			ResolvedPath:     filePath,
			ResolutionMethod: "module_path",
		}, nil
	}

	// Try mod.rs: dir/a/b/c/mod.rs
	modFilePath := path.Join(dir, strings.Join(fileParts, "/"), "mod.rs")
	modFilePath = filepath.ToSlash(modFilePath)
	if r.fileExists(modFilePath) {
		return model.ResolveResult{
			ResolvedPath:     modFilePath,
			ResolutionMethod: "module_path",
		}, nil
	}

	// Progressive partial fallback: try deepest match first, then work up.
	// For render::mesh::vertex, try: render/mesh.rs, render/mesh/mod.rs,
	// then render.rs, render/mod.rs.
	if len(parts) > 1 {
		for depth := len(parts) - 1; depth >= 1; depth-- {
			partialPath := path.Join(dir, strings.Join(parts[:depth], "/")) + ".rs"
			partialPath = filepath.ToSlash(partialPath)
			if r.fileExists(partialPath) {
				return model.ResolveResult{
					ResolvedPath:     partialPath,
					ResolutionMethod: "module_path_partial",
				}, nil
			}
			partialModPath := path.Join(dir, strings.Join(parts[:depth], "/"), "mod.rs")
			partialModPath = filepath.ToSlash(partialModPath)
			if r.fileExists(partialModPath) {
				return model.ResolveResult{
					ResolvedPath:     partialModPath,
					ResolutionMethod: "module_path_partial",
				}, nil
			}
		}
	}

	return model.ResolveResult{
		External:         true,
		ResolutionMethod: "module_path",
		Reason:           "not_found",
	}, nil
}

// resolveInDir resolves a module path within a specific directory.
func (r *Resolver) resolveInDir(dir, modPath string) (model.ResolveResult, error) {
	return r.resolveModulePath(dir, modPath)
}

// isMemberDep checks if a dependency name is declared in the member crate
// that the source file belongs to.
func (r *Resolver) isMemberDep(srcFile, depName string) bool {
	srcNorm := filepath.ToSlash(srcFile)
	for _, m := range r.workMembers {
		prefix := filepath.ToSlash(m.dir) + "/"
		if strings.HasPrefix(srcNorm, prefix) {
			return m.deps[depName]
		}
	}
	return false
}

// fileExists checks if a repo-relative path exists as a file on disk.
func (r *Resolver) fileExists(repoRelPath string) bool {
	return resolver.FileExists(filepath.Join(r.repoRoot, repoRelPath))
}
