// Package grader implements automated grading of contextception analysis output
// against the testing-tracker rubric.
package grader

import "time"

// FileGrade holds per-file grading results across all rubric sections.
type FileGrade struct {
	File         string   `json:"file"`
	Archetype    string   `json:"archetype"`
	MustRead     float64  `json:"must_read"`      // 1-4
	LikelyModify float64  `json:"likely_modify"`   // 1-4
	Tests        float64  `json:"tests"`           // 1-4
	Related      float64  `json:"related"`         // 1-4
	BlastRadius  float64  `json:"blast_radius"`    // 1-4
	Overall      float64  `json:"overall"`         // weighted average
	Notes        []string `json:"notes,omitempty"` // explanation for each grade
}

// SectionGrades holds per-section averages across all files.
type SectionGrades struct {
	MustRead     float64 `json:"must_read"`
	LikelyModify float64 `json:"likely_modify"`
	Tests        float64 `json:"tests"`
	Related      float64 `json:"related"`
	BlastRadius  float64 `json:"blast_radius"`
}

// Issue represents an auto-detected problem in analysis output.
type Issue struct {
	Severity string `json:"severity"` // "error", "warning", "info"
	File     string `json:"file,omitempty"`
	Section  string `json:"section,omitempty"`
	Message  string `json:"message"`
}

// FeatureTest represents a single feature matrix test result.
type FeatureTest struct {
	Group    string `json:"group"`    // e.g., "modes", "token_budget", "caps"
	Name     string `json:"name"`
	Status   string `json:"status"`   // "pass", "fail", "skip"
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Duration int64  `json:"duration_ms"`
	Error    string `json:"error,omitempty"`
}

// TestReport is the top-level report for a full test suite run.
type TestReport struct {
	RepoID       string        `json:"repo_id"`
	RepoURL      string        `json:"repo_url"`
	Timestamp    time.Time     `json:"timestamp"`
	Language     string        `json:"language"`
	FileCount    int           `json:"file_count"`
	EdgeCount    int           `json:"edge_count"`
	IndexTimeMs  int64         `json:"index_time_ms"`
	Files        []FileGrade   `json:"files"`
	Sections     SectionGrades `json:"sections"`
	Overall      float64       `json:"overall"`
	LetterGrade  string        `json:"letter_grade"`
	Issues       []Issue       `json:"issues"`
	FeatureTests []FeatureTest        `json:"feature_tests,omitempty"`
	Archetypes   []ArchetypeCandidate `json:"archetypes,omitempty"`
}

// LetterGradeFromScore converts a numeric grade (1-4) to a letter grade.
func LetterGradeFromScore(score float64) string {
	switch {
	case score >= 3.5:
		return "A"
	case score >= 2.5:
		return "B"
	case score >= 1.5:
		return "C"
	default:
		return "D"
	}
}

// ComputeOverall computes the weighted overall score for a file grade.
// Weights: must_read=0.40, likely_modify=0.20, tests=0.15, related=0.15, blast_radius=0.10.
func (fg *FileGrade) ComputeOverall() {
	fg.Overall = fg.MustRead*0.40 + fg.LikelyModify*0.20 +
		fg.Tests*0.15 + fg.Related*0.15 + fg.BlastRadius*0.10
}

// ComputeSections computes section averages from file grades.
func ComputeSections(files []FileGrade) SectionGrades {
	if len(files) == 0 {
		return SectionGrades{}
	}
	var sg SectionGrades
	for _, f := range files {
		sg.MustRead += f.MustRead
		sg.LikelyModify += f.LikelyModify
		sg.Tests += f.Tests
		sg.Related += f.Related
		sg.BlastRadius += f.BlastRadius
	}
	n := float64(len(files))
	sg.MustRead /= n
	sg.LikelyModify /= n
	sg.Tests /= n
	sg.Related /= n
	sg.BlastRadius /= n
	return sg
}

// ComputeReportOverall computes the overall grade for a full report.
func ComputeReportOverall(files []FileGrade) float64 {
	if len(files) == 0 {
		return 0
	}
	var sum float64
	for _, f := range files {
		sum += f.Overall
	}
	return sum / float64(len(files))
}
