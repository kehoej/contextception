package java

import (
	"testing"
)

func TestSingleImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.java", []byte(`package com.example;

import java.util.List;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// 1 import + 1 synthetic same_package = 2 facts
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if facts[0].Specifier != "java.util.List" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "java.util.List")
	}
	assertNames(t, facts[0].ImportedNames, []string{"List"})
	// Verify same_package fact.
	if facts[1].ImportType != "same_package" {
		t.Errorf("expected same_package fact, got %q", facts[1].ImportType)
	}
	if facts[1].Specifier != "com.example.*" {
		t.Errorf("same_package specifier = %q, want %q", facts[1].Specifier, "com.example.*")
	}
}

func TestWildcardImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.java", []byte(`package com.example;

import java.util.*;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_package), got %d", len(facts))
	}
	if facts[0].Specifier != "java.util.*" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "java.util.*")
	}
	assertNames(t, facts[0].ImportedNames, []string{"*"})
}

func TestStaticImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.java", []byte(`package com.example;

import static java.lang.Math.abs;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_package), got %d", len(facts))
	}
	if facts[0].Specifier != "java.lang.Math.abs" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "java.lang.Math.abs")
	}
	if facts[0].ImportType != "static" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "static")
	}
	assertNames(t, facts[0].ImportedNames, []string{"abs"})
}

func TestStaticWildcardImport(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.java", []byte(`package com.example;

import static java.lang.Math.*;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_package), got %d", len(facts))
	}
	if facts[0].Specifier != "java.lang.Math.*" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "java.lang.Math.*")
	}
}

func TestMultipleImports(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.java", []byte(`package com.example.app;

import java.util.List;
import java.util.Map;
import com.example.model.User;
import static org.junit.Assert.assertEquals;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// 4 imports + 1 same_package = 5
	if len(facts) != 5 {
		t.Fatalf("expected 5 facts (4 imports + 1 same_package), got %d", len(facts))
	}

	specs := make(map[string]bool)
	for _, f := range facts {
		specs[f.Specifier] = true
	}
	for _, want := range []string{"java.util.List", "java.util.Map", "com.example.model.User", "org.junit.Assert.assertEquals"} {
		if !specs[want] {
			t.Errorf("missing import %q", want)
		}
	}
}

func TestStopsAtClassDeclaration(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.java", []byte(`package com.example;

import java.util.List;

public class Foo {
    // import java.util.Map; -- should NOT be parsed
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_package, stops at class), got %d", len(facts))
	}
}

func TestStopsAtInterface(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Handler.java", []byte(`package com.example;

import java.util.List;

public interface Handler {
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_package), got %d", len(facts))
	}
}

func TestSkipsComments(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.java", []byte(`package com.example;

// This is a comment
/* Block comment */
import java.util.List;

/**
 * Javadoc comment
 * import java.util.Map;
 */
import java.io.File;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts (2 imports + 1 same_package), got %d", len(facts))
	}
}

func TestEmptyFile(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Empty.java", []byte(`package com.example;

public class Empty {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// 0 imports + 1 same_package = 1 fact
	if len(facts) != 1 {
		t.Errorf("expected 1 fact (same_package only), got %d", len(facts))
	}
	if len(facts) > 0 && facts[0].ImportType != "same_package" {
		t.Errorf("expected same_package fact, got %q", facts[0].ImportType)
	}
}

func TestExtensions(t *testing.T) {
	ext := New()
	exts := ext.Extensions()
	if len(exts) != 1 || exts[0] != ".java" {
		t.Errorf("Extensions() = %v, want [\".java\"]", exts)
	}
}

func TestLanguage(t *testing.T) {
	ext := New()
	if ext.Language() != "java" {
		t.Errorf("Language() = %q, want %q", ext.Language(), "java")
	}
}

func TestExtractDefinitions(t *testing.T) {
	ext := New()

	cases := []struct {
		name     string
		content  string
		symbols  []string
		expected []string
	}{
		{
			name:     "class",
			content:  "public class Foo extends Bar implements Baz {",
			symbols:  []string{"Foo"},
			expected: []string{"public class Foo extends Bar implements Baz"},
		},
		{
			name:     "interface",
			content:  "public interface Handler {",
			symbols:  []string{"Handler"},
			expected: []string{"public interface Handler"},
		},
		{
			name:     "enum",
			content:  "public enum Status { ACTIVE, INACTIVE }",
			symbols:  []string{"Status"},
			expected: []string{"public enum Status"},
		},
		{
			name:     "constant",
			content:  `public static final String VERSION = "1.0";`,
			symbols:  []string{"VERSION"},
			expected: []string{`public static final String VERSION = "1.0";`},
		},
		{
			name:     "method",
			content:  "public void processOrder(Order order) {",
			symbols:  []string{"processOrder"},
			expected: []string{"public void processOrder(Order order)"},
		},
		{
			name:     "no match",
			content:  "public class Foo {}",
			symbols:  []string{"nope"},
			expected: nil,
		},
		{
			name:     "empty symbols",
			content:  "public class Foo {}",
			symbols:  nil,
			expected: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ext.ExtractDefinitions([]byte(tc.content), tc.symbols)
			if tc.expected == nil {
				if len(got) != 0 {
					t.Errorf("expected nil/empty, got %v", got)
				}
				return
			}
			assertNames(t, got, tc.expected)
		})
	}
}

func TestIsStdlib(t *testing.T) {
	cases := []struct {
		spec string
		want bool
	}{
		{"java.util.List", true},
		{"javax.servlet.http.HttpServlet", true},
		{"sun.misc.Unsafe", true},
		{"jdk.internal.misc.Unsafe", true},
		{"com.example.Foo", false},
		{"org.springframework.boot.Application", false},
	}
	for _, tc := range cases {
		if got := IsStdlib(tc.spec); got != tc.want {
			t.Errorf("IsStdlib(%q) = %v, want %v", tc.spec, got, tc.want)
		}
	}
}

func assertNames(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("length = %d, want %d (%v vs %v)", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
