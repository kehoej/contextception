package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSession(t *testing.T) {
	dir := t.TempDir()
	repoRoot := "/Users/test/myproject"

	// Create a synthetic session JSONL file.
	entries := []map[string]any{
		{
			"type":      "assistant",
			"timestamp": "2026-04-11T10:00:00.000Z",
			"message": map[string]any{
				"content": []map[string]any{
					{
						"type": "tool_use",
						"name": "Edit",
						"input": map[string]any{
							"file_path": "/Users/test/myproject/src/main.py",
						},
					},
				},
			},
		},
		{
			"type":      "assistant",
			"timestamp": "2026-04-11T10:01:00.000Z",
			"message": map[string]any{
				"content": []map[string]any{
					{
						"type": "tool_use",
						"name": "mcp__contextception__get_context",
						"input": map[string]any{
							"file": "src/auth.py",
						},
					},
				},
			},
		},
		{
			"type":      "assistant",
			"timestamp": "2026-04-11T10:02:00.000Z",
			"message": map[string]any{
				"content": []map[string]any{
					{
						"type": "tool_use",
						"name": "Write",
						"input": map[string]any{
							"file_path": "/Users/test/myproject/src/new.py",
						},
					},
				},
			},
		},
		{
			"type":      "assistant",
			"timestamp": "2026-04-11T10:03:00.000Z",
			"message": map[string]any{
				"content": []map[string]any{
					{
						"type": "tool_use",
						"name": "Read",
						"input": map[string]any{
							"file_path": "/Users/test/myproject/src/main.py",
						},
					},
				},
			},
		},
		// Edit outside repo — should be ignored.
		{
			"type":      "assistant",
			"timestamp": "2026-04-11T10:04:00.000Z",
			"message": map[string]any{
				"content": []map[string]any{
					{
						"type": "tool_use",
						"name": "Edit",
						"input": map[string]any{
							"file_path": "/Users/test/other-repo/file.py",
						},
					},
				},
			},
		},
	}

	sessionFile := filepath.Join(dir, "test-session.jsonl")
	f, err := os.Create(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, _ := json.Marshal(entry)
		f.Write(data)
		f.WriteString("\n")
	}
	f.Close()

	edits, contexts, err := ParseSession(sessionFile, repoRoot)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}

	// Should have 2 edits (Edit + Write, not Read, not outside repo).
	if len(edits) != 2 {
		t.Errorf("edits = %d, want 2", len(edits))
	}
	if len(edits) >= 1 && edits[0].FilePath != "src/main.py" {
		t.Errorf("edit[0] = %q, want %q", edits[0].FilePath, "src/main.py")
	}
	if len(edits) >= 2 && edits[1].FilePath != "src/new.py" {
		t.Errorf("edit[1] = %q, want %q", edits[1].FilePath, "src/new.py")
	}

	// Should have 1 context call.
	if len(contexts) != 1 {
		t.Errorf("contexts = %d, want 1", len(contexts))
	}
	if len(contexts) >= 1 && contexts[0].FilePath != "src/auth.py" {
		t.Errorf("context[0] = %q, want %q", contexts[0].FilePath, "src/auth.py")
	}
}

func TestEncodedPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/kehoe/Repositories/myproject", "-Users-kehoe-Repositories-myproject"},
		{"/home/user/code", "-home-user-code"},
	}
	for _, tt := range tests {
		got := EncodedPath(tt.input)
		if got != tt.want {
			t.Errorf("EncodedPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"src/main.py", false},
		{"tests/test_main.py", true},
		{"internal/db/db_test.go", true},
		{"src/app.test.ts", true},
		{"src/app.spec.js", true},
		{"src/__tests__/util.ts", true},
		{"src/utils.py", false},
	}
	for _, tt := range tests {
		got := isTestFile(tt.path)
		if got != tt.want {
			t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestProgressBar(t *testing.T) {
	tests := []struct {
		pct  float64
		want string
	}{
		{0, ".........."},
		{50, "|||||....."},
		{100, "||||||||||"},
		{75, "|||||||..."},
	}
	for _, tt := range tests {
		got := progressBar(tt.pct, 10)
		if got != tt.want {
			t.Errorf("progressBar(%.0f) = %q, want %q", tt.pct, got, tt.want)
		}
	}
}

func TestFormatDiscoverSummary(t *testing.T) {
	result := &DiscoverResult{
		SessionsScanned:  5,
		FilesEdited:      10,
		ContextRequested: 6,
		Coverage:         60,
		MissedFiles: []MissedFile{
			{File: "src/main.py", EditCount: 5, ContextCount: 0},
			{File: "tests/test_main.py", EditCount: 3, ContextCount: 0, IsTest: true},
		},
	}

	text := FormatDiscoverSummary(result)

	checks := []string{
		"Sessions scanned:  5",
		"Files edited:      10",
		"60%",
		"src/main.py",
		"Modified 5x",
	}
	for _, check := range checks {
		found := false
		for i := 0; i <= len(text)-len(check); i++ {
			if text[i:i+len(check)] == check {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestFormatSessionSummary(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "abc12345-6789", Date: time.Now(), Edits: 12, Contexts: 8, Coverage: 66.7},
		{ID: "def12345-6789", Date: time.Now().AddDate(0, 0, -1), Edits: 5, Contexts: 5, Coverage: 100},
	}

	text := FormatSessionSummary(sessions)

	if text == "" {
		t.Fatal("expected non-empty output")
	}

	checks := []string{"abc12345", "Today", "12", "67%", "def12345", "100%"}
	for _, check := range checks {
		found := false
		for i := 0; i <= len(text)-len(check); i++ {
			if text[i:i+len(check)] == check {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestFormatEmptyStates(t *testing.T) {
	// No sessions.
	text := FormatDiscoverSummary(&DiscoverResult{})
	if text == "" {
		t.Error("expected non-empty output for empty discover")
	}

	// No sessions with edits.
	text = FormatSessionSummary(nil)
	if text == "" {
		t.Error("expected non-empty output for empty sessions")
	}
}
