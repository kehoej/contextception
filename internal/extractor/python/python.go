// Package python implements import extraction for Python source files.
package python

import (
	"regexp"
	"strings"

	"github.com/kehoej/contextception/internal/extractor"
	"github.com/kehoej/contextception/internal/model"
)

// Compiled regex patterns for Python import statements.
var (
	// import foo, import foo.bar, import foo, bar, baz
	reAbsoluteImport = regexp.MustCompile(`^import\s+([\w.]+(?:\s*,\s*[\w.]+)*)`)
	// from foo.bar import baz, qux
	reFromImportAbs = regexp.MustCompile(`^from\s+([\w.]+)\s+import\s+(.+)`)
	// from .foo import bar, from ..foo.bar import baz
	reFromImportRel = regexp.MustCompile(`^from\s+(\.+[\w.]*)\s+import\s+(.+)`)
)

// Extractor extracts import facts from Python source files.
type Extractor struct{}

// New returns a new Python extractor.
func New() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Language() string           { return "python" }
func (e *Extractor) Clone() extractor.Extractor { return New() }
func (e *Extractor) Extensions() []string       { return []string{".py"} }

// Extract parses Python source and returns all import facts.
func (e *Extractor) Extract(filePath string, content []byte) ([]model.ImportFact, error) {
	lines := strings.Split(string(content), "\n")
	var facts []model.ImportFact

	var inTripleQuote bool
	var tripleChar string // `"""` or `'''`
	var inParenImport bool
	var parenSpec string
	var parenNames []string
	var parenLine int
	var parenIsRel bool
	var pendingContinuation string
	var continuationLine int

	for i, rawLine := range lines {
		lineNum := i + 1

		// Handle triple-quoted strings.
		if inTripleQuote {
			if strings.Contains(rawLine, tripleChar) {
				inTripleQuote = false
			}
			continue
		}

		// Check for triple-quote start (not inside one).
		stripped := strings.TrimSpace(rawLine)
		if checkTripleQuoteStart(stripped, &tripleChar) {
			// Check if it also closes on the same line (e.g., """docstring""").
			rest := stripped[3:]
			if !strings.Contains(rest, tripleChar) {
				inTripleQuote = true
			}
			continue
		}

		// Handle backslash continuation.
		if pendingContinuation != "" {
			combined := pendingContinuation + " " + stripped
			if strings.HasSuffix(stripped, "\\") {
				pendingContinuation = strings.TrimSuffix(combined, "\\")
				continue
			}
			pendingContinuation = ""
			stripped = combined
			lineNum = continuationLine
		} else if strings.HasSuffix(stripped, "\\") && (strings.HasPrefix(stripped, "import ") || strings.HasPrefix(stripped, "from ")) {
			pendingContinuation = strings.TrimSuffix(stripped, "\\")
			continuationLine = lineNum
			continue
		}

		// Handle multiline parenthesized imports.
		if inParenImport {
			// Strip inline comments.
			line := stripComment(stripped)
			line = strings.TrimSpace(line)

			if strings.Contains(line, ")") {
				line = strings.TrimSuffix(strings.TrimSpace(strings.SplitN(line, ")", 2)[0]), ",")
				if line != "" {
					parenNames = append(parenNames, parseNames(line)...)
				}
				inParenImport = false
				facts = append(facts, buildFromImport(parenSpec, parenNames, parenLine, parenIsRel)...)
				continue
			}
			line = strings.TrimSuffix(line, ",")
			if line != "" {
				parenNames = append(parenNames, parseNames(line)...)
			}
			continue
		}

		// Skip blank and comment-only lines.
		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}

		// Strip inline comment before matching.
		line := stripComment(stripped)
		line = strings.TrimSpace(line)

		// Try relative from-import first (more specific regex).
		if m := reFromImportRel.FindStringSubmatch(line); m != nil {
			spec := m[1]
			namesStr := m[2]

			if strings.Contains(namesStr, "(") {
				// Multiline parenthesized import.
				namesStr = strings.TrimPrefix(namesStr, "(")
				namesStr = strings.TrimSpace(namesStr)
				if strings.Contains(namesStr, ")") {
					namesStr = strings.SplitN(namesStr, ")", 2)[0]
					names := parseNames(namesStr)
					facts = append(facts, buildFromImport(spec, names, lineNum, true)...)
				} else {
					inParenImport = true
					parenSpec = spec
					parenNames = parseNames(namesStr)
					parenLine = lineNum
					parenIsRel = true
				}
			} else {
				names := parseNames(namesStr)
				facts = append(facts, buildFromImport(spec, names, lineNum, true)...)
			}
			continue
		}

		// Try absolute from-import.
		if m := reFromImportAbs.FindStringSubmatch(line); m != nil {
			spec := m[1]
			namesStr := m[2]

			if strings.Contains(namesStr, "(") {
				namesStr = strings.TrimPrefix(namesStr, "(")
				namesStr = strings.TrimSpace(namesStr)
				if strings.Contains(namesStr, ")") {
					namesStr = strings.SplitN(namesStr, ")", 2)[0]
					names := parseNames(namesStr)
					facts = append(facts, buildFromImport(spec, names, lineNum, false)...)
				} else {
					inParenImport = true
					parenSpec = spec
					parenNames = parseNames(namesStr)
					parenLine = lineNum
					parenIsRel = false
				}
			} else {
				names := parseNames(namesStr)
				facts = append(facts, buildFromImport(spec, names, lineNum, false)...)
			}
			continue
		}

		// Try absolute import.
		if m := reAbsoluteImport.FindStringSubmatch(line); m != nil {
			modules := strings.Split(m[1], ",")
			for _, mod := range modules {
				mod = strings.TrimSpace(mod)
				// Strip alias: "foo as bar" → "foo"
				if idx := strings.Index(mod, " as "); idx >= 0 {
					mod = strings.TrimSpace(mod[:idx])
				}
				if mod != "" {
					var importedNames []string
					if dotIdx := strings.LastIndex(mod, "."); dotIdx >= 0 {
						importedNames = []string{mod[dotIdx+1:]}
					}
					facts = append(facts, model.ImportFact{
						Specifier:     mod,
						ImportType:    "absolute",
						LineNumber:    lineNum,
						ImportedNames: importedNames,
					})
				}
			}
			continue
		}
	}

	return facts, nil
}

// buildFromImport creates one ImportFact per imported name from a from-import statement.
// This is more correct for Python — `from . import a, b, c` is semantically three
// separate imports. Each name may resolve to a different file (submodule) or the same
// file (__init__.py attributes). The resolver handles single-name facts correctly, and
// mergeEdgeSymbols in the indexer deduplicates edges with the same (src, dst, specifier).
func buildFromImport(spec string, names []string, lineNum int, isRelative bool) []model.ImportFact {
	importType := "absolute"
	if isRelative {
		importType = "relative"
	}
	if len(names) == 0 {
		return []model.ImportFact{{
			Specifier:  spec,
			ImportType: importType,
			LineNumber: lineNum,
		}}
	}
	facts := make([]model.ImportFact, len(names))
	for i, name := range names {
		facts[i] = model.ImportFact{
			Specifier:     spec,
			ImportType:    importType,
			LineNumber:    lineNum,
			ImportedNames: []string{name},
		}
	}
	return facts
}

// parseNames splits a comma-separated import names string, stripping aliases and whitespace.
func parseNames(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var names []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Strip alias: "Foo as Bar" → "Foo"
		if idx := strings.Index(p, " as "); idx >= 0 {
			p = strings.TrimSpace(p[:idx])
		}
		if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// stripComment removes an inline comment (# ...) from a line, respecting strings.
// This is a simplified version — it doesn't handle all edge cases with # inside strings,
// but is sufficient for import statements which rarely contain string literals.
func stripComment(line string) string {
	inSingle := false
	inDouble := false
	for i, ch := range line {
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

// ExtractDefinitions returns signature lines for the given symbol names.
// Matches: def X(...), class X(...):, X = ..., X: Type = ...
func (e *Extractor) ExtractDefinitions(content []byte, symbolNames []string) []string {
	nameSet := extractor.BuildNameSet(symbolNames)
	if nameSet == nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	var defs []string

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)

		// def name(...) or async def name(...)
		if m := reDefSignature.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, trimmed)
				continue
			}
		}

		// class name(...):
		if m := reClassSignature.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, trimmed)
				continue
			}
		}

		// NAME = ... (module-level constant/assignment)
		if m := reAssignment.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TruncateSig(trimmed, extractor.MaxSignatureLength))
			}
		}
	}

	return defs
}

var (
	reDefSignature   = regexp.MustCompile(`^(?:async\s+)?def\s+(\w+)\s*\(`)
	reClassSignature = regexp.MustCompile(`^class\s+(\w+)`)
	reAssignment     = regexp.MustCompile(`^(\w+)\s*(?::\s*\w[^=]*)?\s*=`)
)

// checkTripleQuoteStart checks if a line starts a triple-quoted string.
// It sets tripleChar to the matched triple-quote sequence.
func checkTripleQuoteStart(line string, tripleChar *string) bool {
	// Only consider lines that are primarily string literals, not import lines.
	if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
		return false
	}
	for _, tq := range []string{`"""`, `'''`} {
		if strings.Contains(line, tq) {
			// Count occurrences — odd number means we're entering a triple-quote.
			count := strings.Count(line, tq)
			if count%2 == 1 {
				*tripleChar = tq
				return true
			}
		}
	}
	return false
}
