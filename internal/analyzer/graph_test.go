package analyzer

import (
	"testing"

	"github.com/kehoej/contextception/internal/db"
)

func TestEvidenceGate_DirectImport(t *testing.T) {
	subject := "pkg/subject.go"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
		"pkg/sibling.go": {
			Path: "pkg/sibling.go", Distance: 1,
			IsSamePackageSibling: true, IsGoSamePackage: true,
			IsImport: true, // has direct import edge
		},
	}
	filterSamePackageSiblings(candidates, subject)
	if _, ok := candidates["pkg/sibling.go"]; !ok {
		t.Error("sibling with direct import should be kept")
	}
}

func TestEvidenceGate_CoChange(t *testing.T) {
	subject := "pkg/subject.go"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
		"pkg/sibling.go": {
			Path: "pkg/sibling.go", Distance: 1,
			IsSamePackageSibling: true, IsGoSamePackage: true,
			CoChangeFreq: 3, // co-change >= 2
		},
	}
	filterSamePackageSiblings(candidates, subject)
	if _, ok := candidates["pkg/sibling.go"]; !ok {
		t.Error("sibling with co-change >= 2 should be kept")
	}
}

func TestEvidenceGate_PrefixMatch(t *testing.T) {
	subject := "pkg/subject.go"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
		"pkg/subject_helper.go": {
			Path: "pkg/subject_helper.go", Distance: 1,
			IsSamePackageSibling: true, IsGoSamePackage: true,
			HasPrefixMatch: true, // prefix match
		},
	}
	filterSamePackageSiblings(candidates, subject)
	if _, ok := candidates["pkg/subject_helper.go"]; !ok {
		t.Error("sibling with prefix match should be kept")
	}
}

func TestEvidenceGate_NoEvidence(t *testing.T) {
	subject := "pkg/subject.go"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
		"pkg/unrelated.go": {
			Path: "pkg/unrelated.go", Distance: 1,
			IsSamePackageSibling: true, IsGoSamePackage: true,
			// No import, no co-change, no prefix match.
		},
	}
	filterSamePackageSiblings(candidates, subject)
	if _, ok := candidates["pkg/unrelated.go"]; ok {
		t.Error("sibling with no evidence should be filtered out")
	}
}

func TestEvidenceGate_LargePackage(t *testing.T) {
	subject := "pkg/subject.go"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
	}
	// Add 25 siblings, only some with evidence.
	for i := 0; i < 25; i++ {
		name := "pkg/sibling_" + string(rune('a'+i)) + ".go"
		c := &candidate{
			Path: name, Distance: 1,
			IsSamePackageSibling: true, IsGoSamePackage: true,
		}
		switch i {
		case 0:
			c.IsImport = true // evidence
		case 1:
			c.CoChangeFreq = 5 // evidence
		}
		// rest have no evidence
		candidates[name] = c
	}
	filterSamePackageSiblings(candidates, subject)
	// Should keep subject + 2 with evidence, filter 23 without.
	kept := 0
	for p := range candidates {
		if p != subject {
			kept++
		}
	}
	if kept != 2 {
		t.Errorf("large package: kept=%d, want 2 (only evidence-gated)", kept)
	}
}

func TestEvidenceGate_JavaSamePackage(t *testing.T) {
	subject := "src/main/java/com/Foo.java"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
		"src/main/java/com/Bar.java": {
			Path: "src/main/java/com/Bar.java", Distance: 1,
			IsJavaSamePackage: true,
			HasJavaClassPrefix: true, // class prefix match = evidence
		},
		"src/main/java/com/Baz.java": {
			Path: "src/main/java/com/Baz.java", Distance: 1,
			IsJavaSamePackage: true,
			// no evidence
		},
	}
	filterSamePackageSiblings(candidates, subject)
	if _, ok := candidates["src/main/java/com/Bar.java"]; !ok {
		t.Error("Java sibling with class prefix should be kept")
	}
	if _, ok := candidates["src/main/java/com/Baz.java"]; ok {
		t.Error("Java sibling without evidence should be filtered")
	}
}

func TestEvidenceGate_RustSameModule(t *testing.T) {
	subject := "src/sync/mutex.rs"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
		"src/sync/rwlock.rs": {
			Path: "src/sync/rwlock.rs", Distance: 1,
			IsRustSameModule: true,
			IsImporter:       true, // has import edge
		},
		"src/sync/barrier.rs": {
			Path: "src/sync/barrier.rs", Distance: 1,
			IsRustSameModule: true,
			// no evidence
		},
	}
	filterSamePackageSiblings(candidates, subject)
	if _, ok := candidates["src/sync/rwlock.rs"]; !ok {
		t.Error("Rust sibling with import edge should be kept")
	}
	if _, ok := candidates["src/sync/barrier.rs"]; ok {
		t.Error("Rust sibling without evidence should be filtered")
	}
}

func TestEvidenceGate_NonSamePackageUntouched(t *testing.T) {
	subject := "pkg/subject.go"
	candidates := map[string]*candidate{
		subject: {Path: subject, Distance: 0},
		"other/dep.go": {
			Path: "other/dep.go", Distance: 1,
			IsImport: true,
			// NOT a same-package sibling — should not be filtered.
		},
		"other/consumer.go": {
			Path: "other/consumer.go", Distance: 2,
			Signals: db.SignalRow{Indegree: 5},
			// NOT a same-package sibling — should not be filtered.
		},
	}
	filterSamePackageSiblings(candidates, subject)
	if _, ok := candidates["other/dep.go"]; !ok {
		t.Error("non-same-package dep should not be filtered")
	}
	if _, ok := candidates["other/consumer.go"]; !ok {
		t.Error("non-same-package consumer should not be filtered")
	}
}
