//go:build !cgo

// Package typescript implements import extraction for TypeScript and JavaScript source files.
// This file provides a regex-based fallback when CGO is not available (no tree-sitter).
package typescript

import (
	"regexp"
	"strings"

	"github.com/kehoej/contextception/internal/extractor"
	"github.com/kehoej/contextception/internal/model"
)

// tsMaxSignatureLength is the max length for TypeScript signatures.
// Higher than the default (120) because TS signatures often include type annotations.
const tsMaxSignatureLength = 150

// Compiled regex patterns for TypeScript/JavaScript import statements.
// Go's regexp (RE2) does not support backreferences, so we match any quote character
// and rely on the structure being well-formed (matching quotes on the same line).
var (
	// import { X, Y } from 'module'  OR  import type { X } from 'module'
	reNamedImport = regexp.MustCompile(`^import\s+(?:type\s+)?\{([^}]*)\}\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)
	// import X from 'module'  OR  import type X from 'module'
	reDefaultImport = regexp.MustCompile(`^import\s+(?:type\s+)?(\w+)\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)
	// import * as X from 'module'
	reNamespaceImport = regexp.MustCompile(`^import\s+\*\s+as\s+\w+\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)
	// import 'module' (side-effect)
	reSideEffectImport = regexp.MustCompile(`^import\s+['"\x60]([^'"\x60]+)['"\x60]\s*;?\s*$`)
	// import X, { Y, Z } from 'module' (default + named)
	reDefaultAndNamed = regexp.MustCompile(`^import\s+(\w+)\s*,\s*\{([^}]*)\}\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)
	// import X, * as Y from 'module' (default + namespace)
	reDefaultAndNamespace = regexp.MustCompile(`^import\s+(\w+)\s*,\s*\*\s+as\s+\w+\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)

	// export { X, Y } from 'module'  OR  export type { X } from 'module'
	reNamedReExport = regexp.MustCompile(`^export\s+(?:type\s+)?\{([^}]*)\}\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)
	// export * from 'module'
	reStarReExport = regexp.MustCompile(`^export\s+\*\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)
	// export * as X from 'module'
	reNamespaceReExport = regexp.MustCompile(`^export\s+\*\s+as\s+\w+\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)

	// const X = require('module')  OR  const { X } = require('module')
	reRequire = regexp.MustCompile(`require\s*\(\s*['"\x60]([^'"\x60]+)['"\x60]\s*\)`)
	// import('module') — dynamic import
	reDynamicImport = regexp.MustCompile(`import\s*\(\s*['"\x60]([^'"\x60]+)['"\x60]\s*\)`)

	// Multiline named import: captures opening brace content + from clause on a later line.
	reMultilineImportStart = regexp.MustCompile(`^import\s+(?:type\s+)?\{([^}]*)$`)
	reMultilineImportEnd   = regexp.MustCompile(`^([^}]*)\}\s+from\s+['"\x60]([^'"\x60]+)['"\x60]`)

	// Definition patterns for ExtractDefinitions.
	reFuncDef      = regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s*\*?\s+(\w+)`)
	reClassDef     = regexp.MustCompile(`^(?:export\s+)?(?:abstract\s+)?class\s+(\w+)`)
	reInterfaceDef = regexp.MustCompile(`^(?:export\s+)?interface\s+(\w+)`)
	reTypeDef      = regexp.MustCompile(`^(?:export\s+)?type\s+(\w+)\s*[=<]`)
	reConstDef     = regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+(\w+)`)
	reEnumDef      = regexp.MustCompile(`^(?:export\s+)?(?:const\s+)?enum\s+(\w+)`)
)

// Extractor extracts import facts from TypeScript/JavaScript source files
// using regex patterns (no CGO / tree-sitter dependency).
type Extractor struct{}

// New returns a new TypeScript/JavaScript extractor.
func New() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Language() string              { return "typescript" }
func (e *Extractor) Clone() extractor.Extractor    { return New() }
func (e *Extractor) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"}
}

// Extract parses TypeScript/JavaScript source and returns all import facts.
func (e *Extractor) Extract(filePath string, content []byte) ([]model.ImportFact, error) {
	lines := strings.Split(string(content), "\n")
	var facts []model.ImportFact

	// State for multiline named imports.
	var inMultilineImport bool
	var multilineNames []string
	var multilineStartLine int

	inBlockComment := false

	for i, rawLine := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(rawLine)

		// Skip block comments.
		if inBlockComment {
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			if !strings.Contains(trimmed, "*/") {
				inBlockComment = true
			}
			continue
		}

		// Skip single-line comments.
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Handle multiline named imports.
		if inMultilineImport {
			if m := reMultilineImportEnd.FindStringSubmatch(trimmed); m != nil {
				namesStr := m[1]
				spec := m[2]
				multilineNames = append(multilineNames, parseImportNames(namesStr)...)
				facts = append(facts, model.ImportFact{
					Specifier:     spec,
					ImportType:    classifySpecifier(spec),
					LineNumber:    multilineStartLine,
					ImportedNames: multilineNames,
				})
				inMultilineImport = false
				continue
			}
			// Accumulate names from intermediate lines.
			multilineNames = append(multilineNames, parseImportNames(trimmed)...)
			continue
		}

		// Check for multiline import start: import { ... (no closing brace on this line)
		if m := reMultilineImportStart.FindStringSubmatch(trimmed); m != nil {
			inMultilineImport = true
			multilineStartLine = lineNum
			multilineNames = parseImportNames(m[1])
			continue
		}

		// Default + named: import React, { useState, useEffect } from 'react'
		if m := reDefaultAndNamed.FindStringSubmatch(trimmed); m != nil {
			defaultName := m[1]
			namedStr := m[2]
			spec := m[3]
			names := []string{defaultName}
			names = append(names, parseImportNames(namedStr)...)
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: names,
			})
			continue
		}

		// Default + namespace: import foo, * as bar from './mod'
		if m := reDefaultAndNamespace.FindStringSubmatch(trimmed); m != nil {
			defaultName := m[1]
			spec := m[2]
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: []string{defaultName, "*"},
			})
			continue
		}

		// Named imports: import { X, Y } from 'module'
		if m := reNamedImport.FindStringSubmatch(trimmed); m != nil {
			namesStr := m[1]
			spec := m[2]
			names := parseImportNames(namesStr)
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: names,
			})
			continue
		}

		// Namespace import: import * as X from 'module'
		if m := reNamespaceImport.FindStringSubmatch(trimmed); m != nil {
			spec := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: []string{"*"},
			})
			continue
		}

		// Default import: import X from 'module'
		if m := reDefaultImport.FindStringSubmatch(trimmed); m != nil {
			defaultName := m[1]
			spec := m[2]
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: []string{defaultName},
			})
			continue
		}

		// Namespace re-export: export * as X from 'module'
		if m := reNamespaceReExport.FindStringSubmatch(trimmed); m != nil {
			spec := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: []string{"*"},
			})
			continue
		}

		// Named re-export: export { X, Y } from 'module'
		if m := reNamedReExport.FindStringSubmatch(trimmed); m != nil {
			namesStr := m[1]
			spec := m[2]
			names := parseExportNames(namesStr)
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: names,
			})
			continue
		}

		// Star re-export: export * from 'module'
		if m := reStarReExport.FindStringSubmatch(trimmed); m != nil {
			spec := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    classifySpecifier(spec),
				LineNumber:    lineNum,
				ImportedNames: []string{"*"},
			})
			continue
		}

		// Side-effect import: import 'module'
		if m := reSideEffectImport.FindStringSubmatch(trimmed); m != nil {
			spec := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:  spec,
				ImportType: classifySpecifier(spec),
				LineNumber: lineNum,
			})
			continue
		}

		// require() — can appear anywhere in a line.
		if m := reRequire.FindStringSubmatch(trimmed); m != nil {
			spec := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:  spec,
				ImportType: classifySpecifier(spec),
				LineNumber: lineNum,
			})
			// Don't continue — line might also have dynamic import.
		}

		// Dynamic import() — can appear anywhere in a line.
		if m := reDynamicImport.FindStringSubmatch(trimmed); m != nil {
			spec := m[1]
			// Avoid double-counting if this is a regular import statement already matched.
			if !strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "import(") || strings.Contains(trimmed, "import (") {
				// Check we didn't already add this as a require match on same line.
				alreadyAdded := false
				for _, f := range facts {
					if f.LineNumber == lineNum && f.Specifier == spec {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					facts = append(facts, model.ImportFact{
						Specifier:  spec,
						ImportType: classifySpecifier(spec),
						LineNumber: lineNum,
					})
				}
			}
		}
	}

	return facts, nil
}

// parseImportNames parses a comma-separated list of import names, handling aliases and type modifiers.
// e.g., "foo, bar as baz, type Qux" -> ["foo", "bar", "Qux"]
func parseImportNames(s string) []string {
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
		// Strip "type " prefix for inline type imports.
		p = strings.TrimPrefix(p, "type ")
		p = strings.TrimSpace(p)
		// Strip alias: "foo as bar" -> "foo"
		if idx := strings.Index(p, " as "); idx >= 0 {
			p = strings.TrimSpace(p[:idx])
		}
		if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// parseExportNames parses a comma-separated list of export names, handling aliases and type modifiers.
// For re-exports, the original name (before "as") is what matters.
// e.g., "foo as bar, type Qux" -> ["foo", "Qux"]
func parseExportNames(s string) []string {
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
		// Strip "type " prefix for inline type exports.
		p = strings.TrimPrefix(p, "type ")
		p = strings.TrimSpace(p)
		// Strip alias: "foo as bar" -> "foo"
		if idx := strings.Index(p, " as "); idx >= 0 {
			p = strings.TrimSpace(p[:idx])
		}
		if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// ExtractDefinitions returns signature strings for the given symbol names
// using regex to find function, class, interface, type alias, enum, and variable declarations.
func (e *Extractor) ExtractDefinitions(content []byte, symbolNames []string) []string {
	if len(symbolNames) == 0 {
		return nil
	}

	nameSet := extractor.BuildNameSet(symbolNames)

	lines := strings.Split(string(content), "\n")
	var defs []string

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}

		// enum (check before const to avoid const enum being matched as const)
		if m := reEnumDef.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				sig := extractor.TrimBodyOpener(trimmed)
				defs = append(defs, extractor.TruncateSig(sig, tsMaxSignatureLength))
				continue
			}
		}

		// function / async function / generator function
		if m := reFuncDef.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				sig := extractor.TrimBodyOpener(trimmed)
				// Strip "export " prefix to match tree-sitter output.
				sig = strings.TrimPrefix(sig, "export ")
				defs = append(defs, extractor.TruncateSig(sig, tsMaxSignatureLength))
				continue
			}
		}

		// class
		if m := reClassDef.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				sig := extractor.TrimBodyOpener(trimmed)
				// Collapse whitespace for multiline class declarations.
				sig = strings.Join(strings.Fields(sig), " ")
				// Strip "export " prefix to match tree-sitter output.
				sig = strings.TrimPrefix(sig, "export ")
				sig = strings.TrimPrefix(sig, "abstract ")
				if !strings.HasPrefix(sig, "class") {
					sig = strings.TrimPrefix(sig, "export ")
				}
				defs = append(defs, extractor.TruncateSig(sig, tsMaxSignatureLength))
				continue
			}
		}

		// interface
		if m := reInterfaceDef.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				sig := extractor.TrimBodyOpener(trimmed)
				// Strip "export " prefix.
				sig = strings.TrimPrefix(sig, "export ")
				defs = append(defs, extractor.TruncateSig(sig, tsMaxSignatureLength))
				continue
			}
		}

		// type alias
		if m := reTypeDef.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				// Strip "export " prefix.
				sig := strings.TrimPrefix(trimmed, "export ")
				defs = append(defs, extractor.TruncateSig(sig, tsMaxSignatureLength))
				continue
			}
		}

		// const/let/var
		if m := reConstDef.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				// Strip "export " prefix.
				sig := strings.TrimPrefix(trimmed, "export ")
				defs = append(defs, extractor.TruncateSig(sig, tsMaxSignatureLength))
				continue
			}
		}
	}

	if len(defs) == 0 {
		return nil
	}
	return defs
}

// classifySpecifier returns "relative" for ./ or ../ prefixed specifiers, "absolute" otherwise.
func classifySpecifier(spec string) string {
	if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return "relative"
	}
	return "absolute"
}

// stripQuotes removes surrounding quotes (', ", or `) from a string.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '\'' || first == '"' || first == '`') && first == last {
			return s[1 : len(s)-1]
		}
	}
	return s
}
