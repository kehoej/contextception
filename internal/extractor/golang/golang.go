// Package golang implements import extraction for Go source files.
package golang

import (
	"regexp"
	"strings"

	"github.com/kehoej/contextception/internal/extractor"
	"github.com/kehoej/contextception/internal/model"
)

var (
	// Single import: import "fmt" or import alias "path/to/pkg"
	reSingleImport = regexp.MustCompile(`^import\s+(?:(\w+|\.)\s+)?"([^"]+)"`)
	// Group import start: import (
	reGroupStart = regexp.MustCompile(`^import\s*\(`)
	// Import line inside group: "fmt" or alias "path/to/pkg" or . "testing" or _ "net/http/pprof"
	reGroupLine = regexp.MustCompile(`^\s*(?:(\w+|\.)\s+)?"([^"]+)"`)
	// Top-level declarations that signal end of imports.
	reTopLevel = regexp.MustCompile(`^(?:func|type|var|const)\s`)
)

// Extractor extracts import facts from Go source files.
type Extractor struct{}

// New returns a new Go extractor.
func New() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Language() string           { return "go" }
func (e *Extractor) Clone() extractor.Extractor { return New() }
func (e *Extractor) Extensions() []string       { return []string{".go"} }

// Extract parses Go source and returns all import facts.
func (e *Extractor) Extract(filePath string, content []byte) ([]model.ImportFact, error) {
	lines := strings.Split(string(content), "\n")
	var facts []model.ImportFact
	inGroup := false

	inBlockComment := false
	for i, rawLine := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(rawLine)

		// Skip block comments using a stateful flag (not inner loop mutation,
		// which doesn't affect the range iterator).
		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
			}
			continue
		}
		if strings.HasPrefix(line, "/*") {
			if !strings.Contains(line, "*/") {
				inBlockComment = true
			}
			continue
		}

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		if inGroup {
			// End of group.
			if strings.Contains(line, ")") {
				inGroup = false
				continue
			}

			// Skip comment lines inside group.
			if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
				continue
			}

			if m := reGroupLine.FindStringSubmatch(line); m != nil {
				alias := m[1]
				path := m[2]
				fact := buildImportFact(path, alias, lineNum)
				if fact != nil {
					facts = append(facts, *fact)
				}
			}
			continue
		}

		// Check for group import start.
		if reGroupStart.MatchString(line) {
			inGroup = true
			continue
		}

		// Check for single import.
		if m := reSingleImport.FindStringSubmatch(line); m != nil {
			alias := m[1]
			path := m[2]
			fact := buildImportFact(path, alias, lineNum)
			if fact != nil {
				facts = append(facts, *fact)
			}
			continue
		}

		// Stop at first top-level declaration (func, type, var, const).
		if reTopLevel.MatchString(line) {
			break
		}
	}

	return facts, nil
}

// ExtractDefinitions returns signature lines for the given symbol names.
// Matches: func X(...), type X struct/interface/..., var X, const X
func (e *Extractor) ExtractDefinitions(content []byte, symbolNames []string) []string {
	nameSet := extractor.BuildNameSet(symbolNames)
	if nameSet == nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	var defs []string

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)

		// func Name(...) or func (recv) Name(...)
		if m := reFuncSignature.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TrimBodyOpener(trimmed))
				continue
			}
		}

		// type Name struct/interface/...
		if m := reTypeSignature.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, trimmed)
				continue
			}
		}

		// var/const Name ...
		if m := reVarConst.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[2]] {
				defs = append(defs, extractor.TruncateSig(trimmed, extractor.MaxSignatureLength))
			}
		}
	}

	return defs
}

var (
	reFuncSignature = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?(\w+)\s*[\(\[]`)
	reTypeSignature = regexp.MustCompile(`^type\s+(\w+)\s+`)
	reVarConst      = regexp.MustCompile(`^(var|const)\s+(\w+)\s+`)
)

// buildImportFact creates an ImportFact for a Go import path.
// Returns nil for the cgo pseudo-package "C".
func buildImportFact(path, alias string, lineNum int) *model.ImportFact {
	// Skip cgo pseudo-package.
	if path == "C" {
		return nil
	}

	var importedNames []string
	if alias != "" {
		// Named import: alias is the local name.
		importedNames = []string{alias}
	} else {
		// Default: last path element is the package name.
		parts := strings.Split(path, "/")
		pkgName := parts[len(parts)-1]
		importedNames = []string{pkgName}
	}

	return &model.ImportFact{
		Specifier:     path,
		ImportType:    "absolute",
		LineNumber:    lineNum,
		ImportedNames: importedNames,
	}
}
