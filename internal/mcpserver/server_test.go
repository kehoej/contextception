package mcpserver

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/model"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// createTestProject creates a minimal Python project for testing.
func createTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := map[string]string{
		"setup.py":           "# setup\n",
		"myapp/__init__.py":  "from .models import User\n",
		"myapp/main.py":      "from .models import User\nfrom .utils import helper\nimport os\n",
		"myapp/models.py":    "from .base import BaseModel\n\nclass User(BaseModel):\n    pass\n",
		"myapp/utils.py":     "from . import models\n",
		"myapp/base.py":      "# no imports\n",
		"tests/test_main.py": "from myapp.main import something\n",
	}

	for relPath, content := range files {
		abs := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

// startTestServer creates a ContextServer and runs it on an in-memory transport,
// returning a connected client session for calling tools.
func startTestServer(t *testing.T, repoRoot string) *mcp.ClientSession {
	t.Helper()

	cs := New(repoRoot)
	t.Cleanup(func() { cs.Close() })

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	server := mcp.NewServer(
		&mcp.Implementation{Name: "contextception-test", Version: "test"},
		nil,
	)
	cs.registerTools(server)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Start server in background.
	go server.Run(ctx, serverTransport)

	// Connect client.
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	return session
}

func TestGetContextAutoIndexes(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("get_context returned error: %s", textContent(t, result))
	}

	// Parse the analysis output.
	var output model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing output: %v\nraw: %s", err, textContent(t, result))
	}

	if output.SchemaVersion != "3.2" {
		t.Errorf("schema_version = %q, want %q", output.SchemaVersion, "3.2")
	}
	if output.Subject != "myapp/main.py" {
		t.Errorf("subject = %q, want %q", output.Subject, "myapp/main.py")
	}
	// Subject should NOT be in must_read.
	for _, entry := range output.MustRead {
		if entry.File == "myapp/main.py" {
			t.Error("subject should not be in must_read")
		}
	}
}

func TestGetContextMissingFile(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": ""},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Error("expected IsError for empty file parameter")
	}
}

func TestGetContextFileNotFound(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// First index so the auto-index doesn't mask the error.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "nonexistent.py"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Error("expected IsError for file not in index")
	}
}

func TestIndex(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("index returned error: %s", textContent(t, result))
	}

	var indexOut indexResult
	if err := json.Unmarshal([]byte(textContent(t, result)), &indexOut); err != nil {
		t.Fatalf("parsing output: %v", err)
	}

	if indexOut.Files == 0 {
		t.Error("expected files > 0 after indexing")
	}
	if indexOut.Edges == 0 {
		t.Error("expected edges > 0 after indexing")
	}
	if indexOut.DurationMs <= 0 {
		t.Error("expected duration_ms > 0")
	}
}

func TestStatusNoIndex(t *testing.T) {
	root := t.TempDir()
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("status returned error: %s", textContent(t, result))
	}

	var status statusResult
	if err := json.Unmarshal([]byte(textContent(t, result)), &status); err != nil {
		t.Fatalf("parsing output: %v", err)
	}

	if status.Indexed {
		t.Error("expected indexed=false for empty directory")
	}
}

func TestStatusAfterIndex(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now check status.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("status returned error: %s", textContent(t, result))
	}

	var status statusResult
	if err := json.Unmarshal([]byte(textContent(t, result)), &status); err != nil {
		t.Fatalf("parsing output: %v", err)
	}

	if !status.Indexed {
		t.Error("expected indexed=true after indexing")
	}
	if status.Files == 0 {
		t.Error("expected files > 0")
	}
	if status.Edges == 0 {
		t.Error("expected edges > 0")
	}
	if status.LastIndexedAt == "" {
		t.Error("expected last_indexed_at to be set")
	}
	if status.UnresolvedByReason == nil {
		t.Error("expected unresolved_by_reason to be present")
	}
	if len(status.UnresolvedByReason) == 0 {
		t.Error("expected unresolved_by_reason to be non-empty after indexing a project with external imports")
	}
}

// textContent extracts the text from the first TextContent in a CallToolResult.
func textContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// --- Integration test helpers ---

func mcpTestdataDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	dir := filepath.Join(repoRoot, "testdata")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("testdata dir not found at %s: %v", dir, err)
	}
	return dir
}

func mcpCopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func mcpSetupGitRepo(t *testing.T, srcDir string) string {
	t.Helper()
	dst := t.TempDir()
	if err := mcpCopyDir(srcDir, dst); err != nil {
		t.Fatalf("copying %s: %v", srcDir, err)
	}
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init", "--allow-empty"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dst
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2024-01-01T00:00:00+00:00", "GIT_COMMITTER_DATE=2024-01-01T00:00:00+00:00")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dst
}

// --- Workstream 2: Integration tests ---

func TestGetContextWithRealProject(t *testing.T) {
	td := mcpTestdataDir(t)
	srcDir := filepath.Join(td, "python_small")
	if _, err := os.Stat(srcDir); err != nil {
		t.Skipf("python_small not found: %v", err)
	}

	repoRoot := mcpSetupGitRepo(t, srcDir)
	session := startTestServer(t, repoRoot)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "mylib/api.py"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_context error: %s", textContent(t, result))
	}

	var output model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// Flatten must_read for containment checks.
	mustReadSet := map[string]bool{}
	for _, entry := range output.MustRead {
		mustReadSet[entry.File] = true
	}
	for _, want := range []string{"mylib/models.py", "mylib/utils.py"} {
		if !mustReadSet[want] {
			t.Errorf("must_read missing %q", want)
		}
	}

	for _, forbidden := range []string{"tests/test_api.py", "docs/tutorial.py", "examples/basic.py"} {
		if mustReadSet[forbidden] {
			t.Errorf("must_read should not contain %q", forbidden)
		}
	}
}

func TestGetContextPathResolution(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	indexResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if indexResult.IsError {
		t.Fatalf("index failed: %s", textContent(t, indexResult))
	}

	tests := []struct {
		name    string
		input   string
		wantOK  bool
		subject string
	}{
		{"relative", "myapp/main.py", true, "myapp/main.py"},
		{"dot_prefix", "./myapp/main.py", true, "myapp/main.py"},
		{"absolute", filepath.Join(root, "myapp/main.py"), true, "myapp/main.py"},
		{"outside_repo", "/etc/passwd", false, ""},
		{"does_not_exist", "does/not/exist.py", false, ""},
		{"empty_string", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "get_context",
				Arguments: map[string]any{"file": tt.input},
			})
			if err != nil {
				t.Fatal(err)
			}

			if tt.wantOK {
				if result.IsError {
					t.Fatalf("expected success for %q, got error: %s", tt.input, textContent(t, result))
				}
				var output model.AnalysisOutput
				if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
					t.Fatalf("parsing: %v", err)
				}
				if output.Subject != tt.subject {
					t.Errorf("subject = %q, want %q", output.Subject, tt.subject)
				}
			} else {
				if !result.IsError {
					t.Errorf("expected error for %q, got success", tt.input)
				}
			}
		})
	}
}

func TestGetContextAfterReindex(t *testing.T) {
	root := createTestProject(t)

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init", "--allow-empty"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	session := startTestServer(t, root)

	// Step 1: Index and analyze.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result1, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result1.IsError {
		t.Fatalf("first analyze error: %s", textContent(t, result1))
	}

	var output1 model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result1)), &output1); err != nil {
		t.Fatal(err)
	}

	// new_consumer.py should not be in output yet.
	for _, entry := range output1.MustRead {
		if entry.File == "myapp/new_consumer.py" {
			t.Fatal("new_consumer.py should not be in must_read before adding it")
		}
	}

	// Step 2: Write new file that imports main.
	newFile := filepath.Join(root, "myapp", "new_consumer.py")
	if err := os.WriteFile(newFile, []byte("from . import main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"git", "add", "myapp/new_consumer.py"},
		{"git", "commit", "-m", "add new_consumer"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Step 3: Re-index.
	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Step 4: Re-analyze.
	result2, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result2.IsError {
		t.Fatalf("second analyze error: %s", textContent(t, result2))
	}

	var output2 model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result2)), &output2); err != nil {
		t.Fatal(err)
	}

	// new_consumer.py should be in must_read, related, or likely_modify.
	found := false
	for _, entry := range output2.MustRead {
		if entry.File == "myapp/new_consumer.py" {
			found = true
		}
	}
	for key, entries := range output2.Related {
		for _, e := range entries {
			fullPath := key + "/" + e.File
			if key == "." {
				fullPath = e.File
			}
			if fullPath == "myapp/new_consumer.py" {
				found = true
			}
		}
	}
	for key, entries := range output2.LikelyModify {
		for _, e := range entries {
			fullPath := key + "/" + e.File
			if key == "." {
				fullPath = e.File
			}
			if fullPath == "myapp/new_consumer.py" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("new_consumer.py not found after re-index")
	}
}

// TestGetContextOutputSchema validates JSON structure of the analysis output.
func TestGetContextOutputSchema(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_context error: %s", textContent(t, result))
	}

	raw := textContent(t, result)

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}

	// Check required top-level keys (v3.2 schema).
	requiredKeys := []string{"schema_version", "subject", "confidence", "must_read", "likely_modify", "related", "tests", "external", "blast_radius"}
	for _, key := range requiredKeys {
		if _, ok := doc[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}

	// must_read should be an array, not a map.
	mrStr := strings.TrimSpace(string(doc["must_read"]))
	if mrStr == "null" {
		t.Error("must_read is null, expected []")
	}
	if !strings.HasPrefix(mrStr, "[") {
		t.Errorf("must_read should be an array, got: %.50s", mrStr)
	}

	// likely_modify, related should be objects (maps).
	mapKeys := []string{"likely_modify", "related"}
	for _, key := range mapKeys {
		s := strings.TrimSpace(string(doc[key]))
		if s == "null" {
			t.Errorf("%s is null, expected {}", key)
		}
		if !strings.HasPrefix(s, "{") {
			t.Errorf("%s should be an object, got: %.50s", key, s)
		}
	}

	// tests should be an array.
	testsStr := strings.TrimSpace(string(doc["tests"]))
	if testsStr == "null" {
		t.Error("tests is null, expected []")
	}
	if !strings.HasPrefix(testsStr, "[") {
		t.Errorf("tests should be an array, got: %.50s", testsStr)
	}

	// external should be [] not null.
	extStr := strings.TrimSpace(string(doc["external"]))
	if extStr == "null" {
		t.Error("external is null, expected []")
	}

	// tests entries should be objects with "file" and "direct" fields.
	var testEntries []map[string]any
	if err := json.Unmarshal(doc["tests"], &testEntries); err != nil {
		// May be empty array, that's fine.
		if testsStr != "[]" {
			t.Errorf("tests is not a valid array of objects: %v", err)
		}
	}
	for _, entry := range testEntries {
		if _, ok := entry["file"]; !ok {
			t.Error("test entry missing 'file' field")
		}
		if _, ok := entry["direct"]; !ok {
			t.Error("test entry missing 'direct' field")
		}
	}

	// blast_radius should be an object.
	brStr := strings.TrimSpace(string(doc["blast_radius"]))
	if brStr == "null" {
		t.Error("blast_radius is null, expected object")
	}
	if !strings.HasPrefix(brStr, "{") {
		t.Errorf("blast_radius should be an object, got: %.50s", brStr)
	}
}

// === Test 2: Configurable Caps MCP Integration ===

func TestGetContextCustomCaps(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_context",
		Arguments: map[string]any{
			"file":          "myapp/main.py",
			"max_must_read": 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_context error: %s", textContent(t, result))
	}

	var output model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if len(output.MustRead) > 2 {
		t.Errorf("must_read = %d, want <= 2 with max_must_read=2", len(output.MustRead))
	}
}

// === Test 4: Multi-file MCP Integration ===

func TestGetContextMultiFileArray(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_context",
		Arguments: map[string]any{
			"file": []any{"myapp/main.py", "myapp/models.py"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_context error: %s", textContent(t, result))
	}

	var output model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// Subject should be comma-separated.
	if output.Subject != "myapp/main.py, myapp/models.py" {
		t.Errorf("subject = %q, want %q", output.Subject, "myapp/main.py, myapp/models.py")
	}

	// Neither subject should be in must_read.
	for _, entry := range output.MustRead {
		if entry.File == "myapp/main.py" || entry.File == "myapp/models.py" {
			t.Errorf("subject file %q should not be in must_read", entry.File)
		}
	}
}

func TestGetContextMultiFileSingleString(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_context error: %s", textContent(t, result))
	}

	var output model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// Single string input should produce single-file output.
	if output.Subject != "myapp/main.py" {
		t.Errorf("subject = %q, want %q", output.Subject, "myapp/main.py")
	}
}

func TestParseFilesEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		file    any
		wantN   int
		wantErr bool
	}{
		{"nil", nil, 0, true},
		{"empty string", "", 0, true},
		{"single string", "foo.py", 1, false},
		{"array of strings", []any{"a.py", "b.py"}, 2, false},
		{"empty array", []any{}, 0, true},
		{"non-string element", []any{123}, 0, true},
		{"invalid type int", 42, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := getContextInput{File: tt.file}
			files, err := input.parseFiles()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got files=%v", files)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(files) != tt.wantN {
				t.Errorf("got %d files, want %d", len(files), tt.wantN)
			}
		})
	}
}

// === Test 7: Code Signatures MCP Integration ===

func TestGetContextWithSignatures(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get_context",
		Arguments: map[string]any{
			"file":               "myapp/main.py",
			"include_signatures": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_context error: %s", textContent(t, result))
	}

	var output model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// Check that at least one must_read entry with symbols has definitions.
	foundDefs := false
	for _, entry := range output.MustRead {
		if len(entry.Symbols) > 0 && len(entry.Definitions) > 0 {
			foundDefs = true
			break
		}
	}
	if !foundDefs {
		t.Error("expected at least one must_read entry with definitions when include_signatures=true")
	}
}

// === Search / Discovery Tool Tests ===

func TestSearchByPathMCP(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("search returned error: %s", textContent(t, result))
	}

	var output searchOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if output.Query != "main" {
		t.Errorf("query = %q, want %q", output.Query, "main")
	}
	if output.Type != "path" {
		t.Errorf("type = %q, want %q", output.Type, "path")
	}
	if output.Count == 0 {
		t.Error("expected at least 1 result for 'main'")
	}

	// Verify results contain myapp/main.py.
	found := false
	for _, r := range output.Results {
		if r.File == "myapp/main.py" {
			found = true
		}
	}
	if !found {
		t.Error("expected myapp/main.py in search results")
	}
}

func TestSearchBySymbolMCP(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "User", "type": "symbol"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("search returned error: %s", textContent(t, result))
	}

	var output searchOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if output.Type != "symbol" {
		t.Errorf("type = %q, want %q", output.Type, "symbol")
	}
	// User is imported from models in main.py and __init__.py, so models.py should appear.
	found := false
	for _, r := range output.Results {
		if r.File == "myapp/models.py" {
			found = true
		}
	}
	if !found {
		t.Error("expected myapp/models.py in symbol search results for 'User'")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected IsError for empty query")
	}
}

func TestGetEntrypointsMCP(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_entrypoints",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_entrypoints returned error: %s", textContent(t, result))
	}

	var output getEntrypointsOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if output.TotalFiles == 0 {
		t.Error("expected total_files > 0")
	}

	// entrypoints and foundations may be nil — json unmarshals null/missing array to nil.
}

func TestGetStructureMCP(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_structure",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_structure returned error: %s", textContent(t, result))
	}

	var output getStructureOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if output.TotalFiles == 0 {
		t.Error("expected total_files > 0")
	}
	if len(output.Languages) == 0 {
		t.Error("expected at least 1 language")
	}
	if output.Languages["python"] == 0 {
		t.Error("expected python language count > 0")
	}
	if len(output.Directories) == 0 {
		t.Error("expected at least 1 directory")
	}

	// Verify myapp directory is present.
	found := false
	for _, d := range output.Directories {
		if d.Path == "myapp" {
			found = true
			if d.FileCount == 0 {
				t.Error("expected myapp file_count > 0")
			}
			if d.Languages["python"] == 0 {
				t.Error("expected myapp python count > 0")
			}
		}
	}
	if !found {
		t.Error("expected 'myapp' directory in structure output")
	}
}

// === Archetype Tool Tests ===

func TestGetArchetypesMCP(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_archetypes",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_archetypes returned error: %s", textContent(t, result))
	}

	var output getArchetypesOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if output.Count != len(output.Archetypes) {
		t.Errorf("count = %d, but archetypes length = %d", output.Count, len(output.Archetypes))
	}

	// The small test project should match at least the test file archetype.
	foundTest := false
	for _, a := range output.Archetypes {
		if a.Archetype == "" {
			t.Error("archetype entry has empty archetype name")
		}
		if a.File == "" {
			t.Error("archetype entry has empty file")
		}
		if a.Archetype == "Test File" {
			foundTest = true
		}
	}
	if !foundTest {
		t.Error("expected 'Test File' archetype to be detected for tests/test_main.py")
	}
}

func TestGetArchetypesAutoIndexes(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Don't index first — should auto-index.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_archetypes",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_archetypes returned error: %s", textContent(t, result))
	}

	var output getArchetypesOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// Should have detected at least one archetype after auto-indexing.
	if output.Count == 0 {
		t.Error("expected at least 1 archetype after auto-indexing")
	}
}

func TestGetArchetypesWithCategories(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Request only the Test File category.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_archetypes",
		Arguments: map[string]any{"categories": []any{"Test File"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_archetypes returned error: %s", textContent(t, result))
	}

	var output getArchetypesOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	// Should only contain "Test File" archetype(s).
	for _, a := range output.Archetypes {
		if a.Archetype != "Test File" {
			t.Errorf("expected only 'Test File' archetype, got %q", a.Archetype)
		}
	}

	// Should have found the test file.
	if output.Count == 0 {
		t.Error("expected at least 1 result for 'Test File' category")
	}
}

func TestGetArchetypesInvalidCategory(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Index first.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Request an unknown category — should return empty, not error.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_archetypes",
		Arguments: map[string]any{"categories": []any{"Nonexistent Category"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected no error for unknown category, got: %s", textContent(t, result))
	}

	var output getArchetypesOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if output.Count != 0 {
		t.Errorf("expected 0 results for unknown category, got %d", output.Count)
	}
}

// === Analyze Change Tool Tests ===

func TestAnalyzeChangeMCP(t *testing.T) {
	root := createTestProject(t)

	// Set up a git repo with two commits.
	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Get the initial commit SHA.
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	baseSHA, err := cmd.Output()
	if err != nil {
		t.Fatalf("getting base SHA: %v", err)
	}
	base := strings.TrimSpace(string(baseSHA))

	// Modify a file and commit.
	if err := os.WriteFile(filepath.Join(root, "myapp/main.py"),
		[]byte("from .models import User\nfrom .utils import helper\nimport os\n# modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "myapp/main.py"},
		{"git", "commit", "-m", "modify main.py"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	session := startTestServer(t, root)

	// Index first.
	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "index",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "analyze_change",
		Arguments: map[string]any{"base": base, "head": "HEAD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("analyze_change returned error: %s", textContent(t, result))
	}

	var report model.ChangeReport
	if err := json.Unmarshal([]byte(textContent(t, result)), &report); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if report.SchemaVersion == "" {
		t.Error("expected non-empty schema_version")
	}
	if len(report.ChangedFiles) == 0 {
		t.Error("expected at least 1 changed file")
	}
	if report.BlastRadius == nil {
		t.Error("expected blast_radius to be present")
	}

	// Verify myapp/main.py is in the changed files.
	found := false
	for _, f := range report.ChangedFiles {
		if f.File == "myapp/main.py" {
			found = true
			// Verify risk fields are populated.
			if f.RiskScore == 0 {
				t.Error("expected non-zero risk_score for modified myapp/main.py")
			}
			if f.RiskTier == "" {
				t.Error("expected non-empty risk_tier")
			}
			if len(f.RiskFactors) == 0 {
				t.Error("expected non-empty risk_factors")
			}
		}
	}
	if !found {
		t.Error("expected myapp/main.py in changed_files")
	}

	// Verify risk triage is populated.
	if report.RiskTriage == nil {
		t.Error("expected risk_triage to be present")
	}
	if report.AggregateRisk == nil {
		t.Error("expected aggregate_risk to be present")
	} else if report.AggregateRisk.Score == 0 {
		t.Error("expected non-zero aggregate risk score")
	}

	// Verify MCP sorts changed files by risk score descending.
	for i := 1; i < len(report.ChangedFiles); i++ {
		prev := report.ChangedFiles[i-1].RiskScore
		curr := report.ChangedFiles[i].RiskScore
		if prev < curr {
			t.Errorf("changed_files not sorted by risk: file[%d]=%d < file[%d]=%d",
				i-1, prev, i, curr)
		}
	}
}

func TestAnalyzeChangeAutoDetect(t *testing.T) {
	root := createTestProject(t)

	// Set up git with main branch.
	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create feature branch, modify, commit.
	for _, args := range [][]string{
		{"git", "checkout", "-b", "feature"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "myapp/utils.py"),
		[]byte("from . import models\n# updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "myapp/utils.py"},
		{"git", "commit", "-m", "update utils"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	session := startTestServer(t, root)

	// Call with no base/head — should auto-detect.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "analyze_change",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("analyze_change auto-detect returned error: %s", textContent(t, result))
	}

	var report model.ChangeReport
	if err := json.Unmarshal([]byte(textContent(t, result)), &report); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if len(report.ChangedFiles) == 0 {
		t.Error("expected at least 1 changed file with auto-detect")
	}
}

func TestAnalyzeChangeNoChanges(t *testing.T) {
	root := createTestProject(t)

	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	session := startTestServer(t, root)

	// HEAD..HEAD should have no changes.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "analyze_change",
		Arguments: map[string]any{"base": "HEAD", "head": "HEAD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("analyze_change returned error: %s", textContent(t, result))
	}

	var report model.ChangeReport
	if err := json.Unmarshal([]byte(textContent(t, result)), &report); err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if len(report.ChangedFiles) != 0 {
		t.Errorf("expected 0 changed files for HEAD..HEAD, got %d", len(report.ChangedFiles))
	}
}

// === Rate Context Tool Tests ===

func TestRateContextMCP(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// First call get_context so there's a usage_log entry to link to.
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now rate the context.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "rate_context",
		Arguments: map[string]any{
			"file":              "myapp/main.py",
			"usefulness":        4,
			"useful_files":      []any{"myapp/models.py", "myapp/utils.py"},
			"unnecessary_files": []any{"myapp/base.py"},
			"missing_files":     []any{},
			"modified_files":    []any{"myapp/models.py"},
			"notes":             "Good context, base.py wasn't needed",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("rate_context returned error: %s", textContent(t, result))
	}

	text := textContent(t, result)
	if !strings.Contains(text, "Feedback recorded") {
		t.Errorf("expected 'Feedback recorded' in response, got: %s", text)
	}
}

func TestRateContextValidation(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// Missing file.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "rate_context",
		Arguments: map[string]any{"usefulness": 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing file parameter")
	}

	// Invalid usefulness.
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "rate_context",
		Arguments: map[string]any{"file": "test.py", "usefulness": 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for usefulness=0")
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "rate_context",
		Arguments: map[string]any{"file": "test.py", "usefulness": 6},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for usefulness=6")
	}
}

func TestGetContextOmitExternal(t *testing.T) {
	root := createTestProject(t)
	session := startTestServer(t, root)

	// First verify external is non-empty without the flag.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_context error: %s", textContent(t, result))
	}

	var output model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result)), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.External) == 0 {
		t.Fatal("expected non-empty external without omit_external")
	}

	// Now with omit_external=true.
	result2, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_context",
		Arguments: map[string]any{"file": "myapp/main.py", "omit_external": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result2.IsError {
		t.Fatalf("get_context error with omit_external: %s", textContent(t, result2))
	}

	var output2 model.AnalysisOutput
	if err := json.Unmarshal([]byte(textContent(t, result2)), &output2); err != nil {
		t.Fatal(err)
	}
	if len(output2.External) != 0 {
		t.Errorf("expected empty external with omit_external=true, got %v", output2.External)
	}
}
