package analyzer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFixtureValidationHTTPX validates the scoring model against the real-world
// httpx library (https://github.com/encode/httpx). Unlike python_small and
// python_medium, this is an external codebase the algorithm was NOT designed
// against — it tests external validity rather than internal consistency.
//
// This test clones httpx to a temp directory and runs fixtures created from
// developer intuition (what files a developer would need to read before making
// a safe change to each subject file).
//
// Run with: go test ./internal/analyzer/ -run TestFixtureValidationHTTPX -v -timeout 60s
// Skip with: SKIP_HTTPX_VALIDATION=1 go test ./internal/analyzer/ -v
func TestFixtureValidationHTTPX(t *testing.T) {
	if os.Getenv("SKIP_HTTPX_VALIDATION") != "" {
		t.Skip("SKIP_HTTPX_VALIDATION is set")
	}

	// Clone httpx to a temp directory.
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "httpx")

	cmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/encode/httpx.git", repoDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone httpx: %v\n%s", err, out)
	}

	fixtureDir := filepath.Join("..", "..", "testdata", "fixtures", "httpx")
	runFixtureValidation(t, repoDir, fixtureDir)
}
