package schema

// ChangeReport is the output of analyze-change: a PR-level impact report.
type ChangeReport struct {
	SchemaVersion string `json:"schema_version"`

	// The git ref range analyzed (e.g., "main..HEAD").
	RefRange string `json:"ref_range"`

	// Files changed in the diff.
	ChangedFiles []ChangedFile `json:"changed_files"`

	// Summary statistics about the change.
	Summary ChangeSummary `json:"summary"`

	// Aggregated blast radius across all changed files.
	BlastRadius *BlastRadius `json:"blast_radius"`

	// Combined must_read: files required to understand all changes.
	MustRead []MustReadEntry `json:"must_read"`

	// Combined likely_modify: additional files likely needing changes.
	LikelyModify map[string][]LikelyModifyEntry `json:"likely_modify"`

	// Combined test coverage for all changed files.
	Tests []TestEntry `json:"tests"`

	// Cross-file coupling: pairs of changed files that are structurally connected.
	Coupling []CouplingPair `json:"coupling,omitempty"`

	// Aggregated hotspots across all changed files.
	Hotspots []string `json:"hotspots,omitempty"`

	// Circular dependencies involving changed files.
	CircularDeps [][]string `json:"circular_deps,omitempty"`

	// Test coverage gaps: changed files with no direct test coverage.
	TestGaps []string `json:"test_gaps,omitempty"`

	// Hidden coupling: co-change partners not in the diff.
	HiddenCoupling []HiddenCouplingEntry `json:"hidden_coupling,omitempty"`

	// Index stats at time of analysis.
	Stats *IndexStats `json:"stats,omitempty"`
}

// ChangedFile represents a single file in the diff with its individual analysis.
type ChangedFile struct {
	File   string `json:"file"`
	Status string `json:"status"`

	// Per-file blast radius (nil for deleted/unindexed files).
	BlastRadius *BlastRadius `json:"blast_radius,omitempty"`

	// Whether this file is indexed (new files may not be).
	Indexed bool `json:"indexed"`
}

// ChangeSummary provides aggregate statistics about the change.
type ChangeSummary struct {
	TotalFiles    int `json:"total_files"`
	Added         int `json:"added"`
	Modified      int `json:"modified"`
	Deleted       int `json:"deleted"`
	Renamed       int `json:"renamed"`
	IndexedFiles  int `json:"indexed_files"`
	TestFiles     int `json:"test_files"`
	HighRiskFiles int `json:"high_risk_files"`
}

// CouplingPair represents two changed files that are structurally connected.
type CouplingPair struct {
	FileA     string `json:"file_a"`
	FileB     string `json:"file_b"`
	Direction string `json:"direction"`
}

// HiddenCouplingEntry represents a co-change partner not in the diff.
type HiddenCouplingEntry struct {
	ChangedFile string `json:"changed_file"`
	Partner     string `json:"partner"`
	Frequency   int    `json:"frequency"`
}
