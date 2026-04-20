// Package csharp implements import extraction for C# source files.
package csharp

import (
	"regexp"
	"strings"

	"github.com/kehoej/contextception/internal/extractor"
	"github.com/kehoej/contextception/internal/model"
)

var (
	// using System.Collections.Generic;
	reUsing = regexp.MustCompile(`^using\s+([\w.]+)\s*;`)

	// using static System.Math;
	reUsingStatic = regexp.MustCompile(`^using\s+static\s+([\w.]+)\s*;`)

	// using Alias = Some.Namespace;
	reUsingAlias = regexp.MustCompile(`^using\s+(\w+)\s*=\s*([\w.]+)\s*;`)

	// global using System.Linq;  /  global using static System.Math;
	reGlobalUsing = regexp.MustCompile(`^global\s+using\s+(static\s+)?([\w.]+)\s*;`)

	// global using Alias = Some.Namespace;
	reGlobalUsingAlias = regexp.MustCompile(`^global\s+using\s+(\w+)\s*=\s*([\w.]+)\s*;`)

	// namespace Foo.Bar.Baz { ... }  or  namespace Foo.Bar.Baz;  (file-scoped)
	reNamespace = regexp.MustCompile(`^namespace\s+([\w.]+)`)

	// Class/interface/struct/enum/record declarations (stop parsing imports here).
	reTypeDecl = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+|internal\s+|abstract\s+|sealed\s+|static\s+|partial\s+)*(?:class|interface|struct|enum|record)\s+(\w+)`)

	// Definition patterns for ExtractDefinitions.
	reMethodDecl   = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+|internal\s+|abstract\s+|sealed\s+|static\s+|virtual\s+|override\s+|async\s+|new\s+|extern\s+)*(?:[\w<>\[\],?\s]+)\s+(\w+)\s*\(`)
	rePropertyDecl = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+|internal\s+|static\s+|virtual\s+|override\s+|abstract\s+|new\s+)*\w[\w<>\[\]?,\s]*\s+(\w+)\s*\{`)
	reConstDecl    = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+|internal\s+)?(?:static\s+)?(?:readonly\s+|const\s+)\S+\s+(\w+)\s*[=;]`)
	reDelegateDecl = regexp.MustCompile(`^(?:public\s+|protected\s+|private\s+|internal\s+)?delegate\s+\S+\s+(\w+)\s*[\(<]`)
)

// Extractor extracts import facts from C# source files.
type Extractor struct{}

// New returns a new C# extractor.
func New() *Extractor {
	return &Extractor{}
}

func (e *Extractor) Language() string           { return "csharp" }
func (e *Extractor) Clone() extractor.Extractor { return New() }
func (e *Extractor) Extensions() []string       { return []string{".cs"} }

// Extract parses C# source and returns all import facts.
func (e *Extractor) Extract(filePath string, content []byte) ([]model.ImportFact, error) {
	lines := strings.Split(string(content), "\n")
	var facts []model.ImportFact
	inBlockComment := false

	var namespaceName string

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

		// Stop parsing imports at class/struct/interface/enum/record declaration.
		if reTypeDecl.MatchString(line) {
			break
		}

		// Capture namespace declaration for same-namespace resolution.
		if m := reNamespace.FindStringSubmatch(line); m != nil {
			namespaceName = m[1]
			continue
		}

		// Global using alias: global using Alias = Some.Namespace;
		if m := reGlobalUsingAlias.FindStringSubmatch(line); m != nil {
			spec := m[2] // RHS namespace
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    "alias",
				LineNumber:    lineNum,
				ImportedNames: []string{m[1]}, // alias name
			})
			continue
		}

		// Global using [static] Namespace;
		if m := reGlobalUsing.FindStringSubmatch(line); m != nil {
			isStatic := strings.TrimSpace(m[1]) == "static"
			spec := m[2]

			importType := "absolute"
			if isStatic {
				importType = "static"
			}

			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    importType,
				LineNumber:    lineNum,
				ImportedNames: []string{"*"},
			})
			continue
		}

		// using Alias = Some.Namespace;
		if m := reUsingAlias.FindStringSubmatch(line); m != nil {
			spec := m[2] // RHS namespace
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    "alias",
				LineNumber:    lineNum,
				ImportedNames: []string{m[1]}, // alias name
			})
			continue
		}

		// using static System.Math;
		if m := reUsingStatic.FindStringSubmatch(line); m != nil {
			spec := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    "static",
				LineNumber:    lineNum,
				ImportedNames: []string{"*"},
			})
			continue
		}

		// using System.Collections.Generic;
		if m := reUsing.FindStringSubmatch(line); m != nil {
			spec := m[1]
			facts = append(facts, model.ImportFact{
				Specifier:     spec,
				ImportType:    "absolute",
				LineNumber:    lineNum,
				ImportedNames: []string{"*"},
			})
			continue
		}
	}

	// Emit synthetic same_namespace fact for C#'s implicit same-namespace type visibility.
	if namespaceName != "" {
		facts = append(facts, model.ImportFact{
			Specifier:  namespaceName + ".*",
			ImportType: "same_namespace",
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

		// Class/interface/struct/enum/record declaration.
		if m := reTypeDecl.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TrimBodyOpener(trimmed))
				continue
			}
		}

		// Delegate declaration.
		if m := reDelegateDecl.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TruncateSig(trimmed, extractor.MaxSignatureLength))
				continue
			}
		}

		// Constant/readonly field.
		if m := reConstDecl.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TruncateSig(trimmed, extractor.MaxSignatureLength))
				continue
			}
		}

		// Property declaration (must come before method to avoid { false positive).
		if m := rePropertyDecl.FindStringSubmatch(trimmed); m != nil {
			if nameSet[m[1]] {
				defs = append(defs, extractor.TrimBodyOpener(trimmed))
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
