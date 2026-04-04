package rust

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func setupCrate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create Cargo.toml.
	cargoToml := `[package]
name = "myapp"
version = "0.1.0"

[dependencies]
serde = "1.0"
tokio = { version = "1", features = ["full"] }
`
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(cargoToml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create source files.
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte("fn main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "config.rs"), []byte("pub struct Config {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "models", "mod.rs"), []byte("pub mod user;"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "models", "user.rs"), []byte("pub struct User {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestResolveStdlib(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "std::collections::HashMap",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external")
	}
	if result.Reason != "stdlib" {
		t.Errorf("reason = %q, want %q", result.Reason, "stdlib")
	}
}

func TestResolveCratePath(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::config",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/config.rs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveCrateDeepPath(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::models::user",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/models/user.rs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveCrateModDir(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::models",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveModDeclaration(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// mod config; in main.rs should find src/config.rs
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier:  "mod:config",
		ImportType: "module",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/config.rs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveModDeclModDir(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// mod models; in main.rs should find src/models/mod.rs
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier:  "mod:models",
		ImportType: "module",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveDependency(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "serde::Deserialize",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external")
	}
	if result.Reason != "dependency" {
		t.Errorf("reason = %q, want %q", result.Reason, "dependency")
	}
}

func TestResolveSuperPath(t *testing.T) {
	dir := setupCrate(t)

	// Create a config.rs in models/ for the named-file super:: test.
	if err := os.WriteFile(filepath.Join(dir, "src", "models", "config.rs"), []byte("pub struct Config {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// super::config from src/models/user.rs (named file) → src/models/config.rs
	// user.rs is crate::models::user. super:: = parent module = crate::models = src/models/
	result, err := r.Resolve("src/models/user.rs", model.ImportFact{
		Specifier: "super::config",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/models/config.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/models/config.rs")
	}
}

func TestResolveSuperPathFromModRs(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// super::config from src/models/mod.rs → src/config.rs
	// mod.rs IS the module root for crate::models. super:: = crate = src/
	result, err := r.Resolve("src/models/mod.rs", model.ImportFact{
		Specifier: "super::config",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/config.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/config.rs")
	}
}

func TestResolveSuperPathFromNamedFile(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// super::models from src/config.rs (named file at src/ level)
	// config.rs is crate::config. super:: = crate = src/
	// super::models → src/models → src/models/mod.rs
	result, err := r.Resolve("src/config.rs", model.ImportFact{
		Specifier: "super::models",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/models/mod.rs")
	}
}

func TestResolveSelfFromNamedFile(t *testing.T) {
	dir := setupCrate(t)

	// Create bar.rs inside a config/ subdirectory.
	configDir := filepath.Join(dir, "src", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "bar.rs"), []byte("pub fn bar() {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// self::bar from src/config.rs → src/config/bar.rs
	// config.rs is the module crate::config. self:: refers to this module.
	// Its children live in config/ subdirectory.
	result, err := r.Resolve("src/config.rs", model.ImportFact{
		Specifier: "self::bar",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/config/bar.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/config/bar.rs")
	}
}

func TestResolveExternCrate(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "extern:serde",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external")
	}
	if result.Reason != "dependency" {
		t.Errorf("reason = %q, want %q", result.Reason, "dependency")
	}
}

func TestResolveThirdParty(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "unknown_crate::Thing",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external")
	}
	if result.Reason != "third_party" {
		t.Errorf("reason = %q, want %q", result.Reason, "third_party")
	}
}

func TestResolveGroupedUse(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// Grouped use: crate::models::{User, Config} → resolve base path crate::models
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::models::{User, Config}",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

// setupWorkspace creates a workspace with explicit members (like Tokio).
func setupWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Root Cargo.toml (workspace).
	rootToml := `[workspace]
resolver = "2"
members = [
  "mycrate",
  "myutil",
]
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	// mycrate member.
	os.MkdirAll(filepath.Join(dir, "mycrate", "src", "runtime"), 0o755)
	os.WriteFile(filepath.Join(dir, "mycrate", "Cargo.toml"), []byte(`[package]
name = "mycrate"
version = "0.1.0"

[dependencies]
serde = "1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "mycrate", "src", "lib.rs"), []byte("pub mod runtime;"), 0o644)
	os.WriteFile(filepath.Join(dir, "mycrate", "src", "runtime", "mod.rs"), []byte("pub mod scheduler;"), 0o644)
	os.WriteFile(filepath.Join(dir, "mycrate", "src", "runtime", "scheduler.rs"), []byte("pub fn run() {}"), 0o644)

	// myutil member.
	os.MkdirAll(filepath.Join(dir, "myutil", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "myutil", "Cargo.toml"), []byte(`[package]
name = "myutil"
version = "0.1.0"

[dependencies]
log = "0.4"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "myutil", "src", "lib.rs"), []byte("pub fn helper() {}"), 0o644)
	os.WriteFile(filepath.Join(dir, "myutil", "src", "format.rs"), []byte("pub fn fmt() {}"), 0o644)

	return dir
}

func TestWorkspaceCrateResolution(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// crate::runtime from mycrate/src/lib.rs → mycrate/src/runtime/mod.rs
	result, err := r.Resolve("mycrate/src/lib.rs", model.ImportFact{
		Specifier: "crate::runtime::scheduler",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution, got external")
	}
	if result.ResolvedPath != "mycrate/src/runtime/scheduler.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "mycrate/src/runtime/scheduler.rs")
	}
}

func TestWorkspaceCrateModule(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// crate::runtime from deep inside mycrate
	result, err := r.Resolve("mycrate/src/runtime/scheduler.rs", model.ImportFact{
		Specifier: "crate::runtime",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution, got external")
	}
	if result.ResolvedPath != "mycrate/src/runtime/mod.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "mycrate/src/runtime/mod.rs")
	}
}

func TestWorkspaceMemberDeps(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// serde is a dep of mycrate, should resolve as dependency
	result, err := r.Resolve("mycrate/src/lib.rs", model.ImportFact{
		Specifier: "serde::Deserialize",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external")
	}
	if result.Reason != "dependency" {
		t.Errorf("reason = %q, want %q", result.Reason, "dependency")
	}

	// log is a dep of myutil, should resolve as dependency
	result, err = r.Resolve("myutil/src/lib.rs", model.ImportFact{
		Specifier: "log::info",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external")
	}
	if result.Reason != "dependency" {
		t.Errorf("reason = %q, want %q", result.Reason, "dependency")
	}
}

func TestWorkspaceCrossReference(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// myutil::format from mycrate should resolve
	result, err := r.Resolve("mycrate/src/lib.rs", model.ImportFact{
		Specifier: "myutil::format",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for workspace cross-reference")
	}
	if result.ResolvedPath != "myutil/src/format.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "myutil/src/format.rs")
	}
}

// setupGlobWorkspace creates a workspace with glob patterns (like Bevy).
func setupGlobWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Root Cargo.toml with glob.
	rootToml := `[workspace]
resolver = "2"
members = [
  "crates/*",
]
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	// crates/engine member.
	os.MkdirAll(filepath.Join(dir, "crates", "engine", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "crates", "engine", "Cargo.toml"), []byte(`[package]
name = "engine"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "crates", "engine", "src", "lib.rs"), []byte("pub mod render;"), 0o644)
	os.WriteFile(filepath.Join(dir, "crates", "engine", "src", "render.rs"), []byte("pub fn draw() {}"), 0o644)

	// crates/utils member.
	os.MkdirAll(filepath.Join(dir, "crates", "utils", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "crates", "utils", "Cargo.toml"), []byte(`[package]
name = "utils"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "crates", "utils", "src", "lib.rs"), []byte("pub fn help() {}"), 0o644)

	return dir
}

func TestGlobWorkspaceCrateResolution(t *testing.T) {
	dir := setupGlobWorkspace(t)
	r := New(dir)

	// Verify glob expansion found both members.
	if len(r.workMembers) < 2 {
		t.Fatalf("expected at least 2 workspace members, got %d", len(r.workMembers))
	}

	// crate::render from engine crate
	result, err := r.Resolve("crates/engine/src/lib.rs", model.ImportFact{
		Specifier: "crate::render",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for crate:: in glob workspace")
	}
	if result.ResolvedPath != "crates/engine/src/render.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "crates/engine/src/render.rs")
	}
}

func TestGlobWorkspaceCrossReference(t *testing.T) {
	dir := setupGlobWorkspace(t)
	r := New(dir)

	// utils::lib from engine crate
	result, err := r.Resolve("crates/engine/src/lib.rs", model.ImportFact{
		Specifier: "utils",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	// "utils" is a workspace member name — should resolve to its lib.rs
	if result.External {
		t.Error("expected local resolution for workspace cross-reference")
	}
}

// I-028: Deep crate:: path resolution.

func TestResolveCrateDeepPartial(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// crate::models::user::Profile → should partially resolve to src/models/user.rs
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::models::user::Profile",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for deep partial path")
	}
	if result.ResolvedPath != "src/models/user.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/models/user.rs")
	}
	if result.ResolutionMethod != "module_path_partial" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "module_path_partial")
	}
}

// I-030: Hyphen/underscore normalization.

func TestWorkspaceHyphenUnderscore(t *testing.T) {
	dir := t.TempDir()

	// Root Cargo.toml with hyphenated member name.
	rootToml := `[workspace]
resolver = "2"
members = [
  "my-core",
  "my-app",
]
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	// my-core member (crate name will be "my_core" in Rust).
	os.MkdirAll(filepath.Join(dir, "my-core", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "my-core", "Cargo.toml"), []byte(`[package]
name = "my-core"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "my-core", "src", "lib.rs"), []byte("pub fn core_fn() {}"), 0o644)

	// my-app member.
	os.MkdirAll(filepath.Join(dir, "my-app", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "my-app", "Cargo.toml"), []byte(`[package]
name = "my-app"
version = "0.1.0"

[dependencies]
my-core = { path = "../my-core" }
`), 0o644)
	os.WriteFile(filepath.Join(dir, "my-app", "src", "lib.rs"), []byte("use my_core;"), 0o644)

	r := New(dir)

	// my_core (underscore form) should resolve to my-core workspace member.
	result, err := r.Resolve("my-app/src/lib.rs", model.ImportFact{
		Specifier: "my_core",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for hyphenated workspace member")
	}
	if result.ResolvedPath != "my-core/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "my-core/src/lib.rs")
	}
}

// --- Bare crate/super/self resolution (from stripped grouped imports) ---

func TestResolveBareCrate(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// Bare "crate" (from stripped crate::{A, B}) should resolve to crate root.
	result, err := r.Resolve("src/config.rs", model.ImportFact{
		Specifier: "crate::{App, Plugin}",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for bare crate")
	}
	// After brace stripping, "crate" should resolve to src/main.rs (crate root).
	if result.ResolvedPath != "src/main.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/main.rs")
	}
}

func TestResolveBareCrateWorkspace(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// Bare "crate" from within a workspace member should resolve to that member's lib.rs.
	result, err := r.Resolve("mycrate/src/runtime/scheduler.rs", model.ImportFact{
		Specifier: "crate::{App, Plugin}",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "mycrate/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "mycrate/src/lib.rs")
	}
}

func TestResolveBareSuperFromModRs(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// Bare "super" from mod.rs → parent directory entry point.
	// src/models/mod.rs: super = src/ → src/main.rs
	result, err := r.Resolve("src/models/mod.rs", model.ImportFact{
		Specifier: "super::{Config, App}",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for bare super")
	}
	if result.ResolvedPath != "src/main.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/main.rs")
	}
}

func TestResolveBareSuperFromNamedFile(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// Bare "super" from named file.
	// src/models/user.rs: super = parent module = src/models/ → src/models/mod.rs
	result, err := r.Resolve("src/models/user.rs", model.ImportFact{
		Specifier: "super::{Config, App}",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for bare super from named file")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/models/mod.rs")
	}
}

func TestResolveBareSelf(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// Bare "self" should resolve to the source file itself.
	result, err := r.Resolve("src/config.rs", model.ImportFact{
		Specifier: "self::{bar, baz}",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for bare self")
	}
	if result.ResolvedPath != "src/config.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/config.rs")
	}
}

// --- Flattened crate:: paths (what the new extractor produces) ---

func TestResolveFlattenedCratePaths(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// The new extractor flattens crate::{config::Config, models} to individual facts.
	// crate::config::Config resolves via partial fallback to src/config.rs
	// (Config is a type name, not a file — partial fallback finds config.rs).
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::config::Config",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/config.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/config.rs")
	}
	if result.ResolutionMethod != "module_path_partial" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "module_path_partial")
	}
}

// --- Wildcard resolution ---

func TestResolveWildcard(t *testing.T) {
	dir := setupCrate(t)

	// Create a prelude.rs for wildcard test.
	if err := os.WriteFile(filepath.Join(dir, "src", "prelude.rs"), []byte("pub use crate::*;"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// crate::prelude (from wildcard use crate::prelude::*) should resolve to src/prelude.rs.
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier:     "crate::prelude",
		ImportedNames: []string{"*"},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for wildcard import")
	}
	if result.ResolvedPath != "src/prelude.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/prelude.rs")
	}
}

func TestResolveWildcardModels(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// crate::models (from use crate::models::*) should resolve to src/models/mod.rs.
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier:     "crate::models",
		ImportedNames: []string{"*"},
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/models/mod.rs")
	}
}

// setupImplicitMemberWorkspace creates a workspace where only some members are
// explicitly listed, with sibling crates that should be implicitly discovered.
func setupImplicitMemberWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Root Cargo.toml lists only "compiler/rustc" as a member.
	rootToml := `[workspace]
resolver = "2"
members = [
  "compiler/rustc",
]
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	// compiler/rustc — explicitly listed member.
	os.MkdirAll(filepath.Join(dir, "compiler", "rustc", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc", "Cargo.toml"), []byte(`[package]
name = "rustc"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc", "src", "lib.rs"), []byte("pub fn main() {}"), 0o644)

	// compiler/rustc_abi — implicit sibling crate (not in workspace members).
	os.MkdirAll(filepath.Join(dir, "compiler", "rustc_abi", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc_abi", "Cargo.toml"), []byte(`[package]
name = "rustc_abi"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc_abi", "src", "lib.rs"), []byte("pub mod extern_abi;"), 0o644)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc_abi", "src", "extern_abi.rs"), []byte("pub struct Abi {}"), 0o644)

	// compiler/rustc_public — another implicit sibling crate.
	os.MkdirAll(filepath.Join(dir, "compiler", "rustc_public", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc_public", "Cargo.toml"), []byte(`[package]
name = "rustc_public"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc_public", "src", "lib.rs"), []byte("pub mod compiler_interface;"), 0o644)
	os.WriteFile(filepath.Join(dir, "compiler", "rustc_public", "src", "compiler_interface.rs"), []byte("pub fn interface() {}"), 0o644)

	return dir
}

func TestImplicitMemberDiscovery(t *testing.T) {
	dir := setupImplicitMemberWorkspace(t)
	r := New(dir)

	// Should have discovered 3 members: rustc (explicit) + rustc_abi + rustc_public (implicit).
	if len(r.workMembers) < 3 {
		t.Fatalf("expected at least 3 workspace members, got %d", len(r.workMembers))
	}

	// crate:: resolution should work for implicit member rustc_abi.
	result, err := r.Resolve("compiler/rustc_abi/src/lib.rs", model.ImportFact{
		Specifier: "crate::extern_abi",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for crate:: in implicit member")
	}
	if result.ResolvedPath != "compiler/rustc_abi/src/extern_abi.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "compiler/rustc_abi/src/extern_abi.rs")
	}

	// Cross-crate reference from rustc to implicit member rustc_abi.
	result, err = r.Resolve("compiler/rustc/src/lib.rs", model.ImportFact{
		Specifier: "rustc_abi::extern_abi",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for cross-crate reference to implicit member")
	}
	if result.ResolvedPath != "compiler/rustc_abi/src/extern_abi.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "compiler/rustc_abi/src/extern_abi.rs")
	}
}

// setupNestedWorkspace creates a repo with a nested workspace (like rust-analyzer in rust-lang/rust).
func setupNestedWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Root Cargo.toml with a top-level member.
	rootToml := `[workspace]
resolver = "2"
members = [
  "tools/helper",
]
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	// tools/helper — explicit member.
	os.MkdirAll(filepath.Join(dir, "tools", "helper", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "helper", "Cargo.toml"), []byte(`[package]
name = "helper"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "tools", "helper", "src", "lib.rs"), []byte("pub fn help() {}"), 0o644)

	// tools/analyzer — nested workspace root (like rust-analyzer).
	os.MkdirAll(filepath.Join(dir, "tools", "analyzer"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "Cargo.toml"), []byte(`[workspace]
resolver = "2"
members = [
  "crates/*",
]
`), 0o644)

	// tools/analyzer/crates/hir-def — nested workspace member.
	os.MkdirAll(filepath.Join(dir, "tools", "analyzer", "crates", "hir-def", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "crates", "hir-def", "Cargo.toml"), []byte(`[package]
name = "hir-def"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "crates", "hir-def", "src", "lib.rs"), []byte("pub mod expr_store;"), 0o644)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "crates", "hir-def", "src", "expr_store.rs"), []byte("pub struct ExprStore {}"), 0o644)

	// tools/analyzer/crates/proc-macro-api — another nested workspace member.
	os.MkdirAll(filepath.Join(dir, "tools", "analyzer", "crates", "proc-macro-api", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "crates", "proc-macro-api", "Cargo.toml"), []byte(`[package]
name = "proc-macro-api"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "crates", "proc-macro-api", "src", "lib.rs"), []byte("pub fn api() {}"), 0o644)

	return dir
}

func TestNestedWorkspaceDiscovery(t *testing.T) {
	dir := setupNestedWorkspace(t)
	r := New(dir)

	// Should have discovered nested workspace members.
	// Expect: helper (explicit) + hir-def + proc-macro-api (nested workspace).
	if len(r.workMembers) < 3 {
		t.Fatalf("expected at least 3 workspace members, got %d (members: %+v)", len(r.workMembers), r.workMembers)
	}

	// crate:: resolution should work for nested workspace member hir-def.
	result, err := r.Resolve("tools/analyzer/crates/hir-def/src/lib.rs", model.ImportFact{
		Specifier: "crate::expr_store",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for crate:: in nested workspace member")
	}
	if result.ResolvedPath != "tools/analyzer/crates/hir-def/src/expr_store.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "tools/analyzer/crates/hir-def/src/expr_store.rs")
	}

	// Cross-crate: proc_macro_api from hir-def.
	result, err = r.Resolve("tools/analyzer/crates/hir-def/src/lib.rs", model.ImportFact{
		Specifier: "proc_macro_api",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for cross-crate reference to nested workspace member")
	}
	if result.ResolvedPath != "tools/analyzer/crates/proc-macro-api/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "tools/analyzer/crates/proc-macro-api/src/lib.rs")
	}
}

func TestParsePackageName(t *testing.T) {
	dir := t.TempDir()

	// Valid package.
	os.WriteFile(filepath.Join(dir, "valid.toml"), []byte(`[package]
name = "my-crate"
version = "0.1.0"
`), 0o644)
	if got := parsePackageName(filepath.Join(dir, "valid.toml")); got != "my-crate" {
		t.Errorf("parsePackageName(valid) = %q, want %q", got, "my-crate")
	}

	// Workspace-only (no [package]).
	os.WriteFile(filepath.Join(dir, "workspace.toml"), []byte(`[workspace]
members = ["crates/*"]
`), 0o644)
	if got := parsePackageName(filepath.Join(dir, "workspace.toml")); got != "" {
		t.Errorf("parsePackageName(workspace) = %q, want %q", got, "")
	}

	// Missing file.
	if got := parsePackageName(filepath.Join(dir, "missing.toml")); got != "" {
		t.Errorf("parsePackageName(missing) = %q, want %q", got, "")
	}
}

func TestParseCargoTomlInfo(t *testing.T) {
	dir := t.TempDir()

	// Combined package + workspace.
	os.WriteFile(filepath.Join(dir, "combined.toml"), []byte(`[package]
name = "my-project"
version = "0.1.0"

[workspace]
members = [
  "crates/*",
  "tools/helper",
]
`), 0o644)
	info := parseCargoTomlInfo(filepath.Join(dir, "combined.toml"))
	if info.packageName != "my-project" {
		t.Errorf("packageName = %q, want %q", info.packageName, "my-project")
	}
	if !info.isWorkspace {
		t.Error("expected isWorkspace = true")
	}
	if len(info.members) != 2 {
		t.Errorf("members count = %d, want 2", len(info.members))
	}

	// Workspace only.
	os.WriteFile(filepath.Join(dir, "ws.toml"), []byte(`[workspace]
members = [
  "sub/*",
]
`), 0o644)
	info2 := parseCargoTomlInfo(filepath.Join(dir, "ws.toml"))
	if info2.packageName != "" {
		t.Errorf("packageName = %q, want empty", info2.packageName)
	}
	if !info2.isWorkspace {
		t.Error("expected isWorkspace = true")
	}
	if len(info2.members) != 1 {
		t.Errorf("members count = %d, want 1", len(info2.members))
	}
}

func TestParseSingleLineArray(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{`members = ["xtask/", "lib/*", "crates/*"]`, []string{"xtask/", "lib/*", "crates/*"}},
		{`members = ["single"]`, []string{"single"}},
		{`members = []`, nil},
		{`resolver = "2"`, nil},           // no array
		{`exclude = ["bench"]`, []string{"bench"}}, // works on any key
		{`members = [`, nil},              // multi-line opening, not a single-line array
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := parseSingleLineArray(tt.line)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSingleLineArray(%q) = %v (len %d), want %v (len %d)",
					tt.line, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseSingleLineArray(%q)[%d] = %q, want %q", tt.line, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseCargoTomlInfoSingleLineMembers(t *testing.T) {
	dir := t.TempDir()

	// Single-line members (like rust-analyzer's Cargo.toml).
	os.WriteFile(filepath.Join(dir, "single.toml"), []byte(`[workspace]
members = ["xtask/", "lib/*", "crates/*"]
exclude = ["bench"]
`), 0o644)
	info := parseCargoTomlInfo(filepath.Join(dir, "single.toml"))
	if !info.isWorkspace {
		t.Error("expected isWorkspace = true")
	}
	if len(info.members) != 3 {
		t.Errorf("members count = %d, want 3; got %v", len(info.members), info.members)
	}
	// Verify exclude and resolver aren't parsed as members.
	for _, m := range info.members {
		if m == "bench" || m == "2" {
			t.Errorf("unexpected member %q (parsed from non-members line)", m)
		}
	}
}

func TestParseCargoTomlInfoComments(t *testing.T) {
	dir := t.TempDir()

	// Members with comments (like rust-lang/rust's Cargo.toml).
	os.WriteFile(filepath.Join(dir, "comments.toml"), []byte(`[workspace]
members = [
# tidy-alphabetical-start
  "compiler/rustc",
  "src/tools/miri",
# tidy-alphabetical-end
]
`), 0o644)
	info := parseCargoTomlInfo(filepath.Join(dir, "comments.toml"))
	if !info.isWorkspace {
		t.Error("expected isWorkspace = true")
	}
	if len(info.members) != 2 {
		t.Errorf("members count = %d, want 2; got %v", len(info.members), info.members)
	}
	for _, m := range info.members {
		if m == "# tidy-alphabetical-start" || m == "# tidy-alphabetical-end" ||
			m == "tidy-alphabetical-start" || m == "tidy-alphabetical-end" {
			t.Errorf("comment parsed as member: %q", m)
		}
	}
}

func TestDetectCrateCommentFiltering(t *testing.T) {
	dir := t.TempDir()

	// Root Cargo.toml with comments in members list.
	rootToml := `[workspace]
resolver = "2"
members = [
# tidy-alphabetical-start
  "mycrate",
# tidy-alphabetical-end
]
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	os.MkdirAll(filepath.Join(dir, "mycrate", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "mycrate", "Cargo.toml"), []byte(`[package]
name = "mycrate"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "mycrate", "src", "lib.rs"), []byte("pub fn f() {}"), 0o644)

	r := New(dir)

	// Should have exactly 1 member (not comments parsed as members).
	memberCount := 0
	for _, m := range r.workMembers {
		if m.dir == "mycrate" {
			memberCount++
		}
	}
	if memberCount != 1 {
		t.Errorf("expected 1 'mycrate' member, got %d; all members: %+v", memberCount, r.workMembers)
	}
}

// setupNestedWorkspaceSingleLine creates a repo with a nested workspace
// that uses single-line members format (like rust-analyzer in rust-lang/rust).
func setupNestedWorkspaceSingleLine(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Root Cargo.toml.
	rootToml := `[workspace]
resolver = "2"
members = [
  "tools/helper",
]
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	// tools/helper — explicit member.
	os.MkdirAll(filepath.Join(dir, "tools", "helper", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "helper", "Cargo.toml"), []byte(`[package]
name = "helper"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "tools", "helper", "src", "lib.rs"), []byte("pub fn help() {}"), 0o644)

	// tools/analyzer — nested workspace with single-line members.
	os.MkdirAll(filepath.Join(dir, "tools", "analyzer"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "Cargo.toml"), []byte(`[workspace]
members = ["crates/*", "lib/lsp-server"]
exclude = ["bench"]
`), 0o644)

	// tools/analyzer/crates/hir — nested member.
	os.MkdirAll(filepath.Join(dir, "tools", "analyzer", "crates", "hir", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "crates", "hir", "Cargo.toml"), []byte(`[package]
name = "hir"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "crates", "hir", "src", "lib.rs"), []byte("pub fn hir() {}"), 0o644)

	// tools/analyzer/lib/lsp-server — another nested member (direct path).
	os.MkdirAll(filepath.Join(dir, "tools", "analyzer", "lib", "lsp-server", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "lib", "lsp-server", "Cargo.toml"), []byte(`[package]
name = "lsp-server"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "tools", "analyzer", "lib", "lsp-server", "src", "lib.rs"), []byte("pub fn serve() {}"), 0o644)

	return dir
}

func TestNestedWorkspaceSingleLineMembers(t *testing.T) {
	dir := setupNestedWorkspaceSingleLine(t)
	r := New(dir)

	// Should have discovered nested workspace members via single-line array.
	// Expect: helper (explicit) + hir (glob) + lsp-server (direct path).
	if len(r.workMembers) < 3 {
		t.Fatalf("expected at least 3 workspace members, got %d (members: %+v)", len(r.workMembers), r.workMembers)
	}

	// crate:: resolution should work for nested member hir.
	result, err := r.Resolve("tools/analyzer/crates/hir/src/lib.rs", model.ImportFact{
		Specifier: "crate",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for crate:: in nested single-line workspace member")
	}
	if result.ResolvedPath != "tools/analyzer/crates/hir/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "tools/analyzer/crates/hir/src/lib.rs")
	}

	// Cross-crate: lsp_server from hir.
	result, err = r.Resolve("tools/analyzer/crates/hir/src/lib.rs", model.ImportFact{
		Specifier: "lsp_server",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for cross-crate reference to nested single-line workspace member")
	}
	if result.ResolvedPath != "tools/analyzer/lib/lsp-server/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "tools/analyzer/lib/lsp-server/src/lib.rs")
	}
}

func TestDetectCrateSingleLineMembers(t *testing.T) {
	dir := t.TempDir()

	// Root Cargo.toml with single-line members.
	rootToml := `[workspace]
members = ["mycrate", "myutil"]
exclude = ["bench"]
resolver = "2"
`
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(rootToml), 0o644)

	os.MkdirAll(filepath.Join(dir, "mycrate", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "mycrate", "Cargo.toml"), []byte(`[package]
name = "mycrate"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "mycrate", "src", "lib.rs"), []byte("pub fn f() {}"), 0o644)

	os.MkdirAll(filepath.Join(dir, "myutil", "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "myutil", "Cargo.toml"), []byte(`[package]
name = "myutil"
version = "0.1.0"
`), 0o644)
	os.WriteFile(filepath.Join(dir, "myutil", "src", "lib.rs"), []byte("pub fn g() {}"), 0o644)

	r := New(dir)

	// Should have 2 members, not bogus entries from exclude/resolver.
	if len(r.workMembers) < 2 {
		t.Fatalf("expected at least 2 workspace members, got %d", len(r.workMembers))
	}

	// Verify resolution works.
	result, err := r.Resolve("mycrate/src/lib.rs", model.ImportFact{
		Specifier: "myutil",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for single-line workspace cross-reference")
	}
	if result.ResolvedPath != "myutil/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "myutil/src/lib.rs")
	}
}

// --- Single-segment type import fallbacks ---

func TestCrateSingleSegmentFallback(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// crate::Config where Config is a type in main.rs, not a module file.
	// config.rs exists so it resolves there, but let's test with a non-existent name.
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::NonExistentType",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Should fall back to crate root (main.rs) instead of not_found.
	if result.External {
		t.Error("expected local resolution for single-segment crate:: fallback")
	}
	if result.ResolvedPath != "src/main.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/main.rs")
	}
}

func TestCrateSingleSegmentFallbackWorkspace(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// crate::SomeType from mycrate — should fall back to mycrate/src/lib.rs.
	result, err := r.Resolve("mycrate/src/runtime/scheduler.rs", model.ImportFact{
		Specifier: "crate::SomeType",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for single-segment crate:: fallback in workspace")
	}
	if result.ResolvedPath != "mycrate/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "mycrate/src/lib.rs")
	}
}

func TestCrateMultiSegmentFallback(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// crate::nonexistent::Thing — multi-segment, should fall back to crate root.
	result, err := r.Resolve("src/main.rs", model.ImportFact{
		Specifier: "crate::nonexistent::Thing",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Multi-segment paths now fall back to crate root (main.rs).
	if result.External {
		t.Error("expected local resolution for multi-segment crate:: fallback")
	}
	if result.ResolvedPath != "src/main.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/main.rs")
	}
}

func TestWorkspaceMemberSingleSegmentFallback(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// myutil::SomeType — single-segment after stripping crate prefix.
	// Should fall back to myutil/src/lib.rs.
	result, err := r.Resolve("mycrate/src/lib.rs", model.ImportFact{
		Specifier: "myutil::SomeType",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for single-segment workspace member fallback")
	}
	if result.ResolvedPath != "myutil/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "myutil/src/lib.rs")
	}
}

// --- Rust 2018 module entry ---

func TestRust2018ModuleEntry(t *testing.T) {
	dir := setupCrate(t)

	// Create Rust 2018-style module: src/handler.rs with src/handler/ directory.
	handlerDir := filepath.Join(dir, "src", "handler")
	os.MkdirAll(handlerDir, 0o755)
	os.WriteFile(filepath.Join(dir, "src", "handler.rs"), []byte("pub mod routes;"), 0o644)
	os.WriteFile(filepath.Join(handlerDir, "routes.rs"), []byte("pub fn handle() {}"), 0o644)

	r := New(dir)

	// super:: from handler/routes.rs → should resolve to handler.rs (Rust 2018).
	// routes.rs is a named file: super = parent module = handler/ dir.
	// But handler/ has no mod.rs — the module root is handler.rs in parent dir.
	result, err := r.Resolve("src/handler/routes.rs", model.ImportFact{
		Specifier: "super",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for Rust 2018 module entry")
	}
	if result.ResolvedPath != "src/handler.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/handler.rs")
	}
}

func TestRust2018ModuleEntryFromModRs(t *testing.T) {
	dir := setupCrate(t)

	// Create a deep Rust 2018-style module: src/api/v2/ with src/api/v2.rs.
	apiDir := filepath.Join(dir, "src", "api")
	v2Dir := filepath.Join(apiDir, "v2")
	os.MkdirAll(v2Dir, 0o755)
	os.WriteFile(filepath.Join(apiDir, "mod.rs"), []byte("pub mod v2;"), 0o644)
	os.WriteFile(filepath.Join(apiDir, "v2.rs"), []byte("pub mod endpoint;"), 0o644)
	os.WriteFile(filepath.Join(v2Dir, "endpoint.rs"), []byte("pub fn get() {}"), 0o644)

	r := New(dir)

	// super:: from api/v2/endpoint.rs → should find api/v2.rs (Rust 2018).
	result, err := r.Resolve("src/api/v2/endpoint.rs", model.ImportFact{
		Specifier: "super",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for Rust 2018 module entry from deep path")
	}
	if result.ResolvedPath != "src/api/v2.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/api/v2.rs")
	}
}

// --- super:: single-segment type fallback ---

func TestSuperSingleSegmentFallbackFromNamedFile(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// super::SomeType from src/models/user.rs — SomeType is not a file,
	// should fall back to parent module entry (src/models/mod.rs).
	result, err := r.Resolve("src/models/user.rs", model.ImportFact{
		Specifier: "super::SomeType",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for super:: single-segment fallback")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/models/mod.rs")
	}
}

func TestSuperSingleSegmentFallbackFromModRs(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// super::SomeType from src/models/mod.rs — SomeType is not a file in src/,
	// should fall back to parent module entry (src/main.rs).
	result, err := r.Resolve("src/models/mod.rs", model.ImportFact{
		Specifier: "super::SomeType",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for super:: single-segment fallback from mod.rs")
	}
	if result.ResolvedPath != "src/main.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/main.rs")
	}
}

// --- self:: single-segment type fallback ---

func TestSelfSingleSegmentFallbackFromNamedFile(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// self::MyType from src/config.rs — MyType is not a file in config/,
	// should fall back to src/config.rs itself.
	result, err := r.Resolve("src/config.rs", model.ImportFact{
		Specifier: "self::MyType",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for self:: single-segment fallback")
	}
	if result.ResolvedPath != "src/config.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/config.rs")
	}
}

func TestSelfSingleSegmentFallbackFromModRs(t *testing.T) {
	dir := setupCrate(t)
	r := New(dir)

	// self::MyType from src/models/mod.rs — MyType is not a file in models/,
	// should fall back to src/models/mod.rs itself.
	result, err := r.Resolve("src/models/mod.rs", model.ImportFact{
		Specifier: "self::MyType",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for self:: single-segment fallback from mod.rs")
	}
	if result.ResolvedPath != "src/models/mod.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "src/models/mod.rs")
	}
}

// --- Multi-segment cross-crate fallback ---

func TestWorkspaceMemberMultiSegmentFallback(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// myutil::some::deep::Type — multi-segment, should fall back to myutil/src/lib.rs.
	result, err := r.Resolve("mycrate/src/lib.rs", model.ImportFact{
		Specifier: "myutil::some::deep::Type",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for multi-segment workspace member fallback")
	}
	if result.ResolvedPath != "myutil/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "myutil/src/lib.rs")
	}
}

// --- Multi-segment crate:: fallback ---

func TestCrateMultiSegmentFallbackWorkspace(t *testing.T) {
	dir := setupWorkspace(t)
	r := New(dir)

	// crate::nonexistent::deep::Thing from workspace member.
	result, err := r.Resolve("mycrate/src/lib.rs", model.ImportFact{
		Specifier: "crate::nonexistent::deep::Thing",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for multi-segment crate:: fallback in workspace")
	}
	if result.ResolvedPath != "mycrate/src/lib.rs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "mycrate/src/lib.rs")
	}
}
