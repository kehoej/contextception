// Package extractor defines the interface for language-specific import extraction.
package extractor

import (
	"strings"

	"github.com/kehoej/contextception/internal/model"
)

// MaxSignatureLength is the maximum length for a signature string before truncation.
const MaxSignatureLength = 120

// Extractor extracts import facts from a source file.
type Extractor interface {
	// Language returns the language this extractor handles (e.g., "python").
	Language() string
	// Extensions returns file extensions this extractor handles (e.g., [".py"]).
	Extensions() []string
	// Extract parses a file and returns all import facts.
	Extract(filePath string, content []byte) ([]model.ImportFact, error)
	// Clone returns a new independent instance for concurrent use.
	Clone() Extractor
}

// DefinitionExtractor extracts code signatures (function/class/type definitions)
// for named symbols. Extractors that also implement this interface can provide
// symbol definitions for the --signatures feature.
type DefinitionExtractor interface {
	// ExtractDefinitions returns signature strings for the given symbol names
	// found in the file content. Only the signature line is returned (not the body).
	ExtractDefinitions(content []byte, symbolNames []string) []string
}

// TruncateSig truncates a signature string to maxLen, appending "..." if truncated.
func TruncateSig(sig string, maxLen int) string {
	if len(sig) > maxLen {
		return sig[:maxLen] + "..."
	}
	return sig
}

// TrimBodyOpener removes the opening brace " {" from the end of a signature line.
// Uses substring matching (not character set) to avoid stripping valid trailing characters.
func TrimBodyOpener(sig string) string {
	if idx := strings.Index(sig, " {"); idx > 0 {
		return sig[:idx]
	}
	return sig
}

// BuildNameSet creates a lookup set from a list of symbol names.
// Returns nil if symbolNames is empty.
func BuildNameSet(symbolNames []string) map[string]bool {
	if len(symbolNames) == 0 {
		return nil
	}
	set := make(map[string]bool, len(symbolNames))
	for _, n := range symbolNames {
		set[n] = true
	}
	return set
}
