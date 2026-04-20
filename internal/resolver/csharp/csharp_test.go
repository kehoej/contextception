package csharp

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

	result, err := r.Resolve("Foo.cs", model.ImportFact{
		Specifier: "System.Collections.Generic",
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

	// Create a C# project layout with .csproj.
	projDir := filepath.Join(dir, "MyApp")
	modelsDir := filepath.Join(projDir, "Models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "MyApp.csproj"), []byte("<Project></Project>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "User.cs"), []byte("public class User {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	result, err := r.Resolve("MyApp/Services/UserService.cs", model.ImportFact{
		Specifier: "Models.User",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "MyApp/Models/User.cs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveThirdParty(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)

	result, err := r.Resolve("Foo.cs", model.ImportFact{
		Specifier: "Newtonsoft.Json",
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

func TestResolveCsprojDetection(t *testing.T) {
	dir := t.TempDir()

	// Create solution with two projects.
	for _, proj := range []string{"MyApp.Core", "MyApp.Web"} {
		projDir := filepath.Join(dir, proj)
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projDir, proj+".csproj"), []byte("<Project></Project>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := New(dir)

	// Should detect both project directories as source roots.
	found := make(map[string]bool)
	for _, root := range r.sourceRoots {
		found[root] = true
	}
	if !found["MyApp.Core"] {
		t.Error("expected MyApp.Core as source root")
	}
	if !found["MyApp.Web"] {
		t.Error("expected MyApp.Web as source root")
	}
}

func TestResolveByFilenameSearch(t *testing.T) {
	dir := t.TempDir()

	// Create a file in a nested directory that doesn't match namespace-to-path mapping.
	projDir := filepath.Join(dir, "MyApp")
	nestedDir := filepath.Join(projDir, "Infrastructure", "Data")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "MyApp.csproj"), []byte("<Project></Project>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "UserRepository.cs"), []byte("public class UserRepository {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// Namespace doesn't map to path: MyApp.Data.UserRepository → try filename search.
	result, err := r.Resolve("MyApp/Services/App.cs", model.ImportFact{
		Specifier: "MyApp.Data.UserRepository",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution via filename search")
	}
	if result.ResolvedPath != "MyApp/Infrastructure/Data/UserRepository.cs" {
		t.Errorf("resolvedPath = %q", result.ResolvedPath)
	}
}

func TestResolveAllSameNamespace(t *testing.T) {
	dir := t.TempDir()

	// Create source files in same directory.
	srcDir := filepath.Join(dir, "MyApp", "Models")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"User.cs", "Product.cs", "Order.cs"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte("public class "+strings.TrimSuffix(name, ".cs")+" {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := New(dir)

	results, err := r.ResolveAll("MyApp/Models/User.cs", model.ImportFact{
		Specifier:  "MyApp.Models.*",
		ImportType: "same_namespace",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should find Product.cs and Order.cs (not User.cs itself).
	if len(results) != 2 {
		t.Fatalf("expected 2 same-namespace results, got %d", len(results))
	}
	for _, result := range results {
		if result.ResolutionMethod != "same_namespace" {
			t.Errorf("method = %q, want %q", result.ResolutionMethod, "same_namespace")
		}
	}
}

func TestResolveAllSkipsTestFiles(t *testing.T) {
	dir := t.TempDir()

	srcDir := filepath.Join(dir, "MyApp", "Services")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"UserService.cs", "UserServiceTest.cs", "UserServiceTests.cs", "OrderService.cs"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte("class "+strings.TrimSuffix(name, ".cs")+" {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := New(dir)

	results, err := r.ResolveAll("MyApp/Services/UserService.cs", model.ImportFact{
		Specifier:  "MyApp.Services.*",
		ImportType: "same_namespace",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should only find OrderService.cs (skip test files and self).
	if len(results) != 1 {
		t.Fatalf("expected 1 same-namespace result, got %d", len(results))
	}
	if results[0].ResolvedPath != "MyApp/Services/OrderService.cs" {
		t.Errorf("resolvedPath = %q, want %q", results[0].ResolvedPath, "MyApp/Services/OrderService.cs")
	}
}

func TestResolveProjectNamespace(t *testing.T) {
	dir := t.TempDir()

	// Create a C# project with dotted directory name (like Jellyfin).
	projDir := filepath.Join(dir, "MediaBrowser.Controller")
	entitiesDir := filepath.Join(projDir, "Entities")
	if err := os.MkdirAll(entitiesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "MediaBrowser.Controller.csproj"), []byte("<Project></Project>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entitiesDir, "Genre.cs"), []byte("public class Genre {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entitiesDir, "BaseItem.cs"), []byte("public class BaseItem {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// using MediaBrowser.Controller.Entities should resolve to a file in the directory.
	result, err := r.Resolve("SomeApp/Foo.cs", model.ImportFact{
		Specifier:  "MediaBrowser.Controller.Entities",
		ImportType: "absolute",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution via project namespace")
	}
	if result.ResolutionMethod != "project_namespace" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "project_namespace")
	}
}

func TestResolveAllAbsoluteNamespace(t *testing.T) {
	dir := t.TempDir()

	// Create dotted project directory with multiple files.
	projDir := filepath.Join(dir, "MediaBrowser.Controller")
	entitiesDir := filepath.Join(projDir, "Entities")
	if err := os.MkdirAll(entitiesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "MediaBrowser.Controller.csproj"), []byte("<Project></Project>"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Genre.cs", "BaseItem.cs", "Folder.cs"} {
		if err := os.WriteFile(filepath.Join(entitiesDir, name), []byte("public class "+strings.TrimSuffix(name, ".cs")+" {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := New(dir)

	// ResolveAll for absolute using should NOT expand to all files (same_namespace handles that).
	// It should delegate to single Resolve, returning 1 result.
	results, err := r.ResolveAll("SomeApp/Foo.cs", model.ImportFact{
		Specifier:  "MediaBrowser.Controller.Entities",
		ImportType: "absolute",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (single Resolve), got %d: %+v", len(results), results)
	}
	if results[0].External {
		t.Error("expected local resolution")
	}
}

func TestResolveProjectNamespaceFile(t *testing.T) {
	dir := t.TempDir()

	// Create dotted project directory with a specific file.
	projDir := filepath.Join(dir, "Jellyfin.Api")
	helpersDir := filepath.Join(projDir, "Helpers")
	if err := os.MkdirAll(helpersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "Jellyfin.Api.csproj"), []byte("<Project></Project>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helpersDir, "RequestHelpers.cs"), []byte("public static class RequestHelpers {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)

	// using Jellyfin.Api.Helpers.RequestHelpers → Jellyfin.Api/Helpers/RequestHelpers.cs
	result, err := r.Resolve("SomeApp/Foo.cs", model.ImportFact{
		Specifier:  "Jellyfin.Api.Helpers.RequestHelpers",
		ImportType: "absolute",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Error("expected local resolution")
	}
	if result.ResolvedPath != "Jellyfin.Api/Helpers/RequestHelpers.cs" {
		t.Errorf("resolvedPath = %q, want %q", result.ResolvedPath, "Jellyfin.Api/Helpers/RequestHelpers.cs")
	}
}

func TestResolveSameNamespaceHandledByResolveAll(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)

	result, err := r.Resolve("Foo.cs", model.ImportFact{
		Specifier:  "MyApp.*",
		ImportType: "same_namespace",
	}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.External {
		t.Error("expected external (delegated to ResolveAll)")
	}
	if result.Reason != "same_namespace_handled_by_resolve_all" {
		t.Errorf("reason = %q", result.Reason)
	}
}
