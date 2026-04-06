package typescript

import (
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestExtract(t *testing.T) {
	ext := New()

	tests := []struct {
		name    string
		input   string
		want    []model.ImportFact
		wantLen int // if set, only check length (for complex cases)
	}{
		// === Basic imports ===
		{
			name:  "default import",
			input: `import foo from 'foo'`,
			want: []model.ImportFact{
				{Specifier: "foo", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"foo"}},
			},
		},
		{
			name:  "named imports",
			input: `import { bar, baz } from './bar'`,
			want: []model.ImportFact{
				{Specifier: "./bar", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"bar", "baz"}},
			},
		},
		{
			name:  "namespace import",
			input: `import * as utils from '../utils'`,
			want: []model.ImportFact{
				{Specifier: "../utils", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"*"}},
			},
		},
		{
			name:  "side-effect import",
			input: `import './side-effects'`,
			want: []model.ImportFact{
				{Specifier: "./side-effects", ImportType: "relative", LineNumber: 1},
			},
		},
		{
			name:  "default and named",
			input: `import React, { useState, useEffect } from 'react'`,
			want: []model.ImportFact{
				{Specifier: "react", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"React", "useState", "useEffect"}},
			},
		},
		{
			name:  "default and namespace",
			input: `import foo, * as bar from './mod'`,
			want: []model.ImportFact{
				{Specifier: "./mod", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"foo", "*"}},
			},
		},
		// === Type imports ===
		{
			name:  "type-only import",
			input: `import type { Config } from './config'`,
			want: []model.ImportFact{
				{Specifier: "./config", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"Config"}},
			},
		},
		{
			name:  "inline type import",
			input: `import { type Foo, Bar } from './mod'`,
			want: []model.ImportFact{
				{Specifier: "./mod", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"Foo", "Bar"}},
			},
		},
		{
			name:  "type-only default import",
			input: `import type Config from './config'`,
			want: []model.ImportFact{
				{Specifier: "./config", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"Config"}},
			},
		},
		// === Re-exports ===
		{
			name:  "named re-export",
			input: `export { handler } from './handler'`,
			want: []model.ImportFact{
				{Specifier: "./handler", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"handler"}},
			},
		},
		{
			name:  "star re-export",
			input: `export * from './types'`,
			want: []model.ImportFact{
				{Specifier: "./types", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"*"}},
			},
		},
		{
			name:  "namespace re-export",
			input: `export * as ns from './types'`,
			want: []model.ImportFact{
				{Specifier: "./types", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"*"}},
			},
		},
		{
			name:  "type re-export",
			input: `export type { Config } from './config'`,
			want: []model.ImportFact{
				{Specifier: "./config", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"Config"}},
			},
		},
		{
			name:  "renamed re-export",
			input: `export { foo as bar } from './foo'`,
			want: []model.ImportFact{
				{Specifier: "./foo", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"foo"}},
			},
		},
		// === CommonJS ===
		{
			name:  "basic require",
			input: `const x = require('./legacy')`,
			want: []model.ImportFact{
				{Specifier: "./legacy", ImportType: "relative", LineNumber: 1},
			},
		},
		{
			name:  "require bare specifier",
			input: `const express = require('express')`,
			want: []model.ImportFact{
				{Specifier: "express", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:  "destructured require",
			input: `const { readFile } = require('fs')`,
			want: []model.ImportFact{
				{Specifier: "fs", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:  "nested require",
			input: "function load() {\n  const m = require('./deep')\n  return m\n}",
			want: []model.ImportFact{
				{Specifier: "./deep", ImportType: "relative", LineNumber: 2},
			},
		},
		// === Dynamic import ===
		{
			name:  "dynamic import relative",
			input: `const mod = await import('./dynamic')`,
			want: []model.ImportFact{
				{Specifier: "./dynamic", ImportType: "relative", LineNumber: 1},
			},
		},
		{
			name:  "dynamic import absolute",
			input: `const mod = await import('lodash')`,
			want: []model.ImportFact{
				{Specifier: "lodash", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:    "dynamic import non-string arg",
			input:   `const mod = await import(varName)`,
			wantLen: 0,
		},
		{
			name:  "dynamic import inside function",
			input: "async function load() {\n  const m = await import('./lazy')\n  return m\n}",
			want: []model.ImportFact{
				{Specifier: "./lazy", ImportType: "relative", LineNumber: 2},
			},
		},
		// === Specifier classification ===
		{
			name:  "relative dot slash",
			input: `import './a'`,
			want: []model.ImportFact{
				{Specifier: "./a", ImportType: "relative", LineNumber: 1},
			},
		},
		{
			name:  "relative dot dot slash",
			input: `import '../b'`,
			want: []model.ImportFact{
				{Specifier: "../b", ImportType: "relative", LineNumber: 1},
			},
		},
		{
			name:  "bare specifier",
			input: `import 'lodash'`,
			want: []model.ImportFact{
				{Specifier: "lodash", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:  "scoped package",
			input: `import x from '@foo/bar'`,
			want: []model.ImportFact{
				{Specifier: "@foo/bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"x"}},
			},
		},
		{
			name:  "scoped package subpath",
			input: `import x from '@foo/bar/baz'`,
			want: []model.ImportFact{
				{Specifier: "@foo/bar/baz", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"x"}},
			},
		},
		// === Edge cases ===
		{
			name:    "empty file",
			input:   "",
			wantLen: 0,
		},
		{
			name:    "no imports",
			input:   "const x = 1;\nfunction foo() { return x; }",
			wantLen: 0,
		},
		{
			name: "multiline named imports",
			input: `import {
  alpha,
  beta,
  gamma
} from './greek'`,
			want: []model.ImportFact{
				{Specifier: "./greek", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"alpha", "beta", "gamma"}},
			},
		},
		{
			name:  "import with alias",
			input: `import { foo as bar } from './mod'`,
			want: []model.ImportFact{
				{Specifier: "./mod", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"foo"}},
			},
		},
		{
			name:    "commented import not extracted",
			input:   `// import foo from 'foo'`,
			wantLen: 0,
		},
		{
			name:    "export without source is not a re-export",
			input:   `export const x = 1`,
			wantLen: 0,
		},
		{
			name:    "export default is not a re-export",
			input:   `export default function() {}`,
			wantLen: 0,
		},
		// === Realistic mixed file ===
		{
			name: "mixed TS file",
			input: `import React from 'react'
import { useState } from 'react'
import type { FC } from 'react'
import * as utils from './utils'
import './styles.css'
export { handler } from './handler'
export * from './types'
const legacy = require('lodash')

async function loadModule() {
  const mod = await import('./lazy')
  return mod
}
`,
			want: []model.ImportFact{
				{Specifier: "react", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"React"}},
				{Specifier: "react", ImportType: "absolute", LineNumber: 2, ImportedNames: []string{"useState"}},
				{Specifier: "react", ImportType: "absolute", LineNumber: 3, ImportedNames: []string{"FC"}},
				{Specifier: "./utils", ImportType: "relative", LineNumber: 4, ImportedNames: []string{"*"}},
				{Specifier: "./styles.css", ImportType: "relative", LineNumber: 5},
				{Specifier: "./handler", ImportType: "relative", LineNumber: 6, ImportedNames: []string{"handler"}},
				{Specifier: "./types", ImportType: "relative", LineNumber: 7, ImportedNames: []string{"*"}},
				{Specifier: "lodash", ImportType: "absolute", LineNumber: 8},
				{Specifier: "./lazy", ImportType: "relative", LineNumber: 11},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, err := ext.Extract("test.ts", []byte(tt.input))
			if err != nil {
				t.Fatalf("Extract() error: %v", err)
			}

			if tt.wantLen >= 0 && tt.want == nil {
				if len(facts) != tt.wantLen {
					t.Fatalf("got %d facts, want %d; facts: %+v", len(facts), tt.wantLen, facts)
				}
				return
			}

			if len(facts) != len(tt.want) {
				t.Fatalf("got %d facts, want %d\ngot:  %+v\nwant: %+v", len(facts), len(tt.want), facts, tt.want)
			}

			for i, got := range facts {
				want := tt.want[i]
				if got.Specifier != want.Specifier {
					t.Errorf("fact[%d].Specifier = %q, want %q", i, got.Specifier, want.Specifier)
				}
				if got.ImportType != want.ImportType {
					t.Errorf("fact[%d].ImportType = %q, want %q", i, got.ImportType, want.ImportType)
				}
				if got.LineNumber != want.LineNumber {
					t.Errorf("fact[%d].LineNumber = %d, want %d", i, got.LineNumber, want.LineNumber)
				}
				if !stringSliceEqual(got.ImportedNames, want.ImportedNames) {
					t.Errorf("fact[%d].ImportedNames = %v, want %v", i, got.ImportedNames, want.ImportedNames)
				}
			}
		})
	}
}

func TestLanguage(t *testing.T) {
	ext := New()
	if got := ext.Language(); got != "typescript" {
		t.Errorf("Language() = %q, want %q", got, "typescript")
	}
}

func TestExtensions(t *testing.T) {
	ext := New()
	exts := ext.Extensions()
	want := map[string]bool{
		".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".mts": true, ".cts": true, ".mjs": true, ".cjs": true,
	}
	if len(exts) != len(want) {
		t.Fatalf("Extensions() returned %d extensions, want %d", len(exts), len(want))
	}
	for _, e := range exts {
		if !want[e] {
			t.Errorf("unexpected extension %q", e)
		}
	}
}

func TestClassifySpecifier(t *testing.T) {
	tests := []struct {
		spec string
		want string
	}{
		{"./foo", "relative"},
		{"../bar", "relative"},
		{"lodash", "absolute"},
		{"@foo/bar", "absolute"},
		{"@scope/pkg/sub", "absolute"},
		{"fs", "absolute"},
	}
	for _, tt := range tests {
		if got := classifySpecifier(tt.spec); got != tt.want {
			t.Errorf("classifySpecifier(%q) = %q, want %q", tt.spec, got, tt.want)
		}
	}
}

func TestStripQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`'foo'`, "foo"},
		{`"bar"`, "bar"},
		{"`baz`", "baz"},
		{"unquoted", "unquoted"},
		{`""`, ""},
		{`'a"`, `'a"`}, // mismatched
	}
	for _, tt := range tests {
		if got := stripQuotes(tt.input); got != tt.want {
			t.Errorf("stripQuotes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestExtractDefinitionsTypeScript(t *testing.T) {
	ext := New()

	tests := []struct {
		name       string
		content    string
		symbols    []string
		wantExact  []string // nil means check wantPrefix instead
		wantPrefix string   // non-empty means check first element HasPrefix
		wantNil    bool     // true when result must be nil
	}{
		{
			name:      "function",
			content:   "function foo(x: number): void {\n  return;\n}",
			symbols:   []string{"foo"},
			wantExact: []string{"function foo(x: number): void"},
		},
		{
			name:      "export function",
			content:   "export function bar() {\n  return;\n}",
			symbols:   []string{"bar"},
			wantExact: []string{"function bar()"},
		},
		{
			name:      "class",
			content:   "class Client {\n  constructor() {}\n}",
			symbols:   []string{"Client"},
			wantExact: []string{"class Client"},
		},
		{
			name:      "class extends implements",
			content:   "class Foo extends Bar implements Baz {\n  x = 1;\n}",
			symbols:   []string{"Foo"},
			wantExact: []string{"class Foo extends Bar implements Baz"},
		},
		{
			name:       "interface",
			content:    "interface Config {\n  key: string;\n}",
			symbols:    []string{"Config"},
			wantPrefix: "interface Config",
		},
		{
			name:       "type alias",
			content:    "type Options = {\n  key: string;\n}",
			symbols:    []string{"Options"},
			wantPrefix: "type Options = {",
		},
		{
			name:      "const",
			content:   "const MAX = 5;",
			symbols:   []string{"MAX"},
			wantExact: []string{"const MAX = 5;"},
		},
		{
			name:       "enum",
			content:    "enum Status {\n  Active,\n  Inactive\n}",
			symbols:    []string{"Status"},
			wantPrefix: "enum Status",
		},
		{
			name:      "export class",
			content:   "export class Exported {\n  x = 1;\n}",
			symbols:   []string{"Exported"},
			wantExact: []string{"class Exported"},
		},
		{
			name:    "no match",
			content: "function foo() {}",
			symbols: []string{"nope"},
			wantNil: true,
		},
		{
			name:    "empty symbols",
			content: "function foo() {}",
			symbols: nil,
			wantNil: true,
		},
		{
			name:    "empty content",
			content: "",
			symbols: []string{"Foo"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ext.ExtractDefinitions([]byte(tt.content), tt.symbols)

			if tt.wantNil {
				if got != nil {
					t.Errorf("ExtractDefinitions() = %v, want nil", got)
				}
				return
			}

			if tt.wantPrefix != "" {
				if len(got) == 0 {
					t.Fatalf("ExtractDefinitions() returned empty slice, want first element with prefix %q", tt.wantPrefix)
				}
				if !strings.HasPrefix(got[0], tt.wantPrefix) {
					t.Errorf("ExtractDefinitions()[0] = %q, want prefix %q", got[0], tt.wantPrefix)
				}
				return
			}

			// Exact match.
			if len(got) != len(tt.wantExact) {
				t.Fatalf("ExtractDefinitions() returned %d elements, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.wantExact), got, tt.wantExact)
			}
			for i, w := range tt.wantExact {
				if got[i] != w {
					t.Errorf("ExtractDefinitions()[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}
