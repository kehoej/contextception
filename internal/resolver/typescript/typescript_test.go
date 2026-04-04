package typescript

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func createFile(t *testing.T, root, relPath string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("// "+relPath+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func createFileContent(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- Relative resolution tests ---

func TestResolveRelativeExact(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/utils.ts")

	r := New(root)
	result, err := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./utils", ImportType: "relative",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/utils.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/utils.ts")
	}
	if result.ResolutionMethod != "relative" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "relative")
	}
}

func TestResolveRelativeTSX(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/Button.tsx")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./Button", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/Button.tsx" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/Button.tsx")
	}
}

func TestResolveRelativeIndex(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/utils/index.ts")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./utils", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/utils/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/utils/index.ts")
	}
}

func TestResolveRelativeIndexTSX(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/components/index.tsx")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./components", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/components/index.tsx" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/components/index.tsx")
	}
}

func TestResolveRelativeParentDir(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/config.ts")

	r := New(root)
	result, _ := r.Resolve("src/sub/deep.ts", model.ImportFact{
		Specifier: "../config", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/config.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/config.ts")
	}
}

func TestResolveRelativeJStoTS(t *testing.T) {
	// TS projects often import with .js extension that maps to .ts source files.
	root := t.TempDir()
	createFile(t, root, "src/utils.ts")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./utils.js", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/utils.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/utils.ts")
	}
}

func TestResolveRelativeJStoTSX(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/Button.tsx")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./Button.js", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/Button.tsx" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/Button.tsx")
	}
}

func TestResolveRelativeJSExists(t *testing.T) {
	// When .js file actually exists (no .ts equivalent), resolve to it.
	root := t.TempDir()
	createFile(t, root, "lib/legacy.js")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "../lib/legacy.js", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "lib/legacy.js" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "lib/legacy.js")
	}
}

func TestResolveRelativeMJStoMTS(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/mod.mts")

	r := New(root)
	result, _ := r.Resolve("src/app.mts", model.ImportFact{
		Specifier: "./mod.mjs", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/mod.mts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/mod.mts")
	}
}

func TestResolveRelativeNotFound(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/app.ts")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./nonexistent", ImportType: "relative",
	}, root)
	if !result.External {
		t.Fatal("expected external for not found")
	}
	if result.Reason != "not_found" {
		t.Errorf("Reason = %q, want %q", result.Reason, "not_found")
	}
}

func TestResolveRelativeEscapesRepo(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "app.ts")

	r := New(root)
	result, _ := r.Resolve("app.ts", model.ImportFact{
		Specifier: "../../outside", ImportType: "relative",
	}, root)
	if !result.External {
		t.Fatal("expected external for repo escape")
	}
	if result.Reason != "escapes_repository" {
		t.Errorf("Reason = %q, want %q", result.Reason, "escapes_repository")
	}
}

func TestResolveRelativePriorityTSoverJS(t *testing.T) {
	// When both .ts and .js exist, .ts should win.
	root := t.TempDir()
	createFile(t, root, "src/mod.ts")
	createFile(t, root, "src/mod.js")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./mod", ImportType: "relative",
	}, root)
	if result.ResolvedPath != "src/mod.ts" {
		t.Errorf("got %q, want %q (.ts should win over .js)", result.ResolvedPath, "src/mod.ts")
	}
}

func TestResolveRelativeFileOverIndex(t *testing.T) {
	// Flat file should resolve before index directory.
	root := t.TempDir()
	createFile(t, root, "src/utils.ts")
	createFile(t, root, "src/utils/index.ts")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./utils", ImportType: "relative",
	}, root)
	if result.ResolvedPath != "src/utils.ts" {
		t.Errorf("got %q, want %q (file should win over index)", result.ResolvedPath, "src/utils.ts")
	}
}

// --- tsconfig paths tests ---

func TestResolveTsconfigPathsWildcard(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/lib/helpers.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/lib/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/helpers.ts")
	}
	if result.ResolutionMethod != "tsconfig_paths" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "tsconfig_paths")
	}
}

func TestResolveTsconfigPathsExact(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/special/config.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@config": ["src/special/config"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@config", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/special/config.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/special/config.ts")
	}
}

func TestResolveTsconfigPathsBaseUrl(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/utils/format.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": "src",
			"paths": {
				"@utils/*": ["utils/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@utils/format", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/utils/format.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/utils/format.ts")
	}
}

func TestResolveTsconfigPathsIndex(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/lib/index.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib": ["src/lib"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/lib/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/index.ts")
	}
}

func TestResolveTsconfigPathsNoMatch(t *testing.T) {
	root := t.TempDir()
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@other/thing", ImportType: "absolute",
	}, root)
	if !result.External {
		t.Fatal("expected external")
	}
	if result.Reason != "external" {
		t.Errorf("Reason = %q, want %q", result.Reason, "external")
	}
}

func TestResolveTsconfigPathsLongestWins(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/core/utils/format.ts")
	createFile(t, root, "src/shared/utils/format.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@app/*": ["src/shared/*"],
				"@app/core/*": ["src/core/*"]
			}
		}
	}`)

	r := New(root)
	// @app/core/utils/format should match the longer @app/core/* pattern.
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@app/core/utils/format", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/core/utils/format.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/core/utils/format.ts")
	}
}

func TestResolveTsconfigNestedDir(t *testing.T) {
	// tsconfig.json in a subdirectory — resolver walks up from source file.
	root := t.TempDir()
	createFile(t, root, "packages/app/src/lib/utils.ts")
	createFileContent(t, root, "packages/app/tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("packages/app/src/main.ts", model.ImportFact{
		Specifier: "@lib/utils", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "packages/app/src/lib/utils.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/app/src/lib/utils.ts")
	}
}

func TestResolveTsconfigWithComments(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/lib/api.ts")
	createFileContent(t, root, "tsconfig.json", `{
		// This is a comment
		"compilerOptions": {
			"baseUrl": ".", // inline comment
			"paths": {
				"@lib/*": ["src/lib/*"], // trailing comma
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/api", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — tsconfig with comments should parse")
	}
	if result.ResolvedPath != "src/lib/api.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/api.ts")
	}
}

// --- baseUrl bare import tests ---

func TestResolveBareImportBaseUrl(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "apps/studio/components/layouts/DatabaseLayout.tsx")
	createFile(t, root, "apps/studio/types.ts")
	createFile(t, root, "apps/studio/types/index.ts")
	createFileContent(t, root, "apps/studio/tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@/*": ["./*"]
			}
		}
	}`)

	r := New(root)

	// Bare specifier with nested path should resolve via baseUrl.
	result, err := r.Resolve("apps/studio/src/app.ts", model.ImportFact{
		Specifier: "components/layouts/DatabaseLayout", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal — bare import should resolve via baseUrl")
	}
	if result.ResolvedPath != "apps/studio/components/layouts/DatabaseLayout.tsx" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "apps/studio/components/layouts/DatabaseLayout.tsx")
	}
	if result.ResolutionMethod != "tsconfig_baseUrl" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "tsconfig_baseUrl")
	}

	// Bare "types" should resolve to types.ts (file wins over directory index).
	result, err = r.Resolve("apps/studio/src/app.ts", model.ImportFact{
		Specifier: "types", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal — bare 'types' should resolve via baseUrl")
	}
	if result.ResolvedPath != "apps/studio/types.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "apps/studio/types.ts")
	}
}

func TestResolveBareImportBaseUrlIndexFallback(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "apps/studio/types/index.ts")
	createFileContent(t, root, "apps/studio/tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": "."
		}
	}`)

	r := New(root)

	// When only types/index.ts exists (no types.ts), bare "types" should resolve to index.
	result, _ := r.Resolve("apps/studio/src/app.ts", model.ImportFact{
		Specifier: "types", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — bare 'types' should resolve to types/index.ts via baseUrl")
	}
	if result.ResolvedPath != "apps/studio/types/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "apps/studio/types/index.ts")
	}
}

func TestResolveBareImportBaseUrlNoMatch(t *testing.T) {
	root := t.TempDir()
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": "."
		}
	}`)

	r := New(root)

	// Bare specifier that doesn't match any file should still be external.
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "react", ImportType: "absolute",
	}, root)
	if !result.External {
		t.Fatal("expected external — 'react' should not resolve via baseUrl")
	}
}

// --- External classification tests ---

func TestResolveExternalBareSpecifier(t *testing.T) {
	root := t.TempDir()

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "react", ImportType: "absolute",
	}, root)
	if !result.External {
		t.Fatal("expected external for bare specifier")
	}
	if result.Reason != "external" {
		t.Errorf("Reason = %q, want %q", result.Reason, "external")
	}
}

func TestResolveExternalScopedPackage(t *testing.T) {
	root := t.TempDir()

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@types/react", ImportType: "absolute",
	}, root)
	if !result.External {
		t.Fatal("expected external for scoped package")
	}
}

func TestResolveExternalNodeBuiltin(t *testing.T) {
	root := t.TempDir()

	r := New(root)
	for _, spec := range []string{"fs", "path", "node:fs", "node:path"} {
		result, _ := r.Resolve("src/app.ts", model.ImportFact{
			Specifier: spec, ImportType: "absolute",
		}, root)
		if !result.External {
			t.Errorf("expected external for %q", spec)
		}
	}
}

// --- tsconfig cache test ---

func TestTsconfigCaching(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/a.ts")
	createFile(t, root, "src/sub/b.ts")
	createFile(t, root, "src/lib/c.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": { "@lib/*": ["src/lib/*"] }
		}
	}`)

	r := New(root)

	// First call populates cache.
	r.Resolve("src/a.ts", model.ImportFact{Specifier: "@lib/c", ImportType: "absolute"}, root)
	// Second call from different dir should use cached tsconfig.
	result, _ := r.Resolve("src/sub/b.ts", model.ImportFact{Specifier: "@lib/c", ImportType: "absolute"}, root)
	if result.External {
		t.Fatal("expected internal — caching should not break resolution")
	}
	if result.ResolvedPath != "src/lib/c.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/c.ts")
	}
}

// --- Extension candidate order test ---

func TestExtensionPriorityOrder(t *testing.T) {
	// Verify .ts wins over .tsx, which wins over .mts, etc.
	root := t.TempDir()
	createFile(t, root, "src/mod.tsx") // only .tsx exists

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./mod", ImportType: "relative",
	}, root)
	if result.ResolvedPath != "src/mod.tsx" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/mod.tsx")
	}
}

// --- JS extension remap with index fallback ---

func TestResolveJSExtensionIndexFallback(t *testing.T) {
	// import './utils.js' where utils is actually a directory with index.ts
	root := t.TempDir()
	createFile(t, root, "src/utils/index.ts")

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./utils.js", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/utils/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/utils/index.ts")
	}
}

// --- No tsconfig at all ---

func TestResolveNoTsconfig(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/utils.ts")

	r := New(root)
	// Relative should still work without tsconfig.
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "./utils", ImportType: "relative",
	}, root)
	if result.External {
		t.Fatal("expected internal — relative resolution should work without tsconfig")
	}
	// Absolute should be external without tsconfig paths.
	result, _ = r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/utils", ImportType: "absolute",
	}, root)
	if !result.External {
		t.Fatal("expected external — no tsconfig paths to match")
	}
}

// --- stripJSONComments tests ---

// --- tsconfig extends tests ---

func TestResolveTsconfigExtendsPaths(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/lib/helpers.ts")
	createFileContent(t, root, "tsconfig.base.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.base.json"
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — paths should be inherited from parent tsconfig")
	}
	if result.ResolvedPath != "src/lib/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/helpers.ts")
	}
}

func TestResolveTsconfigExtendsBaseUrl(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/utils/format.ts")
	createFileContent(t, root, "tsconfig.base.json", `{
		"compilerOptions": {
			"baseUrl": "src"
		}
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.base.json",
		"compilerOptions": {
			"paths": {
				"@utils/*": ["utils/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@utils/format", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — baseUrl should be inherited from parent")
	}
	if result.ResolvedPath != "src/utils/format.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/utils/format.ts")
	}
}

func TestResolveTsconfigExtendsChildOverridesPaths(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/v2/helpers.ts")
	createFileContent(t, root, "tsconfig.base.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.base.json",
		"compilerOptions": {
			"paths": {
				"@lib/*": ["src/v2/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal")
	}
	if result.ResolvedPath != "src/v2/helpers.ts" {
		t.Errorf("got %q, want %q — child paths should override parent", result.ResolvedPath, "src/v2/helpers.ts")
	}
}

func TestResolveTsconfigExtendsChildOverridesBaseUrl(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "app/lib/helpers.ts")
	createFileContent(t, root, "tsconfig.base.json", `{
		"compilerOptions": {
			"baseUrl": "src",
			"paths": {
				"@lib/*": ["lib/*"]
			}
		}
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.base.json",
		"compilerOptions": {
			"baseUrl": "app"
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/main.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — child baseUrl + inherited paths")
	}
	if result.ResolvedPath != "app/lib/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "app/lib/helpers.ts")
	}
}

func TestResolveTsconfigExtendsChain(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/core/utils.ts")
	createFileContent(t, root, "tsconfig.grandparent.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@core/*": ["src/core/*"]
			}
		}
	}`)
	createFileContent(t, root, "tsconfig.base.json", `{
		"extends": "./tsconfig.grandparent.json"
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.base.json"
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@core/utils", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — paths should propagate through 3-level chain")
	}
	if result.ResolvedPath != "src/core/utils.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/core/utils.ts")
	}
}

func TestResolveTsconfigExtendsCircular(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/utils.ts")
	createFileContent(t, root, "tsconfig.a.json", `{
		"extends": "./tsconfig.b.json",
		"compilerOptions": {
			"baseUrl": ".",
			"paths": { "@/*": ["src/*"] }
		}
	}`)
	createFileContent(t, root, "tsconfig.b.json", `{
		"extends": "./tsconfig.a.json"
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.a.json"
	}`)

	r := New(root)
	// Should not hang — circular extends should be detected.
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@/utils", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — circular extends should be handled gracefully")
	}
}

func TestResolveTsconfigExtendsMissingParent(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/lib/helpers.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.nonexistent.json",
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — child's own config should still work when parent is missing")
	}
	if result.ResolvedPath != "src/lib/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/helpers.ts")
	}
}

func TestResolveTsconfigExtendsPackageReference(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/lib/helpers.ts")
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "@tsconfig/node18/tsconfig.json",
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — package extends should be skipped, child's config used")
	}
	if result.ResolvedPath != "src/lib/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/helpers.ts")
	}
}

func TestResolveTsconfigExtendsNoJsonExtension(t *testing.T) {
	root := t.TempDir()
	createFile(t, root, "src/lib/helpers.ts")
	createFileContent(t, root, "tsconfig.base.json", `{
		"compilerOptions": {
			"baseUrl": ".",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./tsconfig.base"
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — auto-append .json should resolve parent")
	}
	if result.ResolvedPath != "src/lib/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/helpers.ts")
	}
}

func TestResolveTsconfigExtendsSubdirectoryBaseUrl(t *testing.T) {
	// Parent tsconfig is in a config/ subdirectory with baseUrl: ".." pointing to repo root.
	root := t.TempDir()
	createFile(t, root, "src/lib/helpers.ts")
	createFileContent(t, root, "config/tsconfig.base.json", `{
		"compilerOptions": {
			"baseUrl": "..",
			"paths": {
				"@lib/*": ["src/lib/*"]
			}
		}
	}`)
	createFileContent(t, root, "tsconfig.json", `{
		"extends": "./config/tsconfig.base.json"
	}`)

	r := New(root)
	result, _ := r.Resolve("src/app.ts", model.ImportFact{
		Specifier: "@lib/helpers", ImportType: "absolute",
	}, root)
	if result.External {
		t.Fatal("expected internal — parent's baseUrl should be relative to parent's directory")
	}
	if result.ResolvedPath != "src/lib/helpers.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "src/lib/helpers.ts")
	}
}

// --- stripJSONComments tests ---

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no comments",
			input: `{"a": 1}`,
			want:  `{"a": 1}`,
		},
		{
			name:  "single-line comment",
			input: "{\n// comment\n\"a\": 1\n}",
			want:  "{\n\n\"a\": 1\n}",
		},
		{
			name:  "inline comment",
			input: `{"a": 1 // comment` + "\n}",
			want:  `{"a": 1 ` + "\n}",
		},
		{
			name:  "multi-line comment",
			input: `{"a": /* comment */ 1}`,
			want:  `{"a":  1}`,
		},
		{
			name:  "trailing comma object",
			input: `{"a": 1, "b": 2, }`,
			want:  `{"a": 1, "b": 2 }`,
		},
		{
			name:  "trailing comma array",
			input: `[1, 2, ]`,
			want:  `[1, 2 ]`,
		},
		{
			name:  "comment-like string preserved",
			input: `{"url": "http://example.com"}`,
			want:  `{"url": "http://example.com"}`,
		},
		{
			name:  "comment then trailing comma",
			input: "{\"a\": 1, // comment\n}",
			want:  "{\"a\": 1 \n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(stripJSONComments([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("stripJSONComments(%q) =\n  %q\nwant:\n  %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- I-012: Bare @alias tsconfig paths with extends ---

func TestResolveTsconfigExtendsChildPathsKeepChildBaseDir(t *testing.T) {
	// I-012: When child tsconfig defines its own paths but no baseUrl,
	// and parent has no baseUrl either, child's paths should resolve
	// relative to the child's directory, not the parent's.
	root := t.TempDir()
	createFile(t, root, "packages/order/src/models/index.ts")
	createFileContent(t, root, "_tsconfig.base.json", `{
		"compilerOptions": {
			"target": "ES2021"
		}
	}`)
	createFileContent(t, root, "packages/order/tsconfig.json", `{
		"extends": "../../_tsconfig.base.json",
		"compilerOptions": {
			"paths": {
				"@models": ["./src/models"],
				"@models/*": ["./src/models/*"]
			}
		}
	}`)

	r := New(root)

	// Exact alias: import { Order } from "@models"
	result, err := r.Resolve("packages/order/src/services/service.ts", model.ImportFact{
		Specifier: "@models", ImportType: "absolute",
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.External {
		t.Fatal("expected internal — @models should resolve via child's tsconfig paths")
	}
	if result.ResolvedPath != "packages/order/src/models/index.ts" {
		t.Errorf("got %q, want %q", result.ResolvedPath, "packages/order/src/models/index.ts")
	}
	if result.ResolutionMethod != "tsconfig_paths" {
		t.Errorf("method = %q, want %q", result.ResolutionMethod, "tsconfig_paths")
	}
}
