package analyzer

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFixtureValidationFlask validates the scoring model against the real-world
// Flask web framework (https://github.com/pallets/flask). Unlike httpx (an HTTP
// client), Flask is a web framework with a src/ layout, subpackages (json/,
// sansio/), and heavy __init__.py barrel re-exports — testing different
// structural patterns.
//
// Run with: go test ./internal/analyzer/ -run TestFixtureValidationFlask -v -timeout 60s
// Skip with: SKIP_FLASK_VALIDATION=1 go test ./internal/analyzer/ -v
func TestFixtureValidationFlask(t *testing.T) {
	if os.Getenv("SKIP_FLASK_VALIDATION") != "" {
		t.Skip("SKIP_FLASK_VALIDATION is set")
	}

	// Clone Flask to a temp directory.
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "flask")

	cmd := exec.Command("git", "clone", "--depth", "1", "https://github.com/pallets/flask.git", repoDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone flask: %v\n%s", err, out)
	}

	fixtureDir := filepath.Join("..", "..", "testdata", "fixtures", "flask")
	runFixtureValidation(t, repoDir, fixtureDir)
}
