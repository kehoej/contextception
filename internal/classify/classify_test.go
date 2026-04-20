package classify

import "testing"

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Python patterns
		{"test_models.py", true},
		{"tests/test_api.py", true},
		{"tests/conftest.py", true}, // under tests/ dir
		{"mylib/tests/test_core.py", true},
		{"mylib/api_test.py", true},
		{"mylib/api_test.go", true},
		// TS/JS patterns
		{"app.test.ts", true},
		{"app.test.tsx", true},
		{"app.test.js", true},
		{"app.test.jsx", true},
		{"app.spec.ts", true},
		{"app.spec.tsx", true},
		{"app.spec.js", true},
		{"app.spec.jsx", true},
		{"src/components/Button.test.tsx", true},
		{"src/utils/helpers.spec.ts", true},
		{"__tests__/integration.ts", true},
		{"src/__tests__/unit.tsx", true},
		// C# patterns
		{"Services/FooTest.cs", true},
		{"Services/FooTests.cs", true},
		{"Services/TestFoo.cs", true},
		{"Services/FooSpec.cs", true},
		{"Services/Foo.cs", false},
		{"Services/Contest.cs", false},   // "Contest" contains "test" but isn't a test
		{"Services/Manifest.cs", false},  // ends with "est" but isn't a test
		{"test/EFCore.Tests/FooTest.cs", true},
		{"tests/MyLib.Tests/BarTests.cs", true},
		// Non-test files
		{"mylib/api.py", false},
		{"mylib/models.py", false},
		{"docs/tutorial.py", false},
		{"examples/basic.py", false},
		{"src/app.ts", false},
		{"src/index.tsx", false},
		{"lib/utils.js", false},
	}
	for _, tt := range tests {
		if got := IsTestFile(tt.path); got != tt.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsIgnorable(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"docs/tutorial.py", true},
		{"docs_src/guide.py", true},
		{"examples/basic.py", true},
		{"mylib/docs/internal.py", false},
		{"apps/docs/middleware.ts", false},
		{"apps/docs/lib/constants.ts", false},
		{"conftest.py", true},
		{"tests/conftest.py", true},
		{"mylib/api.py", false},
		{"mylib/models.py", false},
		{"tests/test_api.py", false},
	}
	for _, tt := range tests {
		if got := IsIgnorable(tt.path); got != tt.want {
			t.Errorf("IsIgnorable(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsNonSource(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Test files
		{"tests/test_api.py", true},
		{"test_models.py", true},
		{"app.test.ts", true},
		{"app.spec.tsx", true},
		{"__tests__/unit.ts", true},
		// Ignorable files
		{"docs/tutorial.py", true},
		{"examples/basic.py", true},
		{"conftest.py", true},
		// Source files
		{"mylib/api.py", false},
		{"mylib/models.py", false},
		{"mylib/__init__.py", false},
		{"src/app.ts", false},
		{"lib/utils.js", false},
		{"apps/docs/middleware.ts", false},
	}
	for _, tt := range tests {
		if got := IsNonSource(tt.path); got != tt.want {
			t.Errorf("IsNonSource(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsInitFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"pkg/__init__.py", true},
		{"__init__.py", true},
		{"a/b/__init__.py", true},
		{"main.py", false},
		{"init.py", false},
		{"__init__.txt", false},
	}
	for _, tt := range tests {
		if got := IsInitFile(tt.path); got != tt.want {
			t.Errorf("IsInitFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsBarrelFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Python barrel files
		{"pkg/__init__.py", true},
		{"__init__.py", true},
		{"a/b/__init__.py", true},
		// TypeScript/JavaScript barrel files
		{"src/index.ts", true},
		{"src/index.tsx", true},
		{"src/index.js", true},
		{"src/index.jsx", true},
		{"src/index.mts", true},
		{"src/index.cts", true},
		{"src/index.mjs", true},
		{"src/index.cjs", true},
		{"packages/ui/src/index.ts", true},
		// Non-barrel files
		{"src/main.ts", false},
		{"src/index.py", false},
		{"src/index.go", false},
		{"src/index.css", false},
		{"src/my-index.ts", false},
		{"init.py", false},
	}
	for _, tt := range tests {
		if got := IsBarrelFile(tt.path); got != tt.want {
			t.Errorf("IsBarrelFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestClassifyRole(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		indegree     int
		outdegree    int
		isEntrypoint bool
		isUtility    bool
		stableThresh int
		want         string
	}{
		{"test file", "tests/test_api.py", 0, 5, false, false, 10, "test"},
		{"barrel file", "src/index.ts", 5, 10, false, false, 10, "barrel"},
		{"python barrel", "pkg/__init__.py", 8, 3, false, false, 10, "barrel"},
		{"foundation", "pkg/types.py", 15, 1, false, false, 10, "foundation"},
		{"foundation edge", "pkg/config.py", 10, 2, false, false, 10, "foundation"},
		{"not foundation high outdegree", "pkg/core.py", 15, 3, false, false, 10, ""},
		{"orchestrator", "pkg/app.py", 8, 8, false, false, 10, "orchestrator"},
		{"entrypoint", "cmd/main.go", 0, 3, true, false, 10, "entrypoint"},
		{"utility", "lib/helpers.py", 2, 5, false, true, 10, "utility"},
		{"no role", "pkg/service.py", 3, 3, false, false, 10, ""},
		// Priority: test beats barrel
		{"test in barrel path", "__tests__/index.ts", 0, 0, false, false, 10, "test"},
		// Priority: foundation beats orchestrator
		{"foundation not orchestrator", "pkg/base.py", 10, 0, false, false, 10, "foundation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyRole(tt.path, tt.indegree, tt.outdegree, tt.isEntrypoint, tt.isUtility, tt.stableThresh)
			if got != tt.want {
				t.Errorf("ClassifyRole(%q, %d, %d, %v, %v, %d) = %q, want %q",
					tt.path, tt.indegree, tt.outdegree, tt.isEntrypoint, tt.isUtility, tt.stableThresh, got, tt.want)
			}
		})
	}
}

func TestStripGoPlatformSuffix(t *testing.T) {
	tests := []struct {
		stem string
		want string
	}{
		// Single GOOS suffix
		{"exec_linux", "exec"},
		{"exec_darwin", "exec"},
		{"exec_windows", "exec"},
		{"exec_freebsd", "exec"},
		// GOOS_GOARCH double suffix
		{"exec_darwin_amd64", "exec"},
		{"exec_linux_arm64", "exec"},
		{"exec_windows_386", "exec"},
		// Single GOARCH suffix
		{"asm_amd64", "asm"},
		{"asm_arm64", "asm"},
		// Multi-segment stem with platform suffix
		{"os_exec_linux", "os_exec"},
		{"my_pkg_darwin_amd64", "my_pkg"},
		// No platform suffix — unchanged
		{"handler", "handler"},
		{"models", "models"},
		{"api_client", "api_client"},
		// Single segment — unchanged
		{"linux", "linux"},
		// Empty-result guard: single underscore + platform = don't strip to empty
		// "linux" alone is a single segment with no underscore, returns as-is
		{"_linux", "_linux"}, // Split produces ["", "linux"], join of [:1] = "" → return stem
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			got := StripGoPlatformSuffix(tt.stem)
			if got != tt.want {
				t.Errorf("StripGoPlatformSuffix(%q) = %q, want %q", tt.stem, got, tt.want)
			}
		})
	}
}

func TestTestsRsClassification(t *testing.T) {
	// tests.rs should be classified as a test file.
	if !IsTestFile("src/fs/file/tests.rs") {
		t.Error("IsTestFile(\"src/fs/file/tests.rs\") = false, want true")
	}
	if !IsTestFile("tests.rs") {
		t.Error("IsTestFile(\"tests.rs\") = false, want true")
	}
	// Regular .rs files should not match.
	if IsTestFile("src/fs/file/utils.rs") {
		t.Error("IsTestFile(\"src/fs/file/utils.rs\") = true, want false")
	}
}

func TestHasInlineRustTests(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"with cfg test", "use std::sync;\n\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn it_works() {}\n}", true},
		{"without cfg test", "use std::sync;\n\nfn main() {\n    println!(\"hello\");\n}", false},
		{"empty file", "", false},
		{"cfg test in comment context", "// This file has #[cfg(test)] in it\nfn main() {}", true},
		{"cfg test only", "#[cfg(test)]", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasInlineRustTests([]byte(tt.content))
			if got != tt.want {
				t.Errorf("HasInlineRustTests(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestPathBase(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"mylib/models.py", "models.py"},
		{"a/b/c.py", "c.py"},
		{"flat.py", "flat.py"},
	}
	for _, tt := range tests {
		if got := PathBase(tt.path); got != tt.want {
			t.Errorf("PathBase(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
