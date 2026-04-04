package python

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

// helper to create a file with optional content in a temp directory.
func createFile(t *testing.T, root, relPath string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("# "+relPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveAbsoluteSimple(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/utils.py")
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("myapp/main.py", model.ImportFact{
		Specifier:  "myapp.utils",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "myapp/utils.py" {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, "myapp/utils.py")
	}
}

func TestResolveAbsoluteDotted(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "myapp/sub/__init__.py")
	createFile(t, root, "myapp/sub/handler.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("tests/test_main.py", model.ImportFact{
		Specifier:  "myapp.sub.handler",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "myapp/sub/handler.py" {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, "myapp/sub/handler.py")
	}
}

func TestResolveFromImportSubmodule(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "myapp/models.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	// "from myapp import models" where models.py is a submodule.
	// Since all imported names resolve as submodules, prefer the submodule.
	result, err := r.Resolve("tests/test_main.py", model.ImportFact{
		Specifier:     "myapp",
		ImportType:    "absolute",
		ImportedNames: []string{"models"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "myapp/models.py" {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, "myapp/models.py")
	}
}

func TestResolveFromImportPackageOverSubmodule(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "django/template/__init__.py")
	createFile(t, root, "django/template/loader.py")
	createFile(t, root, "django/__init__.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	// "from django.template import Context, Engine, TemplateDoesNotExist, loader"
	// Only loader resolves as a submodule; the rest are __init__.py attributes.
	// When not all names are submodules, prefer the package __init__.py.
	result, err := r.Resolve("django/views/defaults.py", model.ImportFact{
		Specifier:     "django.template",
		ImportType:    "absolute",
		ImportedNames: []string{"Context", "Engine", "TemplateDoesNotExist", "loader"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != filepath.Join("django", "template", "__init__.py") {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, filepath.Join("django", "template", "__init__.py"))
	}
}

func TestResolveFromImportSubmoduleFallback(t *testing.T) {
	root := t.TempDir()
	// Namespace package: directory exists but no __init__.py.
	if err := os.MkdirAll(filepath.Join(root, "mypkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	createFile(t, root, "mypkg/sub.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	// "from mypkg import sub" where mypkg has no __init__.py but sub.py exists.
	// All imported names are submodules, so return the submodule.
	result, err := r.Resolve("main.py", model.ImportFact{
		Specifier:     "mypkg",
		ImportType:    "absolute",
		ImportedNames: []string{"sub"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != filepath.Join("mypkg", "sub.py") {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, filepath.Join("mypkg", "sub.py"))
	}
}

func TestResolveRelativeSamePackage(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "myapp/models.py")
	createFile(t, root, "myapp/main.py")

	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("myapp/main.py", model.ImportFact{
		Specifier:     ".models",
		ImportType:    "relative",
		ImportedNames: []string{"User"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "myapp/models.py" {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, "myapp/models.py")
	}
}

func TestResolveRelativeParent(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "myapp/models.py")
	createFile(t, root, "myapp/sub/__init__.py")
	createFile(t, root, "myapp/sub/handler.py")

	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("myapp/sub/handler.py", model.ImportFact{
		Specifier:     "..models",
		ImportType:    "relative",
		ImportedNames: []string{"User"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "myapp/models.py" {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, "myapp/models.py")
	}
}

func TestResolveRelativeDotImport(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "myapp/models.py")
	createFile(t, root, "myapp/utils.py")

	r := New(root)
	r.DetectPackageRoots()

	// from . import models
	result, err := r.Resolve("myapp/utils.py", model.ImportFact{
		Specifier:     ".",
		ImportType:    "relative",
		ImportedNames: []string{"models"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal, got external")
	}
	if result.ResolvedPath != "myapp/models.py" {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, "myapp/models.py")
	}
}

func TestResolveStdlib(t *testing.T) {
	root := t.TempDir()
	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("main.py", model.ImportFact{
		Specifier:  "os",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Fatal("expected external for stdlib")
	}
	if result.Reason != "stdlib" {
		t.Errorf("Reason = %q, want %q", result.Reason, "stdlib")
	}
}

func TestResolveExternal(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("main.py", model.ImportFact{
		Specifier:  "flask",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Fatal("expected external for third-party package")
	}
	if result.Reason != "external" {
		t.Errorf("Reason = %q, want %q", result.Reason, "external")
	}
}

func TestResolveNamespacePackage(t *testing.T) {
	root := t.TempDir()
	// Directory exists but no __init__.py — namespace package.
	if err := os.MkdirAll(filepath.Join(root, "mypkg/sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No __init__.py files, no .py file matching.
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("main.py", model.ImportFact{
		Specifier:  "mypkg.sub",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Fatal("expected external for namespace package without __init__.py")
	}
}

func TestResolveSrcLayout(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "pyproject.toml")
	createFile(t, root, "src/myapp/__init__.py")
	createFile(t, root, "src/myapp/core.py")

	r := New(root)
	roots, err := r.DetectPackageRoots()
	if err != nil {
		t.Fatal(err)
	}

	// Verify src layout was detected.
	hasSrc := false
	for _, pr := range roots {
		if pr.Path == "src" {
			hasSrc = true
			break
		}
	}
	if !hasSrc {
		t.Fatal("expected src layout to be detected")
	}

	result, err := r.Resolve("tests/test_core.py", model.ImportFact{
		Specifier:  "myapp.core",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal for src layout import")
	}
	if result.ResolvedPath != filepath.Join("src", "myapp", "core.py") {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, filepath.Join("src", "myapp", "core.py"))
	}
}

func TestResolvePackageInit(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	result, err := r.Resolve("test.py", model.ImportFact{
		Specifier:  "myapp",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal for package __init__.py")
	}
	if result.ResolvedPath != filepath.Join("myapp", "__init__.py") {
		t.Errorf("ResolvedPath = %q, want %q", result.ResolvedPath, filepath.Join("myapp", "__init__.py"))
	}
}

func TestDetectPackageRoots(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "pyproject.toml")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := New(root)
	roots, err := r.DetectPackageRoots()
	if err != nil {
		t.Fatal(err)
	}

	methods := make(map[string]bool)
	for _, pr := range roots {
		methods[pr.DetectionMethod] = true
	}

	if !methods["pyproject_toml"] {
		t.Error("expected pyproject_toml detection method")
	}
	if !methods["src_layout"] {
		t.Error("expected src_layout detection method")
	}
}

func TestDetectPackageRootsFallback(t *testing.T) {
	root := t.TempDir()
	// No pyproject.toml, setup.py, etc.

	r := New(root)
	roots, err := r.DetectPackageRoots()
	if err != nil {
		t.Fatal(err)
	}

	if len(roots) != 1 {
		t.Fatalf("expected 1 root (fallback), got %d", len(roots))
	}
	if roots[0].DetectionMethod != "repo_root" {
		t.Errorf("expected repo_root fallback, got %q", roots[0].DetectionMethod)
	}
}

// TestResolveRelativeMultiModule verifies that per-name facts from
// "from . import models, views, utils" each resolve to their own file.
func TestResolveRelativeMultiModule(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "myapp/__init__.py")
	createFile(t, root, "myapp/models.py")
	createFile(t, root, "myapp/views.py")
	createFile(t, root, "myapp/utils.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	// Each name is a separate fact (after I-016 fix).
	names := []string{"models", "views", "utils"}
	for _, name := range names {
		result, err := r.Resolve("myapp/main.py", model.ImportFact{
			Specifier:     ".",
			ImportType:    "relative",
			ImportedNames: []string{name},
		}, root)
		if err != nil {
			t.Fatalf("Resolve %q: %v", name, err)
		}
		if result.External {
			t.Errorf("Resolve %q: expected internal, got external", name)
		}
		want := filepath.Join("myapp", name+".py")
		if result.ResolvedPath != want {
			t.Errorf("Resolve %q: ResolvedPath = %q, want %q", name, result.ResolvedPath, want)
		}
	}
}

// TestResolveAbsoluteMultiMixed verifies that per-name facts correctly
// separate submodule resolution from __init__.py attribute resolution.
func TestResolveAbsoluteMultiMixed(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "django/__init__.py")
	createFile(t, root, "django/template/__init__.py")
	createFile(t, root, "django/template/loader.py")
	createFile(t, root, "setup.py")

	r := New(root)
	r.DetectPackageRoots()

	// "loader" is a submodule — single-name fact resolves to loader.py.
	result, err := r.Resolve("django/views/defaults.py", model.ImportFact{
		Specifier:     "django.template",
		ImportType:    "absolute",
		ImportedNames: []string{"loader"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal for 'loader', got external")
	}
	if result.ResolvedPath != filepath.Join("django", "template", "loader.py") {
		t.Errorf("loader: ResolvedPath = %q, want %q", result.ResolvedPath, filepath.Join("django", "template", "loader.py"))
	}

	// "Context" is NOT a submodule — single-name fact resolves to __init__.py.
	result, err = r.Resolve("django/views/defaults.py", model.ImportFact{
		Specifier:     "django.template",
		ImportType:    "absolute",
		ImportedNames: []string{"Context"},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal for 'Context', got external")
	}
	if result.ResolvedPath != filepath.Join("django", "template", "__init__.py") {
		t.Errorf("Context: ResolvedPath = %q, want %q", result.ResolvedPath, filepath.Join("django", "template", "__init__.py"))
	}
}
