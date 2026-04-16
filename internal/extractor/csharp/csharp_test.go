package csharp

import (
	"testing"
)

func TestSingleUsing(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp;

using System.Collections.Generic;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// 1 import + 1 synthetic same_namespace = 2 facts
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if facts[0].Specifier != "System.Collections.Generic" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "System.Collections.Generic")
	}
	if facts[0].ImportType != "absolute" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "absolute")
	}
	assertNames(t, facts[0].ImportedNames, []string{"*"})
	// Verify same_namespace fact.
	if facts[1].ImportType != "same_namespace" {
		t.Errorf("expected same_namespace fact, got %q", facts[1].ImportType)
	}
	if facts[1].Specifier != "MyApp.*" {
		t.Errorf("same_namespace specifier = %q, want %q", facts[1].Specifier, "MyApp.*")
	}
}

func TestStaticUsing(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp;

using static System.Math;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_namespace), got %d", len(facts))
	}
	if facts[0].Specifier != "System.Math" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "System.Math")
	}
	if facts[0].ImportType != "static" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "static")
	}
	assertNames(t, facts[0].ImportedNames, []string{"*"})
}

func TestAliasUsing(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp;

using Dict = System.Collections.Generic.Dictionary;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_namespace), got %d", len(facts))
	}
	if facts[0].Specifier != "System.Collections.Generic.Dictionary" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "System.Collections.Generic.Dictionary")
	}
	if facts[0].ImportType != "alias" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "alias")
	}
	assertNames(t, facts[0].ImportedNames, []string{"Dict"})
}

func TestGlobalUsing(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("GlobalUsings.cs", []byte(`global using System.Linq;
global using static System.Console;

public class App {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// 2 imports, no namespace = 2 facts (no same_namespace fact since no namespace declared)
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if facts[0].Specifier != "System.Linq" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "System.Linq")
	}
	if facts[0].ImportType != "absolute" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "absolute")
	}
	if facts[1].Specifier != "System.Console" {
		t.Errorf("specifier = %q, want %q", facts[1].Specifier, "System.Console")
	}
	if facts[1].ImportType != "static" {
		t.Errorf("importType = %q, want %q", facts[1].ImportType, "static")
	}
}

func TestGlobalUsingAlias(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("GlobalUsings.cs", []byte(`global using Env = System.Environment;

public class App {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "System.Environment" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "System.Environment")
	}
	if facts[0].ImportType != "alias" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "alias")
	}
	assertNames(t, facts[0].ImportedNames, []string{"Env"})
}

func TestMultipleUsings(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp.Services;

using System.Collections.Generic;
using System.Linq;
using MyApp.Models;
using static System.Math;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// 4 imports + 1 same_namespace = 5
	if len(facts) != 5 {
		t.Fatalf("expected 5 facts (4 imports + 1 same_namespace), got %d", len(facts))
	}

	specs := make(map[string]bool)
	for _, f := range facts {
		specs[f.Specifier] = true
	}
	for _, want := range []string{"System.Collections.Generic", "System.Linq", "MyApp.Models", "System.Math"} {
		if !specs[want] {
			t.Errorf("missing import %q", want)
		}
	}
}

func TestStopsAtClassDeclaration(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp;

using System.Linq;

public class Foo {
    // using System.IO; -- should NOT be parsed
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_namespace, stops at class), got %d", len(facts))
	}
}

func TestStopsAtStruct(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Point.cs", []byte(`namespace MyApp;

using System;

public struct Point {
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_namespace), got %d", len(facts))
	}
}

func TestStopsAtInterface(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("IHandler.cs", []byte(`namespace MyApp;

using System;

public interface IHandler {
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_namespace), got %d", len(facts))
	}
}

func TestStopsAtRecord(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Person.cs", []byte(`namespace MyApp;

using System;

public record Person(string Name, int Age);
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts (1 import + 1 same_namespace), got %d", len(facts))
	}
}

func TestSkipsComments(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp;

// This is a comment
/* Block comment */
using System.Linq;

/**
 * XML doc comment
 * using System.IO;
 */
using System.Text;

public class Foo {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts (2 imports + 1 same_namespace), got %d", len(facts))
	}
}

func TestEmptyFile(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Empty.cs", []byte(`namespace MyApp;

public class Empty {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// 0 imports + 1 same_namespace = 1 fact
	if len(facts) != 1 {
		t.Errorf("expected 1 fact (same_namespace only), got %d", len(facts))
	}
	if len(facts) > 0 && facts[0].ImportType != "same_namespace" {
		t.Errorf("expected same_namespace fact, got %q", facts[0].ImportType)
	}
}

func TestNamespaceEmission(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp.Services.Auth;

public class AuthService {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact (same_namespace), got %d", len(facts))
	}
	if facts[0].Specifier != "MyApp.Services.Auth.*" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "MyApp.Services.Auth.*")
	}
	if facts[0].ImportType != "same_namespace" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "same_namespace")
	}
}

func TestFileScopedNamespace(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp.Models;

using System.ComponentModel;

public class User {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	// same_namespace fact should use the file-scoped namespace.
	if facts[1].Specifier != "MyApp.Models.*" {
		t.Errorf("same_namespace specifier = %q, want %q", facts[1].Specifier, "MyApp.Models.*")
	}
}

func TestBlockNamespace(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Foo.cs", []byte(`namespace MyApp.Models
{
    using System.ComponentModel;

    public class User {}
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if facts[1].Specifier != "MyApp.Models.*" {
		t.Errorf("same_namespace specifier = %q, want %q", facts[1].Specifier, "MyApp.Models.*")
	}
}

func TestNoNamespace(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("Program.cs", []byte(`using System;

Console.WriteLine("Hello");
`))
	if err != nil {
		t.Fatal(err)
	}
	// 1 import, no namespace = no same_namespace fact
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].ImportType != "absolute" {
		t.Errorf("importType = %q, want %q", facts[0].ImportType, "absolute")
	}
}

func TestExtensions(t *testing.T) {
	ext := New()
	exts := ext.Extensions()
	if len(exts) != 1 || exts[0] != ".cs" {
		t.Errorf("Extensions() = %v, want [\".cs\"]", exts)
	}
}

func TestLanguage(t *testing.T) {
	ext := New()
	if ext.Language() != "csharp" {
		t.Errorf("Language() = %q, want %q", ext.Language(), "csharp")
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
			content:  "public class UserService : IUserService {",
			symbols:  []string{"UserService"},
			expected: []string{"public class UserService : IUserService"},
		},
		{
			name:     "interface",
			content:  "public interface IHandler {",
			symbols:  []string{"IHandler"},
			expected: []string{"public interface IHandler"},
		},
		{
			name:     "struct",
			content:  "public struct Point {",
			symbols:  []string{"Point"},
			expected: []string{"public struct Point"},
		},
		{
			name:     "enum",
			content:  "public enum Status { Active, Inactive }",
			symbols:  []string{"Status"},
			expected: []string{"public enum Status"},
		},
		{
			name:     "record",
			content:  "public record Person(string Name, int Age) {",
			symbols:  []string{"Person"},
			expected: []string{"public record Person(string Name, int Age)"},
		},
		{
			name:     "method",
			content:  "public async Task<User> GetUserAsync(int id) {",
			symbols:  []string{"GetUserAsync"},
			expected: []string{"public async Task<User> GetUserAsync(int id)"},
		},
		{
			name:     "property",
			content:  "public string Name { get; set; }",
			symbols:  []string{"Name"},
			expected: []string{"public string Name"},
		},
		{
			name:     "delegate",
			content:  "public delegate void EventHandler(object sender, EventArgs e);",
			symbols:  []string{"EventHandler"},
			expected: []string{"public delegate void EventHandler(object sender, EventArgs e);"},
		},
		{
			name:     "const",
			content:  `public const string Version = "1.0";`,
			symbols:  []string{"Version"},
			expected: []string{`public const string Version = "1.0";`},
		},
		{
			name:     "readonly",
			content:  "public static readonly int MaxRetries = 3;",
			symbols:  []string{"MaxRetries"},
			expected: []string{"public static readonly int MaxRetries = 3;"},
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
		{"System.Collections.Generic", true},
		{"System.Linq", true},
		{"System", true},
		{"Microsoft.Extensions.DependencyInjection", true},
		{"Microsoft", true},
		{"Windows.UI.Xaml", true},
		{"MyApp.Models.User", false},
		{"Newtonsoft.Json", false},
		{"NUnit.Framework", false},
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
