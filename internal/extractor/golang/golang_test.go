package golang

import (
	"testing"
)

func TestSingleImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import "fmt"

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "fmt" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "fmt")
	}
	if facts[0].ImportType != "absolute" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "absolute")
	}
	assertNames(t, facts[0].ImportedNames, []string{"fmt"})
}

func TestGroupedImports(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(facts))
	}

	specs := map[string]bool{}
	for _, f := range facts {
		specs[f.Specifier] = true
	}
	for _, want := range []string{"fmt", "os", "strings"} {
		if !specs[want] {
			t.Errorf("missing import %q", want)
		}
	}
}

func TestNamedImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import myalias "github.com/foo/bar"

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "github.com/foo/bar" {
		t.Errorf("specifier = %q", facts[0].Specifier)
	}
	assertNames(t, facts[0].ImportedNames, []string{"myalias"})
}

func TestDotImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import . "testing"

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	assertNames(t, facts[0].ImportedNames, []string{"."})
}

func TestBlankImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import _ "net/http/pprof"

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	assertNames(t, facts[0].ImportedNames, []string{"_"})
}

func TestCgoSkipped(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

// #include <stdio.h>
import "C"
import "fmt"

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact (cgo skipped), got %d", len(facts))
	}
	if facts[0].Specifier != "fmt" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "fmt")
	}
}

func TestGroupedWithComments(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import (
	// Standard library
	"fmt"
	"os"

	// Third party
	"github.com/foo/bar"
)

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(facts))
	}
}

func TestGroupedWithAliases(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import (
	"fmt"
	mydb "github.com/kehoej/contextception/internal/db"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
)

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 4 {
		t.Fatalf("expected 4 facts, got %d", len(facts))
	}

	aliasMap := map[string]string{}
	for _, f := range facts {
		if len(f.ImportedNames) > 0 {
			aliasMap[f.Specifier] = f.ImportedNames[0]
		}
	}

	if aliasMap["fmt"] != "fmt" {
		t.Errorf("fmt alias = %q, want %q", aliasMap["fmt"], "fmt")
	}
	if aliasMap["github.com/kehoej/contextception/internal/db"] != "mydb" {
		t.Errorf("db alias = %q, want %q", aliasMap["github.com/kehoej/contextception/internal/db"], "mydb")
	}
	if aliasMap["github.com/lib/pq"] != "_" {
		t.Errorf("pq alias = %q, want %q", aliasMap["github.com/lib/pq"], "_")
	}
	if aliasMap["github.com/onsi/ginkgo"] != "." {
		t.Errorf("ginkgo alias = %q, want %q", aliasMap["github.com/onsi/ginkgo"], ".")
	}
}

func TestMultipleImportBlocks(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import "fmt"

import (
	"os"
	"strings"
)

func main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(facts))
	}
}

func TestStopsAtFunc(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import "fmt"

func main() {
	// This should not be parsed.
	// import "os"
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact (stops at func), got %d", len(facts))
	}
}

func TestStopsAtType(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import "fmt"

type Foo struct{}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
}

func TestPathLastElement(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.go", []byte(`package main

import "github.com/kehoej/contextception/internal/db"
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	assertNames(t, facts[0].ImportedNames, []string{"db"})
}

func TestExtensions(t *testing.T) {
	ext := New()
	exts := ext.Extensions()
	if len(exts) != 1 || exts[0] != ".go" {
		t.Errorf("Extensions() = %v, want [\".go\"]", exts)
	}
}

func TestLanguage(t *testing.T) {
	ext := New()
	if ext.Language() != "go" {
		t.Errorf("Language() = %q, want %q", ext.Language(), "go")
	}
}

func TestEmptyFile(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("empty.go", []byte(`package main
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts for empty file, got %d", len(facts))
	}
}

func assertNames(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("importedNames length = %d, want %d (%v vs %v)", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("importedNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtractDefinitionsGo(t *testing.T) {
	ext := New()

	cases := []struct {
		name     string
		content  string
		symbols  []string
		expected []string
	}{
		{
			name:     "func",
			content:  "func Foo(x int) error {",
			symbols:  []string{"Foo"},
			expected: []string{"func Foo(x int) error"},
		},
		{
			name:     "method",
			content:  "func (s *Server) Start() {",
			symbols:  []string{"Start"},
			expected: []string{"func (s *Server) Start()"},
		},
		{
			name:     "type struct",
			content:  "type Config struct {",
			symbols:  []string{"Config"},
			expected: []string{"type Config struct {"},
		},
		{
			name:     "type interface",
			content:  "type Handler interface {",
			symbols:  []string{"Handler"},
			expected: []string{"type Handler interface {"},
		},
		{
			name:     "var",
			content:  "var DefaultTimeout = 30",
			symbols:  []string{"DefaultTimeout"},
			expected: []string{"var DefaultTimeout = 30"},
		},
		{
			name:     "const",
			content:  `const Version = "1.0"`,
			symbols:  []string{"Version"},
			expected: []string{`const Version = "1.0"`},
		},
		{
			name:     "no match",
			content:  "func Foo() {}",
			symbols:  []string{"nope"},
			expected: nil,
		},
		{
			name:     "empty symbols",
			content:  "func Foo() {}",
			symbols:  nil,
			expected: nil,
		},
		{
			name:     "empty content",
			content:  "",
			symbols:  []string{"Foo"},
			expected: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ext.ExtractDefinitions([]byte(tc.content), tc.symbols)
			if tc.expected == nil {
				if len(got) != 0 {
					t.Errorf("expected nil/empty result, got %v", got)
				}
				return
			}
			assertNames(t, got, tc.expected)
		})
	}
}
