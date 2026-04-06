package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStdlibDetection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")

	r := New(root)

	tests := []struct {
		spec   string
		reason string
	}{
		{"fmt", "stdlib"},
		{"os", "stdlib"},
		{"net/http", "stdlib"},
		{"encoding/json", "stdlib"},
		{"crypto/sha256", "stdlib"},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			result, err := r.Resolve("main.go", model.ImportFact{
				Specifier:  tt.spec,
				ImportType: "absolute",
			}, root)
			if err != nil {
				t.Fatal(err)
			}
			if !result.External {
				t.Errorf("expected external for %q", tt.spec)
			}
			if result.Reason != tt.reason {
				t.Errorf("reason = %q, want %q", result.Reason, tt.reason)
			}
		})
	}
}

func TestExternalDetection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")

	r := New(root)

	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "github.com/other/pkg",
		ImportType: "absolute",
	}, root)
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

func TestModuleLocalResolution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	writeFile(t, root, "pkg/handler.go", "package pkg\n")
	writeFile(t, root, "pkg/util.go", "package pkg\n")
	writeFile(t, root, "pkg/handler_test.go", "package pkg\n")

	r := New(root)

	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected internal resolution")
	}
	if result.ResolutionMethod != "module_local" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "module_local")
	}
	// Should resolve to the first non-test .go file.
	if result.ResolvedPath != "pkg/handler.go" {
		t.Errorf("resolved = %q, want %q", result.ResolvedPath, "pkg/handler.go")
	}
}

func TestModuleLocalResolveAll(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	writeFile(t, root, "pkg/handler.go", "package pkg\n")
	writeFile(t, root, "pkg/util.go", "package pkg\n")
	writeFile(t, root, "pkg/handler_test.go", "package pkg\n")

	r := New(root)

	results, err := r.ResolveAll("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}

	// Should resolve to 1 file (cap=1, handler.go wins alphabetically).
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	for _, r := range results {
		if r.External {
			t.Error("expected internal resolution")
		}
	}
}

func TestTestFileExclusion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	writeFile(t, root, "pkg/handler_test.go", "package pkg\n")
	// Only test files in the directory.

	r := New(root)

	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external (no non-test files found)")
	}
	if result.Reason != "not_found" {
		t.Errorf("reason = %q, want %q", result.Reason, "not_found")
	}
}

func TestCgoResolution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")

	r := New(root)

	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "C",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external for cgo")
	}
	if result.Reason != "cgo" {
		t.Errorf("reason = %q, want %q", result.Reason, "cgo")
	}
}

func TestGoWorkMultiModule(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "go.work", `go 1.21

use (
	./svc-a
	./svc-b
)
`)
	writeFile(t, root, "svc-a/go.mod", "module example.com/svc-a\n\ngo 1.21\n")
	writeFile(t, root, "svc-a/handler.go", "package main\n")
	writeFile(t, root, "svc-b/go.mod", "module example.com/svc-b\n\ngo 1.21\n")
	writeFile(t, root, "svc-b/pkg/util.go", "package pkg\n")

	r := New(root)

	// svc-a importing svc-b.
	result, err := r.Resolve("svc-a/handler.go", model.ImportFact{
		Specifier:  "example.com/svc-b/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected internal resolution via go.work")
	}
	if result.ResolvedPath != "svc-b/pkg/util.go" {
		t.Errorf("resolved = %q, want %q", result.ResolvedPath, "svc-b/pkg/util.go")
	}
}

func TestNoGoMod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.go", "package main\n")
	// No go.mod.

	r := New(root)

	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	// Without go.mod, everything non-stdlib is external.
	if !result.External {
		t.Error("expected external without go.mod")
	}
}

func TestSubpackageResolution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	writeFile(t, root, "internal/db/store.go", "package db\n")
	writeFile(t, root, "internal/db/migrations.go", "package db\n")

	r := New(root)

	result, err := r.Resolve("cmd/main.go", model.ImportFact{
		Specifier:  "example.com/mymod/internal/db",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected internal resolution for subpackage")
	}
	if result.ResolutionMethod != "module_local" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "module_local")
	}
}

// I-023: Replace directive tests.

func TestReplaceDirectiveSingleLine(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/mymod

go 1.21

replace example.com/othermod => ./othermod
`)
	writeFile(t, root, "othermod/go.mod", "module example.com/othermod\n\ngo 1.21\n")
	writeFile(t, root, "othermod/util.go", "package othermod\n")

	r := New(root)

	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "example.com/othermod",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected internal resolution via replace directive")
	}
	if result.ResolvedPath != "othermod/util.go" {
		t.Errorf("resolved = %q, want %q", result.ResolvedPath, "othermod/util.go")
	}
}

func TestReplaceDirectiveBlock(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/mymod

go 1.21

replace (
	example.com/libA => ./libs/libA
	example.com/libB => ./libs/libB
)
`)
	writeFile(t, root, "libs/libA/go.mod", "module example.com/libA\n\ngo 1.21\n")
	writeFile(t, root, "libs/libA/api.go", "package libA\n")
	writeFile(t, root, "libs/libB/go.mod", "module example.com/libB\n\ngo 1.21\n")
	writeFile(t, root, "libs/libB/impl.go", "package libB\n")

	r := New(root)

	resultA, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "example.com/libA",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if resultA.External {
		t.Error("expected internal resolution for libA via block replace")
	}

	resultB, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "example.com/libB",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if resultB.External {
		t.Error("expected internal resolution for libB via block replace")
	}
}

func TestReplaceExternal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/mymod

go 1.21

replace github.com/old/pkg => github.com/new/pkg v1.0.0
`)

	r := New(root)

	// External replace (not local) should NOT create a module entry.
	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "github.com/old/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external for non-local replace")
	}
}

func TestReplaceSubpath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", `module example.com/mymod

go 1.21

replace example.com/othermod => ./othermod
`)
	writeFile(t, root, "othermod/go.mod", "module example.com/othermod\n\ngo 1.21\n")
	writeFile(t, root, "othermod/pkg/handler.go", "package pkg\n")

	r := New(root)

	result, err := r.Resolve("main.go", model.ImportFact{
		Specifier:  "example.com/othermod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected internal resolution for replace subpath")
	}
	if result.ResolvedPath != "othermod/pkg/handler.go" {
		t.Errorf("resolved = %q, want %q", result.ResolvedPath, "othermod/pkg/handler.go")
	}
}

// I-019: Per-package cap tests.

func TestResolveAllCapsPerPackage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	// Create 5 files in the package.
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go"} {
		writeFile(t, root, "pkg/"+name, "package pkg\n")
	}

	r := New(root)

	results, err := r.ResolveAll("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Errorf("expected exactly 1 result, got %d", len(results))
	}
}

func TestResolveAllPrioritizesMatchingName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	// Create files where the package-named file should be prioritized.
	for _, name := range []string{"handler.go", "doc.go", "util.go", "pkg.go", "types.go"} {
		writeFile(t, root, "pkg/"+name, "package pkg\n")
	}

	r := New(root)

	results, err := r.ResolveAll("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d", len(results))
	}
	// pkg.go should be first (matches package name).
	if results[0].ResolvedPath != "pkg/pkg.go" {
		t.Errorf("first result = %q, want %q (matching package name)", results[0].ResolvedPath, "pkg/pkg.go")
	}
}

// I-022: Same-package edge tests.

func TestSamePackageEdges(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	writeFile(t, root, "pkg/handler.go", "package pkg\n")
	writeFile(t, root, "pkg/util.go", "package pkg\n")
	writeFile(t, root, "pkg/handler_test.go", "package pkg\n")

	r := New(root)

	results := r.ResolveSamePackageEdges("pkg/handler.go", root)
	if len(results) == 0 {
		t.Fatal("expected same-package edges")
	}

	// Should include util.go but not handler_test.go (test files excluded by findGoFiles).
	foundUtil := false
	for _, result := range results {
		if result.ResolvedPath == "pkg/util.go" {
			foundUtil = true
		}
		if result.ResolvedPath == "pkg/handler_test.go" {
			t.Error("test files should not be in same-package edges")
		}
		if result.ResolvedPath == "pkg/handler.go" {
			t.Error("source file itself should not be in same-package edges")
		}
		if result.ResolutionMethod != "same_package" {
			t.Errorf("method = %q, want %q", result.ResolutionMethod, "same_package")
		}
	}
	if !foundUtil {
		t.Error("expected util.go in same-package edges")
	}
}

// Platform-specific file tests.

func TestPlatformSpecificFilesDeprioritized(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	writeFile(t, root, "pkg/pkg.go", "package pkg\n")
	writeFile(t, root, "pkg/pkg_linux.go", "package pkg\n")
	writeFile(t, root, "pkg/pkg_windows.go", "package pkg\n")
	writeFile(t, root, "pkg/pkg_amd64.go", "package pkg\n")

	r := New(root)

	results, err := r.ResolveAll("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Canonical pkg.go should win over platform-specific files.
	if results[0].ResolvedPath != "pkg/pkg.go" {
		t.Errorf("result = %q, want %q", results[0].ResolvedPath, "pkg/pkg.go")
	}
}

func TestPlatformSpecificOnlyPackage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	// Only platform-specific files — should still resolve gracefully.
	writeFile(t, root, "pkg/apparmor_linux.go", "package pkg\n")
	writeFile(t, root, "pkg/apparmor_windows.go", "package pkg\n")

	r := New(root)

	results, err := r.ResolveAll("main.go", model.ImportFact{
		Specifier:  "example.com/mymod/pkg",
		ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Should pick the first alphabetically.
	if results[0].ResolvedPath != "pkg/apparmor_linux.go" {
		t.Errorf("result = %q, want %q", results[0].ResolvedPath, "pkg/apparmor_linux.go")
	}
}

func TestIsPlatformSpecific(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"handler.go", false},
		{"handler_linux.go", true},
		{"handler_windows.go", true},
		{"handler_amd64.go", true},
		{"handler_linux_amd64.go", true},
		{"handler_darwin_arm64.go", true},
		{"apparmor_default.go", false},
		{"pkg.go", false},
		{"doc.go", false},
		{"util_test.go", false},  // _test is not a platform
		{"a_b_linux.go", true},   // linux suffix
		{"file_js.go", true},     // js is a GOOS
		{"file_wasm.go", true},   // wasm is a GOARCH
		{"file_wasip1.go", true}, // wasip1 is a GOOS
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPlatformSpecific(tt.name)
			if got != tt.expected {
				t.Errorf("isPlatformSpecific(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestSamePackageEdgesCapped(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/mymod\n\ngo 1.21\n")
	// Create many sibling files.
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go", "main.go"} {
		writeFile(t, root, "pkg/"+name, "package pkg\n")
	}

	r := New(root)

	results := r.ResolveSamePackageEdges("pkg/main.go", root)
	if len(results) != maxSamePackageFiles {
		t.Errorf("expected %d same-package results, got %d", maxSamePackageFiles, len(results))
	}
}
