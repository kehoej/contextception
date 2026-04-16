// Package classify provides shared file classification predicates used by
// both the indexer (to filter non-source edges) and the analyzer (to route
// files into ignore).
package classify

import (
	"bytes"
	"strings"
)

// IsTestFile returns true if the path looks like a test file.
func IsTestFile(path string) bool {
	base := PathBase(path)

	// Python patterns: test_*.py, *_test.py
	// Note: strings.Contains(base, "_test.") covers _test.py via the dot check.
	if strings.HasPrefix(base, "test_") || strings.Contains(base, "_test.") {
		return true
	}

	// TS/JS patterns: *.test.ts, *.test.tsx, *.test.js, *.test.jsx,
	//                 *.spec.ts, *.spec.tsx, *.spec.js, *.spec.jsx
	for _, suffix := range []string{
		".test.ts", ".test.tsx", ".test.js", ".test.jsx",
		".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx",
	} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}

	// Go patterns: *_test.go
	if strings.HasSuffix(base, "_test.go") {
		return true
	}

	// Java patterns: *Test.java, *Tests.java, Test*.java
	if strings.HasSuffix(base, ".java") {
		stem := strings.TrimSuffix(base, ".java")
		if strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests") || strings.HasPrefix(stem, "Test") {
			return true
		}
	}

	// C# patterns: *Test.cs, *Tests.cs, Test*.cs, *Spec.cs
	if strings.HasSuffix(base, ".cs") {
		stem := strings.TrimSuffix(base, ".cs")
		if strings.HasSuffix(stem, "Test") || strings.HasSuffix(stem, "Tests") ||
			strings.HasPrefix(stem, "Test") || strings.HasSuffix(stem, "Spec") {
			return true
		}
	}

	// Rust patterns: *_test.rs (conventional), tests.rs (separate test module), and tests/ directory (below)
	if strings.HasSuffix(base, "_test.rs") {
		return true
	}
	if base == "tests.rs" {
		return true
	}

	// Under tests/ or __tests__/ directory.
	if strings.HasPrefix(path, "tests/") || strings.Contains(path, "/tests/") {
		return true
	}
	if strings.HasPrefix(path, "__tests__/") || strings.Contains(path, "/__tests__/") {
		return true
	}

	// Java: src/test/ directory (Maven/Gradle convention).
	if strings.HasPrefix(path, "src/test/") || strings.Contains(path, "/src/test/") {
		return true
	}

	// C#: test project directories (*.Tests/, *.Test/).
	if strings.Contains(path, ".Tests/") || strings.Contains(path, ".Test/") {
		return true
	}
	// Also check if path starts with a test project directory.
	for _, part := range strings.Split(path, "/") {
		if strings.HasSuffix(part, ".Tests") || strings.HasSuffix(part, ".Test") {
			return true
		}
	}

	return false
}

// IsIgnorable returns true if the path is a docs/examples/fixture file that
// should be excluded from context (regardless of whether the subject is a test).
func IsIgnorable(path string) bool {
	for _, prefix := range []string{"docs/", "docs_src/", "examples/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	base := PathBase(path)
	return base == "conftest.py"
}

// IsNonSource returns true if the path is a test, doc, example, or other
// non-source file. Non-source files should not contribute to indegree when
// ranking source files.
func IsNonSource(path string) bool {
	return IsTestFile(path) || IsIgnorable(path)
}

// IsInitFile returns true if the path ends with __init__.py.
func IsInitFile(path string) bool {
	return PathBase(path) == "__init__.py"
}

// IsBarrelFile returns true if the path is a barrel/re-export file.
// These are index.{ts,tsx,js,jsx,mts,cts,mjs,cjs} and __init__.py.
func IsBarrelFile(path string) bool {
	base := PathBase(path)
	if base == "__init__.py" {
		return true
	}
	for _, name := range []string{
		"index.ts", "index.tsx", "index.js", "index.jsx",
		"index.mts", "index.cts", "index.mjs", "index.cjs",
	} {
		if base == name {
			return true
		}
	}
	return false
}

// Role classification thresholds.
const (
	// foundationMaxOutdegree is the maximum outdegree for a file to be classified as "foundation"
	// (widely imported but imports very few things itself).
	foundationMaxOutdegree = 2

	// orchestratorMinDegree is the minimum indegree AND outdegree for a file
	// to be classified as "orchestrator" (high connectivity in both directions).
	orchestratorMinDegree = 5
)

// ClassifyRole returns a short role label based on graph signals and path.
// Returns "" if no specific role can be determined.
// Priority order: test → barrel → foundation → orchestrator → entrypoint → utility.
func ClassifyRole(path string, indegree, outdegree int, isEntrypoint, isUtility bool, stableThresh int) string {
	switch {
	case IsTestFile(path):
		return "test"
	case IsBarrelFile(path):
		return "barrel"
	case indegree >= stableThresh && outdegree <= foundationMaxOutdegree:
		return "foundation"
	case outdegree >= orchestratorMinDegree && indegree >= orchestratorMinDegree:
		return "orchestrator"
	case isEntrypoint:
		return "entrypoint"
	case isUtility:
		return "utility"
	default:
		return ""
	}
}

// PathBase returns the filename portion of a path.
func PathBase(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// HasInlineRustTests returns true if the file content contains a #[cfg(test)]
// inline test module. This is the primary Rust test pattern — most Rust source
// files contain their tests inline rather than in separate test files.
func HasInlineRustTests(content []byte) bool {
	return bytes.Contains(content, []byte("#[cfg(test)]"))
}

// StripGoPlatformSuffix removes Go build constraint suffixes (_GOOS, _GOARCH,
// _GOOS_GOARCH) from a filename stem. Returns the original stem if no platform
// suffix is found or if stripping would produce an empty string.
func StripGoPlatformSuffix(stem string) string {
	parts := strings.Split(stem, "_")
	if len(parts) < 2 {
		return stem
	}
	last := parts[len(parts)-1]
	if goosValues[last] || goarchValues[last] {
		// Check GOOS_GOARCH pattern first (strip 2 segments).
		if len(parts) >= 3 {
			secondLast := parts[len(parts)-2]
			if goosValues[secondLast] && goarchValues[last] {
				if result := strings.Join(parts[:len(parts)-2], "_"); result != "" {
					return result
				}
				return stem
			}
		}
		if result := strings.Join(parts[:len(parts)-1], "_"); result != "" {
			return result
		}
	}
	return stem
}

var goosValues = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true,
	"freebsd": true, "hurd": true, "illumos": true, "ios": true,
	"js": true, "linux": true, "nacl": true, "netbsd": true,
	"openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
	"windows": true, "zos": true,
}

var goarchValues = map[string]bool{
	"386": true, "amd64": true, "arm": true, "arm64": true,
	"loong64": true, "mips": true, "mips64": true, "mips64le": true,
	"mipsle": true, "ppc64": true, "ppc64le": true, "riscv64": true,
	"s390x": true, "wasm": true,
}
