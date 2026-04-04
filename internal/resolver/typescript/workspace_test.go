package typescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

// --- Workspace detection tests ---

func TestDetectPnpmWorkspace(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", `packages:
  - 'packages/*'
  - 'apps/*'
`)
	// Create two packages.
	createFileContent(t, root, "packages/utils/package.json", `{"name": "utils", "main": "./dist/index.js"}`)
	createFile(t, root, "packages/utils/src/index.ts")
	createFileContent(t, root, "apps/web/package.json", `{"name": "web", "main": "./index.ts"}`)
	createFile(t, root, "apps/web/index.ts")

	ws := detectWorkspaces(root)
	if ws == nil {
		t.Fatal("expected workspace config, got nil")
	}
	if len(ws.packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(ws.packages))
	}
	if ws.packages["utils"] == nil {
		t.Error("missing 'utils' package")
	}
	if ws.packages["web"] == nil {
		t.Error("missing 'web' package")
	}
}

func TestDetectPackageJSONWorkspacesArray(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "package.json", `{"name": "root", "workspaces": ["packages/*"]}`)
	createFileContent(t, root, "packages/core/package.json", `{"name": "@myorg/core", "main": "./index.ts"}`)
	createFile(t, root, "packages/core/index.ts")

	ws := detectWorkspaces(root)
	if ws == nil {
		t.Fatal("expected workspace config, got nil")
	}
	if ws.packages["@myorg/core"] == nil {
		t.Fatal("missing '@myorg/core' package")
	}
	if ws.packages["@myorg/core"].Root != "packages/core" {
		t.Errorf("got root %q, want %q", ws.packages["@myorg/core"].Root, "packages/core")
	}
}

func TestDetectPackageJSONWorkspacesObject(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "package.json", `{"name": "root", "workspaces": {"packages": ["packages/*"]}}`)
	createFileContent(t, root, "packages/lib/package.json", `{"name": "lib"}`)

	ws := detectWorkspaces(root)
	if ws == nil {
		t.Fatal("expected workspace config, got nil")
	}
	if ws.packages["lib"] == nil {
		t.Fatal("missing 'lib' package")
	}
}

func TestDetectNoWorkspace(t *testing.T) {
	root := t.TempDir()

	// Just a regular package.json without workspaces.
	createFileContent(t, root, "package.json", `{"name": "solo-project"}`)

	ws := detectWorkspaces(root)
	if ws != nil {
		t.Fatal("expected nil workspace config for non-monorepo")
	}
}

// --- Package discovery tests ---

func TestDiscoverMultiplePackages(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "packages/a/package.json", `{"name": "pkg-a"}`)
	createFileContent(t, root, "packages/b/package.json", `{"name": "pkg-b"}`)

	pkgs := discoverPackages(root, []string{"packages/*"})
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs["pkg-a"] == nil || pkgs["pkg-b"] == nil {
		t.Error("missing expected package")
	}
}

func TestDiscoverScopedPackage(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "packages/ui/package.json", `{"name": "@scope/ui", "types": "./dist/index.d.ts"}`)

	pkgs := discoverPackages(root, []string{"packages/*"})
	if pkgs["@scope/ui"] == nil {
		t.Fatal("missing '@scope/ui' package")
	}
	if pkgs["@scope/ui"].Types != "./dist/index.d.ts" {
		t.Errorf("got types %q", pkgs["@scope/ui"].Types)
	}
}

func TestDiscoverSkipsMissingPackageJSON(t *testing.T) {
	root := t.TempDir()

	// Directory exists but no package.json.
	if err := os.MkdirAll(filepath.Join(root, "packages/empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	createFileContent(t, root, "packages/valid/package.json", `{"name": "valid"}`)

	pkgs := discoverPackages(root, []string{"packages/*"})
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs["valid"] == nil {
		t.Fatal("missing 'valid' package")
	}
}

// --- Specifier matching tests ---

func TestMatchUnscoped(t *testing.T) {
	pkgs := map[string]*workspacePackage{
		"my-lib": {Name: "my-lib", Root: "packages/my-lib"},
	}

	pkg, subpath := matchPackage(pkgs, "my-lib")
	if pkg == nil {
		t.Fatal("expected match")
	}
	if subpath != "." {
		t.Errorf("got subpath %q, want %q", subpath, ".")
	}
}

func TestMatchUnscopedSubpath(t *testing.T) {
	pkgs := map[string]*workspacePackage{
		"my-lib": {Name: "my-lib", Root: "packages/my-lib"},
	}

	pkg, subpath := matchPackage(pkgs, "my-lib/utils")
	if pkg == nil {
		t.Fatal("expected match")
	}
	if subpath != "./utils" {
		t.Errorf("got subpath %q, want %q", subpath, "./utils")
	}
}

func TestMatchScoped(t *testing.T) {
	pkgs := map[string]*workspacePackage{
		"@org/core": {Name: "@org/core", Root: "packages/core"},
	}

	pkg, subpath := matchPackage(pkgs, "@org/core")
	if pkg == nil {
		t.Fatal("expected match")
	}
	if subpath != "." {
		t.Errorf("got subpath %q, want %q", subpath, ".")
	}
}

func TestMatchScopedSubpath(t *testing.T) {
	pkgs := map[string]*workspacePackage{
		"@org/core": {Name: "@org/core", Root: "packages/core"},
	}

	pkg, subpath := matchPackage(pkgs, "@org/core/helpers")
	if pkg == nil {
		t.Fatal("expected match")
	}
	if subpath != "./helpers" {
		t.Errorf("got subpath %q, want %q", subpath, "./helpers")
	}
}

// --- Resolution tests ---

func TestResolveWorkspaceExports(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/utils/package.json", `{
		"name": "@myorg/utils",
		"exports": {".": "./dist/index.js"}
	}`)
	createFile(t, root, "packages/utils/src/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@myorg/utils", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "packages/utils/src/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/utils/src/index.ts")
	}
	if result.ResolutionMethod != "workspace_package" {
		t.Errorf("got method %q, want %q", result.ResolutionMethod, "workspace_package")
	}
}

func TestResolveWorkspaceSubpathExports(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/utils/package.json", `{
		"name": "@myorg/utils",
		"exports": {
			".": "./dist/index.js",
			"./helpers": "./dist/helpers.js"
		}
	}`)
	createFile(t, root, "packages/utils/src/helpers.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@myorg/utils/helpers", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "packages/utils/src/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/utils/src/helpers.ts")
	}
}

func TestResolveWorkspaceTypes(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/config/package.json", `{
		"name": "config",
		"types": "./dist/index.d.ts"
	}`)
	createFile(t, root, "packages/config/src/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "config", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "packages/config/src/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/config/src/index.ts")
	}
}

func TestResolveWorkspaceModule(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/lib/package.json", `{
		"name": "lib",
		"module": "./dist/index.mjs"
	}`)
	createFile(t, root, "packages/lib/src/index.mts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "lib", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "packages/lib/src/index.mts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/lib/src/index.mts")
	}
}

func TestResolveWorkspaceMain(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/legacy/package.json", `{
		"name": "legacy",
		"main": "./dist/index.js"
	}`)
	createFile(t, root, "packages/legacy/src/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "legacy", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "packages/legacy/src/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/legacy/src/index.ts")
	}
}

func TestResolveWorkspaceConventional(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/simple/package.json", `{"name": "simple"}`)
	createFile(t, root, "packages/simple/src/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "simple", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "packages/simple/src/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/simple/src/index.ts")
	}
}

func TestResolveWorkspaceSourceMappingExists(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/auth/package.json", `{
		"name": "auth",
		"exports": {".": "./dist/auth.js"}
	}`)
	createFile(t, root, "packages/auth/src/auth.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "auth", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResolvedPath != "packages/auth/src/auth.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/auth/src/auth.ts")
	}
}

func TestResolveWorkspaceNoSourceMapping(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/gen/package.json", `{
		"name": "gen",
		"exports": {".": "./dist/index.js"}
	}`)
	// No src/index.ts, no dist/index.js either.

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "gen", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Fatal("expected external when no source mapping found")
	}
	if result.Reason != "workspace_entry_not_found" {
		t.Errorf("got reason %q, want %q", result.Reason, "workspace_entry_not_found")
	}
}

// --- Integration tests ---

func TestResolveFullPriority(t *testing.T) {
	root := t.TempDir()

	// Set up workspace.
	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/shared/package.json", `{"name": "shared"}`)
	createFile(t, root, "packages/shared/src/index.ts")

	// Set up tsconfig that does NOT match "shared".
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"paths": {"@alias/*": ["src/alias/*"]}
		}
	}`)

	r := New(root)
	r.DetectWorkspaces()

	// "shared" should resolve via workspace (tsconfig paths miss).
	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "shared", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal via workspace")
	}
	if result.ResolutionMethod != "workspace_package" {
		t.Errorf("got method %q, want %q", result.ResolutionMethod, "workspace_package")
	}
}

func TestResolveWorkspaceNotFound(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/utils/package.json", `{"name": "utils"}`)
	createFile(t, root, "packages/utils/src/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	// "unknown-pkg" doesn't match any workspace package → external.
	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "unknown-pkg", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Fatal("expected external for unknown package")
	}
	if result.ResolutionMethod != "external" {
		t.Errorf("got method %q, want %q", result.ResolutionMethod, "external")
	}
}

func TestResolveNonMonorepo(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "package.json", `{"name": "solo"}`)

	r := New(root)
	r.DetectWorkspaces()

	// No workspace → should fall through to external without panic.
	result, err := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "some-lib", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Fatal("expected external")
	}
}

// --- Edge case tests ---

func TestExportsNestedConditions(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/typed/package.json", `{
		"name": "typed",
		"exports": {
			".": {
				"types": "./dist/index.d.ts",
				"import": "./dist/index.mjs",
				"require": "./dist/index.cjs"
			}
		}
	}`)
	// Source-first: dist/index.d.ts → src/index.ts
	createFile(t, root, "packages/typed/src/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "typed", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "packages/typed/src/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/typed/src/index.ts")
	}
}

func TestPnpmWorkspaceWithComments(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", `# This is a comment
packages:
  # Package directories
  - 'packages/*'
  - 'apps/*'  # inline comment is part of glob but harmless since no match
`)
	createFileContent(t, root, "packages/foo/package.json", `{"name": "foo"}`)
	createFile(t, root, "packages/foo/src/index.ts")

	ws := detectWorkspaces(root)
	if ws == nil {
		t.Fatal("expected workspace config")
	}
	if ws.packages["foo"] == nil {
		t.Fatal("missing 'foo' package")
	}
}

func TestTsconfigPathsTakesPriorityOverWorkspace(t *testing.T) {
	root := t.TempDir()

	// Workspace has "shared" package.
	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/shared/package.json", `{"name": "shared"}`)
	createFile(t, root, "packages/shared/src/index.ts")

	// tsconfig paths also maps "shared" to a local alias.
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"paths": {"shared": ["src/shared/index.ts"]}
		}
	}`)
	createFile(t, root, "src/shared/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	// tsconfig paths (step 2) should win over workspace (step 3).
	result, err := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "shared", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolutionMethod != "tsconfig_paths" {
		t.Errorf("got method %q, want %q", result.ResolutionMethod, "tsconfig_paths")
	}
	if result.ResolvedPath != "src/shared/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/shared/index.ts")
	}
}

// --- mapToSource unit tests ---

func TestMapToSourceDist(t *testing.T) {
	got := mapToSource("./dist/index.js")
	if got != "src/index.ts" {
		t.Errorf("got %q, want %q", got, "src/index.ts")
	}
}

func TestMapToSourceBuild(t *testing.T) {
	got := mapToSource("./build/utils.mjs")
	if got != "src/utils.mts" {
		t.Errorf("got %q, want %q", got, "src/utils.mts")
	}
}

func TestMapToSourceNotDistOrBuild(t *testing.T) {
	got := mapToSource("./src/index.ts")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- GetWorkspacePackages tests ---

func TestGetWorkspacePackagesNil(t *testing.T) {
	r := New(t.TempDir())
	if pkgs := r.GetWorkspacePackages(); pkgs != nil {
		t.Errorf("expected nil, got %v", pkgs)
	}
}

func TestGetWorkspacePackagesSorted(t *testing.T) {
	root := t.TempDir()
	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/z/package.json", `{"name": "z-pkg"}`)
	createFileContent(t, root, "packages/a/package.json", `{"name": "a-pkg"}`)

	r := New(root)
	r.DetectWorkspaces()

	names := r.GetWorkspacePackages()
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d", len(names))
	}
	if names[0] != "a-pkg" || names[1] != "z-pkg" {
		t.Errorf("expected sorted [a-pkg z-pkg], got %v", names)
	}
}

// --- Subpath file fallback tests (Fix 3) ---

// TestResolveWorkspaceSubpathNoExports verifies that @repo/validation/api
// resolves to packages/validation/src/api.ts when the package has no exports field.
func TestResolveWorkspaceSubpathNoExports(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/validation/package.json", `{"name": "@repo/validation"}`)
	createFile(t, root, "packages/validation/src/api.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@repo/validation/api", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "packages/validation/src/api.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/validation/src/api.ts")
	}
}

// TestResolveWorkspaceSubpathNoExportsDirectFile verifies fallback to direct
// file path when src/ doesn't contain the file.
func TestResolveWorkspaceSubpathNoExportsDirectFile(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/validation/package.json", `{"name": "@repo/validation"}`)
	// No src/ dir, file is directly under package root.
	createFile(t, root, "packages/validation/api.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@repo/validation/api", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "packages/validation/api.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/validation/api.ts")
	}
}

// TestResolveWorkspaceSubpathNoExportsIndexFile verifies fallback to index file
// in a subpath directory.
func TestResolveWorkspaceSubpathNoExportsIndexFile(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/validation/package.json", `{"name": "@repo/validation"}`)
	createFile(t, root, "packages/validation/src/api/index.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@repo/validation/api", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "packages/validation/src/api/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/validation/src/api/index.ts")
	}
}

// TestResolveWorkspaceSubpathNoExportsNestedPath verifies that nested subpaths
// like @repo/validation/utils/helpers resolve correctly.
func TestResolveWorkspaceSubpathNoExportsNestedPath(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/validation/package.json", `{"name": "@repo/validation"}`)
	createFile(t, root, "packages/validation/src/utils/helpers.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@repo/validation/utils/helpers", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "packages/validation/src/utils/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/validation/src/utils/helpers.ts")
	}
}

// TestResolveWorkspaceSubpathSourceFirstPriority verifies that src/ is checked
// before the package root for subpath fallback.
func TestResolveWorkspaceSubpathSourceFirstPriority(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/validation/package.json", `{"name": "@repo/validation"}`)
	// Both src/api.ts and api.ts exist — src/ should win.
	createFile(t, root, "packages/validation/src/api.ts")
	createFile(t, root, "packages/validation/api.ts")

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@repo/validation/api", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "packages/validation/src/api.ts" {
		t.Errorf("got %q, want %q (src/ should take priority)", result.ResolvedPath, "packages/validation/src/api.ts")
	}
}

// TestResolveWorkspaceSubpathNoExportsNotFound verifies that when no matching
// files exist for a subpath, it still returns workspace_entry_not_found.
func TestResolveWorkspaceSubpathNoExportsNotFound(t *testing.T) {
	root := t.TempDir()

	createFileContent(t, root, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n")
	createFileContent(t, root, "packages/validation/package.json", `{"name": "@repo/validation"}`)
	// No api.ts anywhere.

	r := New(root)
	r.DetectWorkspaces()

	result, err := r.Resolve("apps/web/app.ts", model.ImportFact{
		Specifier: "@repo/validation/api", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Fatal("expected external when no matching files found")
	}
	if result.Reason != "workspace_entry_not_found" {
		t.Errorf("got reason %q, want %q", result.Reason, "workspace_entry_not_found")
	}
}
