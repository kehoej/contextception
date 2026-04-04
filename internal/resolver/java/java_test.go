package java

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestResolveStdlib(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)

	result, err := r.Resolve("Foo.java", model.ImportFact{
		Specifier: "java.util.List",
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

func TestResolveLocalFile(t *testing.T) {
	dir := t.TempDir()

	// Create Maven-style layout.
	srcDir := filepath.Join(dir, "src", "main", "java", "com", "example")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "User.java"), []byte("public class User {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	result, err := r.Resolve("src/main/java/com/example/App.java", model.ImportFact{
		Specifier: "com.example.User",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/main/java/com/example/User.java" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveThirdParty(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)

	result, err := r.Resolve("Foo.java", model.ImportFact{
		Specifier: "org.springframework.boot.SpringApplication",
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

func TestResolveSimpleLayout(t *testing.T) {
	dir := t.TempDir()

	// Simple layout: src/com/example/User.java
	srcDir := filepath.Join(dir, "src", "com", "example")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "User.java"), []byte("public class User {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	result, err := r.Resolve("src/com/example/App.java", model.ImportFact{
		Specifier: "com.example.User",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "src/com/example/User.java" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveWildcard(t *testing.T) {
	dir := t.TempDir()

	// Wildcard imports resolve to the package directory's files.
	// For now they resolve as external since we can't map *.
	r := New(dir)

	result, err := r.Resolve("Foo.java", model.ImportFact{
		Specifier: "com.example.model.*",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	// Wildcard with no local match → external.
	if !result.External {
		t.Error("expected external for unresolved wildcard")
	}
}

func TestResolveStaticImportMember(t *testing.T) {
	dir := t.TempDir()

	// Create class file for static import resolution.
	srcDir := filepath.Join(dir, "src", "main", "java", "com", "example", "util")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "MathHelper.java"), []byte("public class MathHelper {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// Static import of a method: com.example.util.MathHelper.calculate
	result, err := r.Resolve("Foo.java", model.ImportFact{
		Specifier: "com.example.util.MathHelper.calculate",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for static import")
	}
	if result.ResolvedPath != "src/main/java/com/example/util/MathHelper.java" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

// I-027: Static import with explicit flag.

func TestResolveStaticImportWithFlag(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "src", "main", "java", "com", "example")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "Config.java"), []byte("public class Config { public static final int MAX_VALUE = 100; }"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// Static import with uppercase member (would fail lowercase heuristic without static flag).
	result, err := r.Resolve("Foo.java", model.ImportFact{
		Specifier:  "com.example.Config.MAX_VALUE",
		ImportType: "static",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for static import with flag")
	}
	if result.ResolutionMethod != "static_import_class" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "static_import_class")
	}
	if result.ResolvedPath != "src/main/java/com/example/Config.java" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

// I-025: Inner type resolution.

func TestResolveInnerType(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "src", "main", "java", "org", "example")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "Banner.java"), []byte("public class Banner { public enum Mode { OFF, CONSOLE, LOG } }"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// Inner type: org.example.Banner.Mode → strip .Mode → Banner.java
	result, err := r.Resolve("Foo.java", model.ImportFact{
		Specifier: "org.example.Banner.Mode",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for inner type")
	}
	if result.ResolutionMethod != "inner_type" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "inner_type")
	}
	if result.ResolvedPath != "src/main/java/org/example/Banner.java" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveNestedInnerType(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "src", "main", "java", "com", "example")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "Outer.java"), []byte("public class Outer { class Inner { class Deep {} } }"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// Nested inner type: com.example.Outer.Inner.Deep → strip .Deep, .Inner → Outer.java
	result, err := r.Resolve("Foo.java", model.ImportFact{
		Specifier: "com.example.Outer.Inner.Deep",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution for nested inner type")
	}
	if result.ResolvedPath != "src/main/java/com/example/Outer.java" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

// I-026: Same-package (ResolveAll) test.

func TestResolveAllSamePackage(t *testing.T) {
	dir := t.TempDir()

	// Create source files in same package.
	srcDir := filepath.Join(dir, "src", "main", "java", "com", "example")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Foo.java", "Bar.java", "Baz.java"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte("public class "+strings.TrimSuffix(name, ".java")+" {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := New(dir)

	results, err := r.ResolveAll("src/main/java/com/example/Foo.java", model.ImportFact{
		Specifier:  "com.example.*",
		ImportType: "same_package",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should find Bar.java and Baz.java (not Foo.java itself).
	if len(results) != 2 {
		t.Fatalf("expected 2 same-package results, got %d", len(results))
	}
	for _, result := range results {
		if result.ResolutionMethod != "same_package" {
			t.Errorf("method = %q, want %q", result.ResolutionMethod, "same_package")
		}
	}
}
