package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kehoej/contextception/internal/extractor"
	goextractor "github.com/kehoej/contextception/internal/extractor/golang"
	pyextractor "github.com/kehoej/contextception/internal/extractor/python"
	tsextractor "github.com/kehoej/contextception/internal/extractor/typescript"
)

// definitionExtractors maps file extensions to DefinitionExtractor implementations.
// Initialized once per program via init().
var definitionExtractors map[string]extractor.DefinitionExtractor

func init() {
	py := pyextractor.New()
	ts := tsextractor.New()
	go_ := goextractor.New()

	definitionExtractors = map[string]extractor.DefinitionExtractor{
		".py": py,
		".ts": ts, ".tsx": ts, ".js": ts, ".jsx": ts, ".mts": ts, ".cts": ts, ".mjs": ts, ".cjs": ts,
		".go": go_,
	}
}

// enrichDefinitions populates the Definitions field on must_read entries
// by reading each file and extracting signatures for tracked symbols.
func enrichDefinitions(repoRoot string, entries []MustReadEntryRef) {
	for i := range entries {
		entry := entries[i]
		if len(entry.Symbols) == 0 {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.File))
		defExtractor, ok := definitionExtractors[ext]
		if !ok {
			continue
		}

		content, err := os.ReadFile(filepath.Join(repoRoot, entry.File))
		if err != nil {
			continue
		}

		defs := defExtractor.ExtractDefinitions(content, entry.Symbols)
		if len(defs) > 0 {
			*entry.Definitions = defs
		}
	}
}

// MustReadEntryRef holds references to a MustReadEntry's fields for enrichment.
type MustReadEntryRef struct {
	File        string
	Symbols     []string
	Definitions *[]string
}
