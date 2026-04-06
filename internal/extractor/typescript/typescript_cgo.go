//go:build cgo

// Package typescript implements import extraction for TypeScript and JavaScript source files.
package typescript

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/tsx"

	"github.com/kehoej/contextception/internal/extractor"
	"github.com/kehoej/contextception/internal/model"
)

// tsMaxSignatureLength is the max length for TypeScript signatures.
// Higher than the default (120) because TS signatures often include type annotations.
const tsMaxSignatureLength = 150

// Extractor extracts import facts from TypeScript/JavaScript source files
// using tree-sitter with the TSX grammar (superset of TS, JS, and JSX).
type Extractor struct {
	parser *sitter.Parser
}

// New returns a new TypeScript/JavaScript extractor.
func New() *Extractor {
	p := sitter.NewParser()
	p.SetLanguage(tsx.GetLanguage())
	return &Extractor{parser: p}
}

func (e *Extractor) Language() string           { return "typescript" }
func (e *Extractor) Clone() extractor.Extractor { return New() }
func (e *Extractor) Extensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"}
}

// Extract parses TypeScript/JavaScript source and returns all import facts.
func (e *Extractor) Extract(filePath string, content []byte) ([]model.ImportFact, error) {
	tree, err := e.parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	var facts []model.ImportFact

	// Walk top-level statements for import/export declarations.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "import_statement":
			if f := extractImport(child, content); f != nil {
				facts = append(facts, *f)
			}
		case "export_statement":
			if f := extractExport(child, content); f != nil {
				facts = append(facts, *f)
			}
		}
	}

	// Recursive walk for require() and dynamic import() calls.
	facts = append(facts, extractCallExpressions(root, content)...)

	return facts, nil
}

// extractImport handles all import_statement variants:
//   - import foo from 'foo'
//   - import { bar, baz } from './bar'
//   - import * as utils from '../utils'
//   - import type { Config } from './config'
//   - import './side-effects'
func extractImport(node *sitter.Node, src []byte) *model.ImportFact {
	// Find the source (string literal) — the module specifier.
	source := findChildByType(node, "string")
	if source == nil {
		return nil
	}
	spec := stripQuotes(nodeText(source, src))
	if spec == "" {
		return nil
	}

	line := int(node.StartPoint().Row) + 1

	// Find the import clause to determine imported names.
	clause := findChildByType(node, "import_clause")
	if clause == nil {
		// Side-effect import: import './side-effects'
		return &model.ImportFact{
			Specifier:  spec,
			ImportType: classifySpecifier(spec),
			LineNumber: line,
		}
	}

	names := extractImportClauseNames(clause, src)
	return &model.ImportFact{
		Specifier:     spec,
		ImportType:    classifySpecifier(spec),
		LineNumber:    line,
		ImportedNames: names,
	}
}

// extractExport handles re-export statements (exports with a source clause):
//   - export { handler } from './handler'
//   - export * from './types'
//   - export * as ns from './types'
//   - export type { Foo } from './foo'
func extractExport(node *sitter.Node, src []byte) *model.ImportFact {
	// Only re-exports have a source string — plain exports don't.
	source := findChildByType(node, "string")
	if source == nil {
		return nil
	}
	spec := stripQuotes(nodeText(source, src))
	if spec == "" {
		return nil
	}

	line := int(node.StartPoint().Row) + 1
	var names []string

	// Check for export * (namespace_export or bare *)
	if findChildByType(node, "namespace_export") != nil {
		names = []string{"*"}
	} else if clause := findChildByType(node, "export_clause"); clause != nil {
		names = extractExportClauseNames(clause, src)
	} else {
		// export * from '...' — the * is not wrapped in namespace_export in some grammars
		for i := 0; i < int(node.ChildCount()); i++ {
			c := node.Child(i)
			if nodeText(c, src) == "*" {
				names = []string{"*"}
				break
			}
		}
	}

	return &model.ImportFact{
		Specifier:     spec,
		ImportType:    classifySpecifier(spec),
		LineNumber:    line,
		ImportedNames: names,
	}
}

// extractCallExpressions recursively walks the AST to find require() and dynamic import() calls.
func extractCallExpressions(node *sitter.Node, src []byte) []model.ImportFact {
	var facts []model.ImportFact
	walkTree(node, func(n *sitter.Node) {
		if n.Type() != "call_expression" {
			return
		}

		fn := n.ChildByFieldName("function")
		if fn == nil {
			return
		}
		fnName := nodeText(fn, src)

		// require('...')
		if fnName == "require" {
			if f := extractCallArg(n, src); f != nil {
				facts = append(facts, *f)
			}
			return
		}

		// dynamic import('...')
		if fn.Type() == "import" || fnName == "import" {
			if f := extractCallArg(n, src); f != nil {
				facts = append(facts, *f)
			}
		}
	})
	return facts
}

// extractCallArg extracts the string argument from a call_expression (require or import).
func extractCallArg(callNode *sitter.Node, src []byte) *model.ImportFact {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return nil
	}
	// The first named child of arguments should be the string literal.
	if args.NamedChildCount() == 0 {
		return nil
	}
	arg := args.NamedChild(0)
	if arg.Type() != "string" {
		return nil
	}
	spec := stripQuotes(nodeText(arg, src))
	if spec == "" {
		return nil
	}

	line := int(callNode.StartPoint().Row) + 1
	return &model.ImportFact{
		Specifier:  spec,
		ImportType: classifySpecifier(spec),
		LineNumber: line,
	}
}

// extractImportClauseNames walks an import_clause to collect imported names.
// Handles: default imports, named imports ({ a, b }), namespace imports (* as x),
// type-only imports, and inline type annotations.
func extractImportClauseNames(clause *sitter.Node, src []byte) []string {
	var names []string

	for i := 0; i < int(clause.NamedChildCount()); i++ {
		child := clause.NamedChild(i)
		switch child.Type() {
		case "identifier":
			// Default import: import foo from '...'
			names = append(names, nodeText(child, src))
		case "namespace_import":
			// import * as utils from '...'
			names = append(names, "*")
		case "named_imports":
			// import { a, b, type c } from '...'
			names = append(names, extractNamedImports(child, src)...)
		}
	}

	// If no named children matched (e.g., `import type Foo from '...'`),
	// check for a standalone identifier or type keyword pattern.
	if len(names) == 0 {
		// Walk all children (including unnamed) for identifiers after 'type'.
		for i := 0; i < int(clause.ChildCount()); i++ {
			c := clause.Child(i)
			if c.Type() == "identifier" {
				names = append(names, nodeText(c, src))
			}
		}
	}

	return names
}

// extractNamedImports extracts names from a named_imports node ({ a, b as c, type d }).
func extractNamedImports(node *sitter.Node, src []byte) []string {
	var names []string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "import_specifier" {
			name := extractImportSpecifierName(child, src)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// extractImportSpecifierName gets the original name from an import_specifier.
// For `a as b`, returns "a". For `type a`, returns "a". For `a`, returns "a".
func extractImportSpecifierName(node *sitter.Node, src []byte) string {
	// The "name" field holds the imported (original) name.
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		text := nodeText(nameNode, src)
		// Skip if the name is the "type" keyword — the real name is the alias field.
		if text == "type" {
			aliasNode := node.ChildByFieldName("alias")
			if aliasNode != nil {
				return nodeText(aliasNode, src)
			}
			// If no alias, try to find the next identifier.
			for i := 0; i < int(node.NamedChildCount()); i++ {
				c := node.NamedChild(i)
				if c.Type() == "identifier" && nodeText(c, src) != "type" {
					return nodeText(c, src)
				}
			}
			return ""
		}
		return text
	}
	// Fallback: first identifier child.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c := node.NamedChild(i)
		if c.Type() == "identifier" {
			return nodeText(c, src)
		}
	}
	return ""
}

// extractExportClauseNames extracts names from an export_clause ({ a, b as c }).
func extractExportClauseNames(clause *sitter.Node, src []byte) []string {
	var names []string
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		child := clause.NamedChild(i)
		if child.Type() == "export_specifier" {
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				text := nodeText(nameNode, src)
				if text != "type" {
					names = append(names, text)
				} else {
					// Inline type export: export { type Foo } from '...'
					aliasNode := child.ChildByFieldName("alias")
					if aliasNode != nil {
						names = append(names, nodeText(aliasNode, src))
					} else {
						for j := 0; j < int(child.NamedChildCount()); j++ {
							c := child.NamedChild(j)
							if c.Type() == "identifier" && nodeText(c, src) != "type" {
								names = append(names, nodeText(c, src))
								break
							}
						}
					}
				}
			}
		}
	}
	return names
}

// ExtractDefinitions returns signature strings for the given symbol names
// using tree-sitter to find function, class, interface, type alias, and variable declarations.
func (e *Extractor) ExtractDefinitions(content []byte, symbolNames []string) []string {
	if len(symbolNames) == 0 {
		return nil
	}

	nameSet := make(map[string]bool, len(symbolNames))
	for _, n := range symbolNames {
		nameSet[n] = true
	}

	tree, err := e.parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil
	}
	defer tree.Close()

	root := tree.RootNode()
	var defs []string

	// Walk top-level declarations.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if sig := extractDefinition(child, content, nameSet); sig != "" {
			defs = append(defs, sig)
		}
		// Handle export_statement wrapping a declaration.
		if child.Type() == "export_statement" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				inner := child.NamedChild(j)
				if sig := extractDefinition(inner, content, nameSet); sig != "" {
					defs = append(defs, sig)
				}
			}
		}
	}

	return defs
}

// extractDefinition checks if a node defines one of the target symbols and returns its signature.
func extractDefinition(node *sitter.Node, src []byte, targets map[string]bool) string {
	switch node.Type() {
	case "function_declaration", "generator_function_declaration":
		name := node.ChildByFieldName("name")
		if name != nil && targets[nodeText(name, src)] {
			return extractSignatureLine(node, src)
		}

	case "class_declaration":
		name := node.ChildByFieldName("name")
		if name != nil && targets[nodeText(name, src)] {
			return extractClassSignature(node, src)
		}

	case "interface_declaration":
		name := node.ChildByFieldName("name")
		if name != nil && targets[nodeText(name, src)] {
			return extractSignatureLine(node, src)
		}

	case "type_alias_declaration":
		name := node.ChildByFieldName("name")
		if name != nil && targets[nodeText(name, src)] {
			return extractor.TruncateSig(extractSingleLine(node, src), tsMaxSignatureLength)
		}

	case "lexical_declaration":
		// const/let/var declarations: const Foo = ...
		for j := 0; j < int(node.NamedChildCount()); j++ {
			declarator := node.NamedChild(j)
			if declarator.Type() == "variable_declarator" {
				name := declarator.ChildByFieldName("name")
				if name != nil && targets[nodeText(name, src)] {
					return extractor.TruncateSig(extractSingleLine(node, src), tsMaxSignatureLength)
				}
			}
		}

	case "enum_declaration":
		name := node.ChildByFieldName("name")
		if name != nil && targets[nodeText(name, src)] {
			return extractSignatureLine(node, src)
		}
	}

	return ""
}

// extractSignatureLine returns the first line of a node's text (the signature),
// with the body opener " {" removed and length capped.
func extractSignatureLine(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	if idx := strings.Index(text, "\n"); idx >= 0 {
		text = text[:idx]
	}
	text = extractor.TrimBodyOpener(strings.TrimSpace(text))
	return extractor.TruncateSig(text, tsMaxSignatureLength)
}

// extractClassSignature returns "class Name extends Base implements I1, I2".
func extractClassSignature(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	// Take up to the opening brace.
	if idx := strings.Index(text, "{"); idx >= 0 {
		text = text[:idx]
	}
	// Collapse any newlines into single space.
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	return extractor.TruncateSig(text, tsMaxSignatureLength)
}

// extractSingleLine returns the first line of a node.
func extractSingleLine(node *sitter.Node, src []byte) string {
	text := nodeText(node, src)
	if idx := strings.Index(text, "\n"); idx >= 0 {
		text = text[:idx]
	}
	return strings.TrimSpace(text)
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

// nodeText returns the source text of a tree-sitter node.
func nodeText(node *sitter.Node, src []byte) string {
	return string(src[node.StartByte():node.EndByte()])
}

// findChildByType returns the first child of node with the given type, or nil.
func findChildByType(node *sitter.Node, typeName string) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(i)
		if c.Type() == typeName {
			return c
		}
	}
	return nil
}

// walkTree recursively visits every node in the tree, calling fn for each.
func walkTree(node *sitter.Node, fn func(*sitter.Node)) {
	fn(node)
	for i := 0; i < int(node.ChildCount()); i++ {
		walkTree(node.Child(i), fn)
	}
}
