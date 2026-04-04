// Package java implements import extraction for Java source files.
package java

import (
	"regexp"
	"strings"

	"github.com/kehoej/contextception/internal/extractor"
	"github.com/kehoej/contextception/internal/model"
)

var (
	// import java.util.List;
	// import static java.lang.Math.abs;
	reImport = regexp.MustCompile(`^import\s+(static\s+)?([\w.]+(?:\.\*)?)\s*;`)

	// package com.example.app;
	rePackage = regexp.MustCompile(`^package\s+([\w.]+)\s*;`)

	// Definition patterns.
	reClassDecl     = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+|abstract\s+|final\s+|static\s+)*(?:class|interface|enum|record)\s+(\w+)`)
	reMethodDecl    = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+|abstract\s+|final\s+|static\s+|synchronized\s+|native\s+|default\s+)*(?:[\w<>\[\],\s]+)\s+(\w+)\s*\(`)
	reConstantDecl  = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+)?static\s+final\s+\S+\s+(\w+)\s*=`)
)

// Extractor extracts import facts from Java source files.
type Extractor struct{}

// New returns a new Java extractor.
func New() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Language() string           { return "java" }
func (e *Extractor) Clone() extractor.Extractor { return New() }
func (e *Extractor) Extensions() []string       { return []string{".java"} }

// Extract parses Java source and returns all import facts.
func (e *Extractor) Extract(filePath string, content []byte) ([]model.ImportFact, error) {
	lines := strings.Split(string(content), "\n")
	var facts []model.ImportFact
	inBlockComment := false

	var packageName string

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
			// Single-line block comment.
			idx := strings.Index(line, "*/")
			line = strings.TrimSpace(line[idx+2:])
		}

		// Skip single-line comments.
		if strings.HasPrefix(line, "//") {
			continue
		}

		// Skip empty lines.
		if line == "" {
			continue
		}

		// Stop parsing imports at class/interface/enum declaration.
		if reClassDecl.MatchString(line) {
			break
		}

		// Capture package declaration for same-package resolution.
		if m := rePackage.FindStringSubmatch(line); m != nil {
			packageName = m[1]
			continue
		}

		// Match import statement.
		if m := reImport.FindStringSubmatch(line); m != nil {
			isStatic := strings.TrimSpace(m[1]) == "static"
			spec := m[2]

			importType := "absolute"
			if isStatic {
				importType = "static"
			}

			var importedNames []string

			if strings.HasSuffix(spec, ".*") {
				// Wildcard import: import java.util.* → package is java.util
				importedNames = []string{"*"}
			} else {
				// Single import: last dot-separated component is the imported name.
				parts := strings.Split(spec, ".")
				name := parts[len(parts)-1]
				importedNames = []string{name}
			}

			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    importType,
				LineNumber:    lineNum,
				ImportedNames: importedNames,
			})
		}
	}

	// Emit synthetic same-package fact for Java's implicit same-package type visibility.
	if packageName != "" {
		facts = append(facts, model.ImportFact{
			Specifier:  packageName + ".*",
			ImportType: "same_package",
			LineNumber: 0,
		})
	}

	return facts, nil
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

		// Class/interface/enum/record declaration.
		if m := reClassDecl.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TrimBodyOpener(trimmed))
				continue
			}
		}

		// Static final constant.
		if m := reConstantDecl.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TruncateSig(trimmed, extractor.MaxSignatureLength))
				continue
			}
		}

		// Method declaration.
		if m := reMethodDecl.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TrimBodyOpener(trimmed))
				continue
			}
		}
	}

	return defs
}
