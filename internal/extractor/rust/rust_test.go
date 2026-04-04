package rust

import (
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestSimpleUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.rs", []byte(`use std::collections::HashMap;

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "std::collections::HashMap" {
		t.Errorf("specifier = %q", facts[0].Specifier)
	}
	if facts[0].ImportType != "absolute" {
		t.Errorf("importType = %q, want absolute", facts[0].ImportType)
	}
	assertNames(t, facts[0].ImportedNames, []string{"HashMap"})
}

func TestGroupedUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.rs", []byte(`use std::io::{Read, Write};

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// Grouped imports are now flattened into individual facts.
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if facts[0].Specifier != "std::io::Read" {
		t.Errorf("specifier[0] = %q, want %q", facts[0].Specifier, "std::io::Read")
	}
	if facts[1].Specifier != "std::io::Write" {
		t.Errorf("specifier[1] = %q, want %q", facts[1].Specifier, "std::io::Write")
	}
	assertNames(t, facts[0].ImportedNames, []string{"Read"})
	assertNames(t, facts[1].ImportedNames, []string{"Write"})
}

func TestCrateRelative(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use crate::models::User;

pub fn process() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].ImportType != "relative" {
		t.Errorf("importType = %q, want relative", facts[0].ImportType)
	}
	assertNames(t, facts[0].ImportedNames, []string{"User"})
}

func TestSuperUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("sub/mod.rs", []byte(`use super::utils;
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].ImportType != "relative" {
		t.Errorf("importType = %q, want relative", facts[0].ImportType)
	}
}

func TestSelfUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use self::helpers;
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].ImportType != "relative" {
		t.Errorf("importType = %q, want relative", facts[0].ImportType)
	}
}

func TestModDeclaration(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`mod config;
pub mod routes;

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	if facts[0].Specifier != "mod:config" {
		t.Errorf("specifier[0] = %q", facts[0].Specifier)
	}
	if facts[0].ImportType != "module" {
		t.Errorf("importType[0] = %q, want module", facts[0].ImportType)
	}
	if facts[1].Specifier != "mod:routes" {
		t.Errorf("specifier[1] = %q", facts[1].Specifier)
	}
}

func TestExternCrate(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`extern crate serde;

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "extern:serde" {
		t.Errorf("specifier = %q", facts[0].Specifier)
	}
	assertNames(t, facts[0].ImportedNames, []string{"serde"})
}

func TestPubUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`pub use crate::config::Config;
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	assertNames(t, facts[0].ImportedNames, []string{"Config"})
}

func TestMultipleImports(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.rs", []byte(`use std::collections::HashMap;
use std::io;
use crate::models::User;
mod config;

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 4 {
		t.Fatalf("expected 4 facts, got %d", len(facts))
	}
}

func TestSkipsComments(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`// use std::io;
/* use std::fs; */
use std::collections::HashMap;

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
}

func TestEmptyFile(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("empty.rs", []byte(`fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Errorf("expected 0 facts, got %d", len(facts))
	}
}

func TestExtensions(t *testing.T) {
	ext := New()
	exts := ext.Extensions()
	if len(exts) != 1 || exts[0] != ".rs" {
		t.Errorf("Extensions() = %v, want [\".rs\"]", exts)
	}
}

func TestLanguage(t *testing.T) {
	ext := New()
	if ext.Language() != "rust" {
		t.Errorf("Language() = %q, want %q", ext.Language(), "rust")
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
			name:     "fn",
			content:  "pub fn process(input: &str) -> Result<Output> {",
			symbols:  []string{"process"},
			expected: []string{"pub fn process(input: &str) -> Result<Output>"},
		},
		{
			name:     "struct",
			content:  "pub struct Config {",
			symbols:  []string{"Config"},
			expected: []string{"pub struct Config"},
		},
		{
			name:     "enum",
			content:  "pub enum Status {",
			symbols:  []string{"Status"},
			expected: []string{"pub enum Status"},
		},
		{
			name:     "trait",
			content:  "pub trait Handler {",
			symbols:  []string{"Handler"},
			expected: []string{"pub trait Handler"},
		},
		{
			name:     "const",
			content:  `pub const VERSION: &str = "1.0";`,
			symbols:  []string{"VERSION"},
			expected: []string{`pub const VERSION: &str = "1.0";`},
		},
		{
			name:     "async fn",
			content:  "pub async fn fetch(url: &str) -> Result<Response> {",
			symbols:  []string{"fetch"},
			expected: []string{"pub async fn fetch(url: &str) -> Result<Response>"},
		},
		{
			name:     "no match",
			content:  "pub fn foo() {}",
			symbols:  []string{"nope"},
			expected: nil,
		},
		{
			name:     "empty symbols",
			content:  "pub fn foo() {}",
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
		{"std::collections::HashMap", true},
		{"core::fmt", true},
		{"alloc::vec::Vec", true},
		{"serde::Deserialize", false},
		{"crate::models::User", false},
	}
	for _, tc := range cases {
		if got := IsStdlib(tc.spec); got != tc.want {
			t.Errorf("IsStdlib(%q) = %v, want %v", tc.spec, got, tc.want)
		}
	}
}

// --- Multi-line use statement tests ---

func TestMultiLineUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use crate::{
    change_detection::{MutUntyped, Ticks, TicksMut},
    error::Result,
    world::unsafe_world_cell::UnsafeWorldCell,
};

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// Should flatten to 5 individual facts.
	if len(facts) != 5 {
		t.Fatalf("expected 5 facts, got %d: %v", len(facts), factsSpecs(facts))
	}

	expected := []string{
		"crate::change_detection::MutUntyped",
		"crate::change_detection::Ticks",
		"crate::change_detection::TicksMut",
		"crate::error::Result",
		"crate::world::unsafe_world_cell::UnsafeWorldCell",
	}
	for i, want := range expected {
		if facts[i].Specifier != want {
			t.Errorf("facts[%d].Specifier = %q, want %q", i, facts[i].Specifier, want)
		}
		if facts[i].ImportType != "relative" {
			t.Errorf("facts[%d].ImportType = %q, want relative", i, facts[i].ImportType)
		}
	}
}

func TestMultiLineUseSimple(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("main.rs", []byte(`use std::io::{
    Read,
    Write,
};

fn main() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	if facts[0].Specifier != "std::io::Read" {
		t.Errorf("facts[0] = %q", facts[0].Specifier)
	}
	if facts[1].Specifier != "std::io::Write" {
		t.Errorf("facts[1] = %q", facts[1].Specifier)
	}
}

func TestMultiLineNestedGroups(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use crate::{
    a::{B, C},
    d::E,
};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	expected := []string{"crate::a::B", "crate::a::C", "crate::d::E"}
	for i, want := range expected {
		if facts[i].Specifier != want {
			t.Errorf("facts[%d] = %q, want %q", i, facts[i].Specifier, want)
		}
	}
}

func TestMultiLineWithComments(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use crate::{
    // This is a comment
    models::User,
    config::Config,
};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	if facts[0].Specifier != "crate::models::User" {
		t.Errorf("facts[0] = %q", facts[0].Specifier)
	}
	if facts[1].Specifier != "crate::config::Config" {
		t.Errorf("facts[1] = %q", facts[1].Specifier)
	}
}

func TestPubUseMultiLine(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`pub use crate::{
    App,
    Plugin,
};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	if facts[0].Specifier != "crate::App" {
		t.Errorf("facts[0] = %q", facts[0].Specifier)
	}
	if facts[1].Specifier != "crate::Plugin" {
		t.Errorf("facts[1] = %q", facts[1].Specifier)
	}
}

func TestPubCrateUseMultiLine(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`pub(crate) use crate::{
    models::User,
    config::Config,
};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	if facts[0].Specifier != "crate::models::User" {
		t.Errorf("facts[0] = %q", facts[0].Specifier)
	}
}

func TestSingleLineNestedGroups(t *testing.T) {
	ext := New()
	// This tests nested groups on a single line (which the old regex couldn't handle).
	facts, err := ext.Extract("lib.rs", []byte(`use crate::{a::{B, C}, d::E};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	expected := []string{"crate::a::B", "crate::a::C", "crate::d::E"}
	for i, want := range expected {
		if facts[i].Specifier != want {
			t.Errorf("facts[%d] = %q, want %q", i, facts[i].Specifier, want)
		}
	}
}

func TestSingleLineGroupedRegression(t *testing.T) {
	// Regression: single-line grouped imports must still work.
	ext := New()
	facts, err := ext.Extract("main.rs", []byte(`use crate::{App, Plugin};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	if facts[0].Specifier != "crate::App" {
		t.Errorf("facts[0] = %q", facts[0].Specifier)
	}
	if facts[1].Specifier != "crate::Plugin" {
		t.Errorf("facts[1] = %q", facts[1].Specifier)
	}
	if facts[0].ImportType != "relative" {
		t.Errorf("facts[0].ImportType = %q, want relative", facts[0].ImportType)
	}
}

// --- Wildcard import tests ---

func TestWildcardUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use crate::prelude::*;
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "crate::prelude" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "crate::prelude")
	}
	if facts[0].ImportType != "relative" {
		t.Errorf("importType = %q, want relative", facts[0].ImportType)
	}
	assertNames(t, facts[0].ImportedNames, []string{"*"})
}

func TestWildcardAbsolute(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use bevy::prelude::*;
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "bevy::prelude" {
		t.Errorf("specifier = %q, want %q", facts[0].Specifier, "bevy::prelude")
	}
	if facts[0].ImportType != "absolute" {
		t.Errorf("importType = %q, want absolute", facts[0].ImportType)
	}
}

// --- Self-in-group tests ---

func TestSelfInGroup(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use crate::models::{self, User};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	// self refers to the prefix module itself.
	if facts[0].Specifier != "crate::models" {
		t.Errorf("facts[0] = %q, want %q", facts[0].Specifier, "crate::models")
	}
	if facts[1].Specifier != "crate::models::User" {
		t.Errorf("facts[1] = %q, want %q", facts[1].Specifier, "crate::models::User")
	}
}

// --- As rename tests ---

func TestAsRename(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use crate::models::User as AppUser;
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("expected 1 fact, got %d", len(facts))
	}
	if facts[0].Specifier != "crate::models::User" {
		t.Errorf("specifier = %q", facts[0].Specifier)
	}
	assertNames(t, facts[0].ImportedNames, []string{"AppUser"})
}

// --- Top-level braces test ---

func TestTopLevelBraces(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`use {crate::models::User, std::collections::HashMap};
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d: %v", len(facts), factsSpecs(facts))
	}
	if facts[0].Specifier != "crate::models::User" {
		t.Errorf("facts[0] = %q", facts[0].Specifier)
	}
	if facts[0].ImportType != "relative" {
		t.Errorf("facts[0].ImportType = %q, want relative", facts[0].ImportType)
	}
	if facts[1].Specifier != "std::collections::HashMap" {
		t.Errorf("facts[1] = %q", facts[1].Specifier)
	}
	if facts[1].ImportType != "absolute" {
		t.Errorf("facts[1].ImportType = %q, want absolute", facts[1].ImportType)
	}
}

// --- Deeply nested multi-line (Bevy-style) ---

func TestBevyStyleMultiLineUse(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("system.rs", []byte(`use crate::{
    archetype::{Archetype, ArchetypeComponentId},
    component::{ComponentId, Tick},
    prelude::World,
    query::{
        Access,
        FilteredAccessSet,
    },
    system::{
        check_system_change_tick, ReadOnlySystemParam, System, SystemParam,
    },
    world::{unsafe_world_cell::UnsafeWorldCell, DeferredWorld},
};

pub struct FunctionSystem {}
`))
	if err != nil {
		t.Fatal(err)
	}
	// Count expected facts:
	// archetype::Archetype, archetype::ArchetypeComponentId = 2
	// component::ComponentId, component::Tick = 2
	// prelude::World = 1
	// query::Access, query::FilteredAccessSet = 2
	// system::check_system_change_tick, system::ReadOnlySystemParam, system::System, system::SystemParam = 4
	// world::unsafe_world_cell::UnsafeWorldCell, world::DeferredWorld = 2
	// Total = 13
	if len(facts) != 13 {
		t.Fatalf("expected 13 facts, got %d: %v", len(facts), factsSpecs(facts))
	}

	// Spot-check a few key specs.
	specs := make(map[string]bool)
	for _, f := range facts {
		specs[f.Specifier] = true
	}
	for _, want := range []string{
		"crate::archetype::Archetype",
		"crate::component::Tick",
		"crate::query::Access",
		"crate::system::SystemParam",
		"crate::world::unsafe_world_cell::UnsafeWorldCell",
		"crate::world::DeferredWorld",
	} {
		if !specs[want] {
			t.Errorf("missing expected spec %q", want)
		}
	}
}

// --- Line number tracking ---

func TestMultiLineUseLineNumber(t *testing.T) {
	ext := New()
	facts, err := ext.Extract("lib.rs", []byte(`fn before() {}

use crate::{
    models::User,
    config::Config,
};

fn after() {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(facts))
	}
	// Line number should be the start of the use statement (line 3).
	if facts[0].LineNumber != 3 {
		t.Errorf("facts[0].LineNumber = %d, want 3", facts[0].LineNumber)
	}
	if facts[1].LineNumber != 3 {
		t.Errorf("facts[1].LineNumber = %d, want 3", facts[1].LineNumber)
	}
}

// --- Helpers ---

func factsSpecs(facts []model.ImportFact) []string {
	var specs []string
	for _, f := range facts {
		specs = append(specs, f.Specifier)
	}
	return specs
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
