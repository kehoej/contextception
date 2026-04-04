// Package schema defines the stable JSON output types for contextception.
// External consumers (e.g. the GitHub PR app) import this package
// to deserialize CLI JSON output without depending on internal packages.
package schema

// AnalysisOutput is the top-level output of the analysis engine (v3.2).
type AnalysisOutput struct {
	SchemaVersion  string                         `json:"schema_version"`
	Subject        string                         `json:"subject"`
	Confidence     float64                        `json:"confidence"`
	ConfidenceNote string                         `json:"confidence_note,omitempty"`
	External       []string                       `json:"external"`
	MustRead       []MustReadEntry                `json:"must_read"`
	MustReadNote   string                         `json:"must_read_note,omitempty"`
	LikelyModify     map[string][]LikelyModifyEntry `json:"likely_modify"`
	LikelyModifyNote string                         `json:"likely_modify_note,omitempty"`
	Tests            []TestEntry                    `json:"tests"`
	TestsNote        string                         `json:"tests_note,omitempty"`
	Related        map[string][]RelatedEntry      `json:"related"`
	RelatedNote    string                         `json:"related_note,omitempty"`
	BlastRadius    *BlastRadius                   `json:"blast_radius,omitempty"`
	Hotspots       []string                       `json:"hotspots,omitempty"`
	CircularDeps   [][]string                     `json:"circular_deps,omitempty"`
	Stats          *IndexStats                    `json:"stats,omitempty"`
}

// MustReadEntry represents a file in the must_read list with optional symbols and stable flag.
type MustReadEntry struct {
	File        string   `json:"file"`
	Symbols     []string `json:"symbols,omitempty"`
	Definitions []string `json:"definitions,omitempty"`
	Stable      bool     `json:"stable,omitempty"`
	Direction   string   `json:"direction,omitempty"`
	Role        string   `json:"role,omitempty"`
	Circular    bool     `json:"circular,omitempty"`
}

// TestEntry represents a test file with its tier (direct vs dependency).
type TestEntry struct {
	File   string `json:"file"`
	Direct bool   `json:"direct"`
}

// BlastRadius summarizes the overall risk profile of a change.
type BlastRadius struct {
	Level     string  `json:"level"`
	Detail    string  `json:"detail"`
	Fragility float64 `json:"fragility,omitempty"`
}

// IndexStats provides index health metadata at analysis time.
type IndexStats struct {
	TotalFiles      int `json:"total_files"`
	TotalEdges      int `json:"total_edges"`
	UnresolvedCount int `json:"unresolved_count"`
}

// LikelyModifyEntry represents a file likely to need modification.
type LikelyModifyEntry struct {
	File       string   `json:"file"`
	Confidence string   `json:"confidence"`
	Signals    []string `json:"signals"`
	Symbols    []string `json:"symbols,omitempty"`
	Role       string   `json:"role,omitempty"`
}

// RelatedEntry represents a related context file with its signals.
type RelatedEntry struct {
	File    string   `json:"file"`
	Signals []string `json:"signals"`
}

// ExternalDependency represents an import that resolved to an external package or stdlib.
type ExternalDependency struct {
	Specifier  string `json:"specifier"`
	Reason     string `json:"reason"`
	LineNumber int    `json:"line_number"`
}
