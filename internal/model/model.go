// Package model defines shared types used across the contextception engine.
package model

import "github.com/kehoej/contextception/schema"

// ImportFact represents a single import extracted from a source file.
type ImportFact struct {
	Specifier     string   // e.g. "foo.bar" or ".utils"
	ImportType    string   // "absolute", "relative"
	LineNumber    int
	ImportedNames []string // e.g. ["baz", "qux"] for "from foo.bar import baz, qux"
}

// ResolveResult represents the outcome of resolving an import.
type ResolveResult struct {
	ResolvedPath     string // repo-relative path, empty if external
	External         bool
	ResolutionMethod string // "relative", "package_local", "external"
	Reason           string // explanation when unresolved
}

// Type aliases — all existing code continues using model.X unchanged.
type ExternalDependency = schema.ExternalDependency
type MustReadEntry = schema.MustReadEntry
type TestEntry = schema.TestEntry
type BlastRadius = schema.BlastRadius
type AnalysisOutput = schema.AnalysisOutput
type IndexStats = schema.IndexStats
type LikelyModifyEntry = schema.LikelyModifyEntry
type RelatedEntry = schema.RelatedEntry
