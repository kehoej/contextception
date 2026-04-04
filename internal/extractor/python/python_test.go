package python

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kehoej/contextception/internal/model"
)

func TestExtract(t *testing.T) {
	ext := New()

	tests := []struct {
		name  string
		input string
		want  []model.ImportFact
	}{
		{
			name:  "absolute import",
			input: "import os",
			want: []model.ImportFact{
				{Specifier: "os", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:  "dotted absolute import",
			input: "import os.path",
			want: []model.ImportFact{
				{Specifier: "os.path", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"path"}},
			},
		},
		{
			name:  "multi import",
			input: "import os, sys, json",
			want: []model.ImportFact{
				{Specifier: "os", ImportType: "absolute", LineNumber: 1},
				{Specifier: "sys", ImportType: "absolute", LineNumber: 1},
				{Specifier: "json", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:  "from import absolute",
			input: "from foo.bar import baz, qux",
			want: []model.ImportFact{
				{Specifier: "foo.bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"baz"}},
				{Specifier: "foo.bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"qux"}},
			},
		},
		{
			name:  "from import relative single dot",
			input: "from .utils import helper",
			want: []model.ImportFact{
				{Specifier: ".utils", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"helper"}},
			},
		},
		{
			name:  "from import relative double dot",
			input: "from ..models import User",
			want: []model.ImportFact{
				{Specifier: "..models", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"User"}},
			},
		},
		{
			name:  "from import relative dot only",
			input: "from . import models",
			want: []model.ImportFact{
				{Specifier: ".", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"models"}},
			},
		},
		{
			name:  "star import",
			input: "from foo import *",
			want: []model.ImportFact{
				{Specifier: "foo", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"*"}},
			},
		},
		{
			name:  "import with alias",
			input: "import numpy as np",
			want: []model.ImportFact{
				{Specifier: "numpy", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:  "from import with alias",
			input: "from datetime import datetime as dt",
			want: []model.ImportFact{
				{Specifier: "datetime", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"datetime"}},
			},
		},
		{
			name: "multiline parenthesized import",
			input: `from foo.bar import (
    baz,
    qux,
    quux
)`,
			want: []model.ImportFact{
				{Specifier: "foo.bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"baz"}},
				{Specifier: "foo.bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"qux"}},
				{Specifier: "foo.bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"quux"}},
			},
		},
		{
			name:  "inline comment stripped",
			input: "import os  # operating system",
			want: []model.ImportFact{
				{Specifier: "os", ImportType: "absolute", LineNumber: 1},
			},
		},
		{
			name:  "comment line skipped",
			input: "# import os",
			want:  nil,
		},
		{
			name:  "empty file",
			input: "",
			want:  nil,
		},
		{
			name:  "no imports",
			input: "x = 1\nprint(x)\n",
			want:  nil,
		},
		{
			name: "conditional import extracted",
			input: `if TYPE_CHECKING:
    from myapp.models import User`,
			want: []model.ImportFact{
				{Specifier: "myapp.models", ImportType: "absolute", LineNumber: 2, ImportedNames: []string{"User"}},
			},
		},
		{
			name: "triple quoted string skipped",
			input: `"""
import os
from foo import bar
"""
import sys`,
			want: []model.ImportFact{
				{Specifier: "sys", ImportType: "absolute", LineNumber: 5},
			},
		},
		{
			name: "backslash continuation",
			input: `from foo.bar import \
    baz, qux`,
			want: []model.ImportFact{
				{Specifier: "foo.bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"baz"}},
				{Specifier: "foo.bar", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"qux"}},
			},
		},
		{
			name: "multiple imports different lines",
			input: `import os
from pathlib import Path
from .utils import helper
import json`,
			want: []model.ImportFact{
				{Specifier: "os", ImportType: "absolute", LineNumber: 1},
				{Specifier: "pathlib", ImportType: "absolute", LineNumber: 2, ImportedNames: []string{"Path"}},
				{Specifier: ".utils", ImportType: "relative", LineNumber: 3, ImportedNames: []string{"helper"}},
				{Specifier: "json", ImportType: "absolute", LineNumber: 4},
			},
		},
		{
			name: "realistic file",
			input: `#!/usr/bin/env python3
"""Module docstring."""

import os
import sys
from typing import Optional, List
from pathlib import Path

from .models import User, Group
from ..config import settings
from myapp.utils import (
    format_date,
    validate_email,
)

def main():
    pass
`,
			want: []model.ImportFact{
				{Specifier: "os", ImportType: "absolute", LineNumber: 4},
				{Specifier: "sys", ImportType: "absolute", LineNumber: 5},
				{Specifier: "typing", ImportType: "absolute", LineNumber: 6, ImportedNames: []string{"Optional"}},
				{Specifier: "typing", ImportType: "absolute", LineNumber: 6, ImportedNames: []string{"List"}},
				{Specifier: "pathlib", ImportType: "absolute", LineNumber: 7, ImportedNames: []string{"Path"}},
				{Specifier: ".models", ImportType: "relative", LineNumber: 9, ImportedNames: []string{"User"}},
				{Specifier: ".models", ImportType: "relative", LineNumber: 9, ImportedNames: []string{"Group"}},
				{Specifier: "..config", ImportType: "relative", LineNumber: 10, ImportedNames: []string{"settings"}},
				{Specifier: "myapp.utils", ImportType: "absolute", LineNumber: 11, ImportedNames: []string{"format_date"}},
				{Specifier: "myapp.utils", ImportType: "absolute", LineNumber: 11, ImportedNames: []string{"validate_email"}},
			},
		},
		{
			name:  "single line paren import",
			input: "from foo import (bar, baz)",
			want: []model.ImportFact{
				{Specifier: "foo", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"bar"}},
				{Specifier: "foo", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"baz"}},
			},
		},
		{
			name:  "from import relative multi-module",
			input: "from . import a, b, c",
			want: []model.ImportFact{
				{Specifier: ".", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"a"}},
				{Specifier: ".", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"b"}},
				{Specifier: ".", ImportType: "relative", LineNumber: 1, ImportedNames: []string{"c"}},
			},
		},
		{
			name:  "from import absolute multi-name",
			input: "from pkg import X, Y",
			want: []model.ImportFact{
				{Specifier: "pkg", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"X"}},
				{Specifier: "pkg", ImportType: "absolute", LineNumber: 1, ImportedNames: []string{"Y"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ext.Extract("test.py", []byte(tt.input))
			if err != nil {
				t.Fatalf("Extract returned error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d facts, want %d\ngot:  %+v\nwant: %+v", len(got), len(tt.want), got, tt.want)
			}

			for i := range got {
				if got[i].Specifier != tt.want[i].Specifier {
					t.Errorf("fact[%d].Specifier = %q, want %q", i, got[i].Specifier, tt.want[i].Specifier)
				}
				if got[i].ImportType != tt.want[i].ImportType {
					t.Errorf("fact[%d].ImportType = %q, want %q", i, got[i].ImportType, tt.want[i].ImportType)
				}
				if got[i].LineNumber != tt.want[i].LineNumber {
					t.Errorf("fact[%d].LineNumber = %d, want %d", i, got[i].LineNumber, tt.want[i].LineNumber)
				}
				if !strSliceEqual(got[i].ImportedNames, tt.want[i].ImportedNames) {
					t.Errorf("fact[%d].ImportedNames = %v, want %v", i, got[i].ImportedNames, tt.want[i].ImportedNames)
				}
			}
		})
	}
}

func TestLanguageAndExtensions(t *testing.T) {
	ext := New()
	if ext.Language() != "python" {
		t.Errorf("Language() = %q, want %q", ext.Language(), "python")
	}
	if exts := ext.Extensions(); len(exts) != 1 || exts[0] != ".py" {
		t.Errorf("Extensions() = %v, want [\".py\"]", exts)
	}
}

func TestIsStdlib(t *testing.T) {
	if !IsStdlib("os") {
		t.Error("IsStdlib(\"os\") = false, want true")
	}
	if !IsStdlib("json") {
		t.Error("IsStdlib(\"json\") = false, want true")
	}
	if IsStdlib("flask") {
		t.Error("IsStdlib(\"flask\") = true, want false")
	}
	if IsStdlib("myapp") {
		t.Error("IsStdlib(\"myapp\") = true, want false")
	}
}

func strSliceEqual(a, b []string) bool {
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

func TestExtractDefinitionsPython(t *testing.T) {
	ext := New()

	longVal := strings.Repeat("a", 130)
	longContent := fmt.Sprintf("LONG_VAR = \"%s\"", longVal)
	longExpected := longContent[:120] + "..."

	tests := []struct {
		name     string
		content  string
		symbols  []string
		expected []string
	}{
		{
			name:     "def function",
			content:  "def foo(x, y):",
			symbols:  []string{"foo"},
			expected: []string{"def foo(x, y):"},
		},
		{
			name:     "async def",
			content:  "async def bar(data):",
			symbols:  []string{"bar"},
			expected: []string{"async def bar(data):"},
		},
		{
			name:     "class",
			content:  "class MyClass(Base):",
			symbols:  []string{"MyClass"},
			expected: []string{"class MyClass(Base):"},
		},
		{
			name:     "constant assignment",
			content:  "MAX_RETRIES = 3",
			symbols:  []string{"MAX_RETRIES"},
			expected: []string{"MAX_RETRIES = 3"},
		},
		{
			name:     "typed assignment",
			content:  "x: int = 5",
			symbols:  []string{"x"},
			expected: []string{"x: int = 5"},
		},
		{
			name:     "multiple defs",
			content:  "def foo():\n    pass\ndef bar():\n    pass",
			symbols:  []string{"foo", "bar"},
			expected: []string{"def foo():", "def bar():"},
		},
		{
			name:     "no match",
			content:  "def foo():\n    pass",
			symbols:  []string{"nonexistent"},
			expected: nil,
		},
		{
			name:     "empty content",
			content:  "",
			symbols:  []string{"foo"},
			expected: nil,
		},
		{
			name:     "empty symbols",
			content:  "def foo():",
			symbols:  nil,
			expected: nil,
		},
		{
			name:     "long assignment truncated",
			content:  longContent,
			symbols:  []string{"LONG_VAR"},
			expected: []string{longExpected},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ext.ExtractDefinitions([]byte(tt.content), tt.symbols)
			if !strSliceEqual(got, tt.expected) {
				t.Errorf("ExtractDefinitions() = %v, want %v", got, tt.expected)
			}
		})
	}
}
