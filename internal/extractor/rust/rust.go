// Package rust implements import extraction for Rust source files.
package rust

import (
	"regexp"
	"strings"

	"github.com/kehoej/contextception/internal/extractor"
	"github.com/kehoej/contextception/internal/model"
)

var (
	// reUseStart detects the beginning of a use statement.
	// Handles: use, pub use, pub(crate) use, pub(super) use, etc.
	reUseStart = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?use\s+`)

	// mod config;
	// pub mod routes;
	reModDecl = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?mod\s+(\w+)\s*;`)

	// extern crate serde;
	reExternCrate = regexp.MustCompile(`^extern\s+crate\s+(\w+)`)

	// Definition patterns.
	reFnDecl     = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:unsafe\s+)?(?:const\s+)?fn\s+(\w+)`)
	reStructDecl = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?struct\s+(\w+)`)
	reEnumDecl   = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?enum\s+(\w+)`)
	reTraitDecl  = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?trait\s+(\w+)`)
	reTypeAlias  = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?type\s+(\w+)`)
	reConstDecl  = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?(?:static|const)\s+(\w+)\s*:`)
)

// Extractor extracts import facts from Rust source files.
type Extractor struct{}

// New returns a new Rust extractor.
func New() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Language() string           { return "rust" }
func (e *Extractor) Clone() extractor.Extractor { return New() }
func (e *Extractor) Extensions() []string       { return []string{".rs"} }

// Extract parses Rust source and returns all import/module facts.
func (e *Extractor) Extract(filePath string, content []byte) ([]model.ImportFact, error) {
	lines := strings.Split(string(content), "\n")
	var facts []model.ImportFact
	inBlockComment := false

	// Multi-line use accumulator state.
	var useAccum strings.Builder
	useStartLine := 0

	for i, rawLine := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(rawLine)

		// Handle block comments.
		if inBlockComment {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				inBlockComment = false
				line = strings.TrimSpace(line[idx+2:])
			} else {
				continue
			}
		}
		if strings.HasPrefix(line, "/*") {
			if !strings.Contains(line, "*/") {
				inBlockComment = true
				continue
			}
			idx := strings.Index(line, "*/")
			line = strings.TrimSpace(line[idx+2:])
		}

		// Skip line comments.
		if strings.HasPrefix(line, "//") {
			continue
		}

		if line == "" {
			continue
		}

		// Multi-line use accumulation.
		if useAccum.Len() > 0 {
			useAccum.WriteString(" ")
			useAccum.WriteString(line)
			if strings.Contains(line, ";") {
				joined := useAccum.String()
				useAccum.Reset()
				spec := extractUseSpec(joined)
				if spec != "" {
					facts = append(facts, parseUseStatement(spec, useStartLine)...)
				}
			}
			continue
		}

		// use statements.
		if reUseStart.MatchString(line) {
			if strings.Contains(line, ";") {
				// Single-line use statement.
				spec := extractUseSpec(line)
				if spec != "" {
					facts = append(facts, parseUseStatement(spec, lineNum)...)
				}
			} else {
				// Start of multi-line use statement.
				useAccum.WriteString(line)
				useStartLine = lineNum
			}
			continue
		}

		// mod declarations (file-backed).
		if m := reModDecl.FindStringSubmatch(line); m != nil {
			modName := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:     "mod:" + modName,
				ImportType:    "module",
				LineNumber:    lineNum,
				ImportedNames: []string{modName},
			})
			continue
		}

		// extern crate (legacy).
		if m := reExternCrate.FindStringSubmatch(line); m != nil {
			crateName := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:     "extern:" + crateName,
				ImportType:    "absolute",
				LineNumber:    lineNum,
				ImportedNames: []string{crateName},
			})
			continue
		}
	}

	return facts, nil
}

// extractUseSpec extracts the path spec from a use statement string.
// Input: "use crate::{A, B};" or "pub use crate::foo;" or "pub(crate) use crate::bar;"
// Output: "crate::{A, B}" or "crate::foo" or "crate::bar"
func extractUseSpec(stmt string) string {
	idx := strings.Index(stmt, "use ")
	if idx < 0 {
		return ""
	}
	spec := stmt[idx+4:]
	if semiIdx := strings.Index(spec, ";"); semiIdx >= 0 {
		spec = spec[:semiIdx]
	}
	return strings.TrimSpace(spec)
}

// parseUseStatement parses a use path spec into import facts.
// Handles simple paths, grouped imports, nested groups, wildcards, and "as" renames.
// Grouped imports are flattened into individual facts for maximum edge coverage.
func parseUseStatement(spec string, lineNum int) []model.ImportFact {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}

	// Flatten any grouped/nested imports into individual paths.
	paths := flattenUseTree(spec)

	var facts []model.ImportFact
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Handle "as" rename.
		importedName := ""
		if asIdx := strings.Index(p, " as "); asIdx >= 0 {
			importedName = strings.TrimSpace(p[asIdx+4:])
			p = strings.TrimSpace(p[:asIdx])
		}

		// Handle wildcard: ends with ::*
		if strings.HasSuffix(p, "::*") {
			modulePath := strings.TrimSuffix(p, "::*")
			facts = append(facts, model.ImportFact{
				Specifier:     modulePath,
				ImportType:    classifyRustImport(modulePath),
				LineNumber:    lineNum,
				ImportedNames: []string{"*"},
			})
			continue
		}

		// Extract last path component as imported name.
		if importedName == "" {
			parts := strings.Split(p, "::")
			importedName = parts[len(parts)-1]
		}

		facts = append(facts, model.ImportFact{
			Specifier:     p,
			ImportType:    classifyRustImport(p),
			LineNumber:    lineNum,
			ImportedNames: []string{importedName},
		})
	}

	return facts
}

// flattenUseTree recursively expands a Rust use path with nested groups
// into individual fully-qualified paths.
// Example: "crate::{a::{B, C}, d::E}" → ["crate::a::B", "crate::a::C", "crate::d::E"]
func flattenUseTree(spec string) []string {
	// Handle top-level braces without prefix: {crate::foo, std::bar}
	if len(spec) > 0 && spec[0] == '{' {
		closeBrace := matchBrace(spec, 0)
		if closeBrace == len(spec)-1 {
			inner := spec[1:closeBrace]
			return flattenItems(inner, "")
		}
	}

	// Find ::{ for grouped imports.
	braceIdx := strings.Index(spec, "::{")
	if braceIdx < 0 {
		// No group — leaf path.
		return []string{spec}
	}

	prefix := spec[:braceIdx]
	openBrace := braceIdx + 2 // position of '{'
	closeBrace := matchBrace(spec, openBrace)
	if closeBrace < 0 {
		return []string{spec} // malformed, return as-is
	}

	inner := spec[openBrace+1 : closeBrace]
	return flattenItems(inner, prefix)
}

// flattenItems splits comma-separated items at the top brace level
// and recursively flattens each, prepending the prefix.
func flattenItems(inner, prefix string) []string {
	items := splitTopLevel(inner)
	var result []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		// "self" within a group refers to the prefix module itself.
		if item == "self" {
			if prefix != "" {
				result = append(result, prefix)
			}
			continue
		}
		var fullPath string
		if prefix != "" {
			fullPath = prefix + "::" + item
		} else {
			fullPath = item
		}
		result = append(result, flattenUseTree(fullPath)...)
	}
	return result
}

// matchBrace finds the matching closing brace for the opening brace at pos.
// Returns -1 if no match found.
func matchBrace(s string, pos int) int {
	depth := 0
	for i := pos; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevel splits a string on commas that are not inside braces.
func splitTopLevel(s string) []string {
	var items []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				items = append(items, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		items = append(items, s[start:])
	}
	return items
}

// classifyRustImport returns "relative" for crate/super/self paths and "absolute" otherwise.
func classifyRustImport(spec string) string {
	if spec == "crate" || strings.HasPrefix(spec, "crate::") ||
		spec == "super" || strings.HasPrefix(spec, "super::") ||
		spec == "self" || strings.HasPrefix(spec, "self::") {
		return "relative"
	}
	return "absolute"
}

// defPattern pairs a regex with a post-processing strategy for definition extraction.
type defPattern struct {
	re        *regexp.Regexp
	trimBrace bool // if true, strip " {" suffix; otherwise truncate at MaxSignatureLength
}

// defPatterns lists all Rust definition patterns, checked in order.
var defPatterns = []defPattern{
	{reFnDecl, true},
	{reStructDecl, true},
	{reEnumDecl, true},
	{reTraitDecl, true},
	{reTypeAlias, false},
	{reConstDecl, false},
}

// ExtractDefinitions returns signature lines for the given symbol names.
func (e *Extractor) ExtractDefinitions(content []byte, symbolNames []string) []string {
	nameSet := extractor.BuildNameSet(symbolNames)
	if nameSet == nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	var defs []string

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		for _, dp := range defPatterns {
			if m := dp.re.FindStringSubmatch(trimmed); m != nil {
				if nameSet[m[1]] {
					if dp.trimBrace {
						defs = append(defs, extractor.TrimBodyOpener(trimmed))
					} else {
						defs = append(defs, extractor.TruncateSig(trimmed, extractor.MaxSignatureLength))
					}
				}
				break // only match first pattern per line
			}
		}
	}

	return defs
}
