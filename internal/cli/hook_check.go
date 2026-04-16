package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kehoej/contextception/internal/extractor"
	goextractor "github.com/kehoej/contextception/internal/extractor/golang"
	javaextractor "github.com/kehoej/contextception/internal/extractor/java"
	pyextractor "github.com/kehoej/contextception/internal/extractor/python"
	rustextractor "github.com/kehoej/contextception/internal/extractor/rust"
	tsextractor "github.com/kehoej/contextception/internal/extractor/typescript"
	csharpextractor "github.com/kehoej/contextception/internal/extractor/csharp"
	"github.com/spf13/cobra"
)

func newHookCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook-check",
		Short:  "Check if a file edit should trigger a contextception reminder",
		Long:   "Reads the TOOL_INPUT environment variable (set by Claude Code hooks) and prints a reminder if the file being edited is a supported language. Always exits 0.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHookCheck()
		},
	}
}

func runHookCheck() error {
	input := os.Getenv("TOOL_INPUT")
	if input == "" {
		return nil
	}

	var toolInput struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(input), &toolInput); err != nil {
		return nil
	}
	if toolInput.FilePath == "" {
		return nil
	}

	ext := filepath.Ext(toolInput.FilePath)
	if ext == "" {
		return nil
	}

	if isSupportedExtension(ext) {
		fmt.Printf("CONTEXTCEPTION REMINDER: Before modifying %s, did you call get_context on it? If not, call it now to understand the dependency context before making changes.\n",
			filepath.Base(toolInput.FilePath))
	}

	return nil
}

// supportedExtensions returns the sorted list of file extensions supported by
// all language extractors. This mirrors the logic in extensions.go.
func supportedExtensions() []string {
	extractors := []extractor.Extractor{
		pyextractor.New(),
		tsextractor.New(),
		goextractor.New(),
		javaextractor.New(),
		rustextractor.New(),
		csharpextractor.New(),
	}

	var exts []string
	for _, ext := range extractors {
		exts = append(exts, ext.Extensions()...)
	}
	sort.Strings(exts)
	return exts
}

func isSupportedExtension(ext string) bool {
	for _, supported := range supportedExtensions() {
		if ext == supported {
			return true
		}
	}
	return false
}
