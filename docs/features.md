# Contextception Feature Reference

Complete reference for Contextception's capabilities, output format, and configuration options.

**Schema version:** 3.2 | **Languages:** Python, TypeScript/JavaScript, Go, Java, Rust

---

## Table of Contents

- [Analysis Output](#analysis-output)
- [Confidence Scoring](#confidence-scoring)
- [Blast Radius](#blast-radius)
- [Hotspot Detection](#hotspot-detection)
- [Hidden Coupling Detection](#hidden-coupling-detection)
- [Circular Dependency Detection](#circular-dependency-detection)
- [Scoring and Ranking](#scoring-and-ranking)
- [Output Categories](#output-categories)
- [Language Support](#language-support)
- [CLI Reference](#cli-reference)
- [MCP Server](#mcp-server)
- [Configuration](#configuration)
- [Known Limitations](#known-limitations)

---

## Analysis Output

Every analysis produces a structured JSON context bundle (schema v3.2). See [protocol/](../protocol/) for the formal JSON Schema specification.

### Top-level fields

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Always `"3.2"` |
| `subject` | string | Repo-relative path of the analyzed file(s) |
| `confidence` | number | Analysis reliability score, 0.0--1.0 |
| `confidence_note` | string | Explanation when confidence < 1.0 |
| `external` | string[] | Unresolved import specifiers (stdlib, third-party) |
| `must_read` | MustReadEntry[] | Files required to safely understand the change |
| `likely_modify` | object | Files likely needing modification, grouped by confidence tier |
| `tests` | TestEntry[] | Test files covering the subject |
| `related` | object | Nearby context worth reviewing, grouped by relationship type |
| `blast_radius` | BlastRadius | Risk profile (level + detail + fragility) |
| `hotspots` | string[] | High-churn structural bottlenecks in the neighborhood |
| `circular_deps` | string[][] | Import cycles involving the subject file |
| `stats` | IndexStats | Index health (total_files, total_edges, unresolved_count) |

### Multi-file analysis

Pass multiple files to get a merged analysis:

```bash
contextception analyze src/auth/login.ts src/auth/session.ts
```

The merged result deduplicates `must_read`, takes the lowest confidence score (conservative), reports the highest blast radius, and combines test coverage.

---

## Confidence Scoring

Measures what fraction of the subject's internal imports were successfully resolved:

```
confidence = resolved_imports / (resolved_imports + unresolved_imports)
```

- Leaf files (no imports): confidence = 1.0
- When confidence < 1.0, a note explains what couldn't be resolved
- Rounded to 2 decimal places

Low confidence means the dependency graph is incomplete -- `must_read` may be missing files.

---

## Blast Radius

Classifies the risk of modifying the subject file.

| Level | Condition |
|-------|-----------|
| `low` | Few dependents, limited ripple effect |
| `medium` | Moderate number of dependents or reverse importers |
| `high` | Many likely modifications or 50+ reverse importers |

The `detail` string provides specifics (e.g., `"8 of 12 likely_modify, 3 of 5 related, 2 tests"`).

### Fragility metric

The `fragility` field measures instability using Robert C. Martin's metric:

```
fragility = Ce / (Ca + Ce)
```

Where Ce = files the subject imports (outdegree), Ca = files that import the subject (indegree). Range 0.0--1.0. Higher values mean the file depends on many things but few things depend on it -- making it vulnerable to upstream changes.

---

## Hotspot Detection

Identifies files that are both high-churn AND structural bottlenecks:

- **High churn:** Changes frequently in the 90-day git history window
- **High fan-in:** Many other files depend on it

These files are the most dangerous to change -- they change often and breaking them affects many consumers.

---

## Hidden Coupling Detection

Surfaces files that frequently co-change with the subject but have no import relationship. These represent behavioral coupling invisible to static analysis -- often caused by shared data formats, API contracts, or coordinated changes.

Hidden couplings appear in the `related` section with a `hidden_coupling:N` signal where N is the co-change frequency (minimum 3).

---

## Circular Dependency Detection

Detects import cycles the subject participates in using bounded depth-first search (max depth 10, max 5 cycles reported).

When cycles are detected:
- `circular_deps` lists each cycle as an array of file paths
- `must_read` entries in a cycle are flagged with `circular: true`
- `blast_radius` escalates from `low` to `medium`

---

## Scoring and Ranking

Every candidate file receives a deterministic score combining structural and historical signals.

### Structural signals

| Signal | Weight | Description |
|--------|--------|-------------|
| Indegree | 4.0 | Normalized by max indegree across the repo |
| Distance | 3.0 | 1.0 for direct imports, 0.0 for 2-hop |
| Proximity bonus | 2.0 | Fixed bonus for all direct dependencies |
| API surface | 1.5 | Bonus for files with 3+ dependents |
| Entrypoint | 1.0 | Bonus for configured entrypoints |
| Transitive caller | 1.0 | Bonus for barrel-piercing discovery |

### Historical signals

| Signal | Weight | Description |
|--------|--------|-------------|
| Co-change | 2.0 | How often files change together (90-day window) |
| Churn | 1.0 | How frequently the file changes |

Historical signals are capped relative to structural weight to prevent git noise from dominating rankings.

### Topological sorting

`must_read` entries are topologically sorted so foundational dependencies appear first. If you read files in the order returned, you'll build understanding from the bottom up.

### Barrel-piercing

When a direct importer is a barrel file (`index.ts`, `__init__.py`), the analysis looks through it to find transitive callers -- files that semantically depend on the subject through the barrel re-export.

---

## Output Categories

### must_read

Files required to safely understand the change. Default cap: 10.

Each entry includes:
- `file` -- repo-relative path
- `symbols` -- imported names used by the subject
- `definitions` -- code signatures (when `--signatures` is enabled)
- `direction` -- `"imports"`, `"imported_by"`, `"mutual"`, or `"same_package"`
- `role` -- structural classification: `test`, `barrel`, `foundation`, `orchestrator`, `entrypoint`, `utility`
- `stable` -- true if the file rarely changes (high indegree, low churn)
- `circular` -- true if part of an import cycle with the subject

Overflow entries move to `related`.

### likely_modify

Files likely needing modification when the subject changes. Default cap: 15. Grouped by confidence tier:

| Tier | Meaning |
|------|---------|
| **high** | Strong evidence: frequent co-changes, tight coupling, or both |
| **medium** | Moderate evidence: passes structural and historical gates |
| **low** | Partial evidence: near-threshold co-change or pure structural signal |

Each entry includes: `file`, `confidence`, `signals`, `symbols`, `role`.

### tests

Test files covering the subject. Default cap: 5. Two tiers:

- **Direct tests** -- tests that import the subject or match its filename stem
- **Dependency tests** (sub-cap: 2) -- tests covering `must_read` files, only shown when direct tests exist

### related

Nearby context worth reviewing. Default cap: 10. Grouped by relationship type. Sources include:
- Overflow from `must_read` cap
- 2-hop dependencies (prioritizing co-change partners)
- Hidden coupling signals

### external

Sorted list of import specifiers that couldn't be resolved to files in the repo (stdlib modules, third-party packages). Omitted when `--no-external` is set.

### Signal labels

Compact labels on `likely_modify` and `related` entries:

| Signal | Meaning |
|--------|---------|
| `imports` | Subject imports this file |
| `imported_by` | This file imports the subject |
| `transitive_caller` | Discovered via barrel-piercing |
| `two_hop` | 2-hop dependency, no direct import |
| `co_change:N` | Co-changed N times in the 90-day window |
| `high_churn` | High change frequency |
| `hotspot` | High churn + high fan-in |
| `hidden_coupling:N` | Co-changed N times but no import path |
| `circular` | Part of an import cycle with the subject |
| `same_package` | Same-package sibling (no explicit import) |
| `entrypoint` | Configured as an entrypoint |

---

## Language Support

### Python

**Extractor:** Regex | **Extensions:** `.py`

| Import pattern | Example |
|---------------|---------|
| Absolute | `import os`, `import os.path` |
| From-import | `from foo.bar import baz, qux` |
| Relative | `from .utils import helper`, `from ..models import User` |
| Star import | `from foo import *` |
| Multiline (parens) | `from foo import (\n  bar,\n  baz\n)` |
| Multiline (backslash) | `from foo import \\\n  bar, baz` |

**Resolution:** Package hierarchy, `pyproject.toml`, `__init__.py` detection. 200+ stdlib modules recognized.

**Definitions extracted:** Functions (`def`, `async def`), classes, module-level assignments (PascalCase/ALL_CAPS).

### TypeScript / JavaScript

**Extractor:** Tree-sitter (CGO) with regex fallback | **Extensions:** `.ts`, `.tsx`, `.js`, `.jsx`, `.mts`, `.cts`, `.mjs`, `.cjs`

| Import pattern | Example |
|---------------|---------|
| Default | `import foo from './foo'` |
| Named | `import { bar, baz } from './bar'` |
| Namespace | `import * as utils from '../utils'` |
| Type-only | `import type { Config } from './config'` |
| Re-export | `export { handler } from './handler'` |
| Star re-export | `export * from './types'` |
| CommonJS | `const x = require('./legacy')` |
| Dynamic | `await import('./dynamic')` |

**Resolution:** tsconfig paths with `extends` chains, workspace monorepos (npm/yarn/pnpm), barrel file detection, subpath file fallback.

**Definitions extracted:** Functions, classes, interfaces, type aliases, variables/constants, enums.

### Go

**Extractor:** Regex | **Extensions:** `.go`

| Import pattern | Example |
|---------------|---------|
| Single | `import "fmt"` |
| Grouped | `import (\n  "fmt"\n  "os"\n)` |
| Named/aliased | `import myalias "github.com/foo/bar"` |
| Blank | `import _ "net/http/pprof"` |

**Resolution:** `go.mod` module paths, `go.work` multi-module workspaces, same-package file resolution.

**Definitions extracted:** Functions (including methods), types (struct, interface), variables, constants.

### Java

**Extractor:** Regex | **Extensions:** `.java`

| Import pattern | Example |
|---------------|---------|
| Single | `import java.util.List;` |
| Wildcard | `import java.util.*;` |
| Static | `import static java.lang.Math.abs;` |
| Static wildcard | `import static java.lang.Math.*;` |

**Resolution:** Package-to-directory mapping, auto-detected source roots (`src/main/java/`, `src/test/java/`, `src/`), Maven and Gradle project detection, mirror-directory test discovery.

**Definitions extracted:** Classes, interfaces, enums, records, methods, constants.

### Rust

**Extractor:** Regex | **Extensions:** `.rs`

| Import pattern | Example |
|---------------|---------|
| Simple use | `use std::collections::HashMap;` |
| Grouped use | `use std::io::{Read, Write};` |
| Crate-relative | `use crate::models::User;` |
| Parent module | `use super::utils;` |
| Module declaration | `mod config;` / `pub mod routes;` |
| Pub re-export | `pub use crate::config::Config;` |
| Extern crate | `extern crate serde;` |

**Resolution:** `Cargo.toml` package and workspace detection, `crate::`/`super::`/`self::` path resolution, `mod.rs` conventions, inline `#[cfg(test)]` module detection.

**Definitions extracted:** Functions, structs, enums, traits, type aliases, constants/statics.

---

## CLI Reference

### Commands

```
contextception index                    Build or update the index
contextception analyze <file> [files]   Analyze context dependencies (single or multi-file)
contextception analyze-change [base]    Analyze the impact of a PR or commit range
contextception search <query>           Search the index by path or symbol
contextception archetypes               Detect archetype files from the index
contextception history <subcommand>     Historical analysis (trend, hotspots, distribution, file)
contextception reindex                  Full rebuild of the index
contextception extensions                List supported file extensions
contextception status                   Index status and diagnostics
contextception mcp                      Start MCP server (stdio transport)
```

### Global flags

| Flag | Description |
|------|-------------|
| `--repo <path>` | Repository root (default: auto-detected via git) |
| `-v, --verbose` | Verbose output |
| `--version` | Display version |

### Analysis flags (analyze, analyze-change)

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | Compact JSON output |
| `--no-external` | false | Omit external dependencies |
| `--max-must-read` | 10 | Cap on must_read entries |
| `--max-related` | 10 | Cap on related entries |
| `--max-likely-modify` | 15 | Cap on likely_modify entries |
| `--max-tests` | 5 | Cap on test entries |
| `--signatures` | false | Include code signatures for must_read symbols |
| `--stable-threshold` | adaptive | Indegree threshold for the stable flag |
| `--ci` | false | CI mode: suppress output, exit code reflects blast radius |
| `--fail-on` | high | Blast radius level that triggers non-zero exit (`high` or `medium`) |
| `--mode` | (none) | Workflow mode: `plan`, `implement`, or `review` |
| `--token-budget` | 0 | Target token budget (auto-adjusts caps) |

### CI mode

When `--ci` is set, output is suppressed and the exit code reflects blast radius:

| Exit code | Meaning |
|-----------|---------|
| 0 | Blast radius below threshold |
| 1 | Medium blast radius (with `--fail-on medium`) |
| 2 | High blast radius |

```bash
# Fail PR if blast radius is high
contextception analyze-change --ci --fail-on high

# Fail on medium or high
contextception analyze-change --ci --fail-on medium
```

### Workflow modes

The `--mode` flag sets output caps optimized for different stages:

| Mode | must_read | likely_modify | tests | related | Use case |
|------|-----------|---------------|-------|---------|----------|
| `plan` | 15 | 5 | 3 | 15 | Understanding scope |
| `implement` | 10 | 20 | 5 | 5 | Writing code |
| `review` | 5 | 10 | 10 | 5 | Verifying changes |

Individual `--max-*` flags override mode defaults. `--token-budget` further constrains.

### Token budget

`--token-budget N` automatically scales all caps to fit output within approximately N tokens (estimated at ~4 chars/token):

- must_read: 40% of budget
- likely_modify: 30%
- related: 20%
- tests: 10%

Each cap is bounded between 3 and 3x the default.

### Search flags

| Flag | Default | Description |
|------|---------|-------------|
| `--type` | path | Search type: `path` or `symbol` |
| `--limit` | 50 | Max results (max 100) |
| `--json` | false | JSON output |

### Archetypes flags

| Flag | Description |
|------|-------------|
| `--categories` | Filter to specific archetype categories |
| `--list` | List available archetype categories |

### History subcommands

| Subcommand | Description |
|------------|-------------|
| `history trend` | Blast radius trend over recent analyses |
| `history hotspots` | Files that frequently appear as hotspots |
| `history distribution` | Blast radius distribution over time |
| `history file <path>` | Risk history for a specific file |

---

## MCP Server

Start with `contextception mcp`. Communicates via stdio using the Model Context Protocol.

### Tools

| Tool | Description |
|------|-------------|
| `get_context` | Analyze a file's dependency context. Auto-indexes on first call. Accepts a single path or array for multi-file analysis. |
| `index` | Build or update the repository index. |
| `status` | Return index diagnostics (file count, edge count, staleness). |
| `search` | Search the index by path pattern or symbol name. |
| `get_entrypoints` | Return entrypoint and foundation files for project orientation. |
| `get_structure` | Return directory structure with file counts and language distribution. |
| `get_archetypes` | Detect representative files across architectural layers. |
| `analyze_change` | Analyze the impact of a git diff / PR. Returns blast radius, test gaps, coupling signals. |

### get_context parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `file` | string or string[] | yes | -- | Repo-relative or absolute path(s) |
| `mode` | string | no | (none) | Workflow mode: `plan`, `implement`, `review` |
| `token_budget` | integer | no | 0 | Target token budget |
| `omit_external` | boolean | no | false | Omit external dependencies |
| `include_signatures` | boolean | no | false | Include code signatures |
| `max_must_read` | integer | no | 10 | Cap on must_read entries |
| `max_related` | integer | no | 10 | Cap on related entries |
| `max_likely_modify` | integer | no | 15 | Cap on likely_modify entries |
| `max_tests` | integer | no | 5 | Cap on test entries |

### search parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | -- | Search string |
| `type` | string | no | `path` | `path` or `symbol` |
| `limit` | integer | no | 50 | Max results (max 100) |

### get_entrypoints parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `limit` | integer | no | 10 | Max foundation files |

### get_archetypes parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `categories` | string[] | no | (all) | Filter to specific categories |

### analyze_change parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `base` | string | no | auto-detect | Base git ref |
| `head` | string | no | HEAD | Head git ref |

---

## Configuration

Optional. Create `.contextception/config.yaml` in your repo root:

```yaml
version: 1
entrypoints:
  - cmd/server/main.go
  - src/index.ts
ignore:
  - vendor
  - node_modules
  - third_party
generated:
  - proto/generated
  - src/__generated__
```

| Field | Type | Description |
|-------|------|-------------|
| `version` | integer | Must be `1` (required) |
| `entrypoints` | string[] | Files treated as architectural entry points (boosted in scoring) |
| `ignore` | string[] | Directory prefixes excluded from analysis |
| `generated` | string[] | Directory prefixes for generated code (excluded from analysis) |

All paths must be relative. Unrecognized YAML keys are rejected.

---

## Known Limitations

| Limitation | Scope | Notes |
|------------|-------|-------|
| Dynamic imports not tracked | Python, TypeScript | `importlib.import_module()`, `import(varName)` |
| Conditional imports extracted normally | Python | `if TYPE_CHECKING:` blocks treated as regular imports |
| Build tags not considered | Go | `//go:build` conditional compilation not handled |
| File-level granularity only | All | No function/class-level analysis (symbol names tracked, not graphs) |
| No cross-repository context | All | Analysis scoped to a single repository |
| CommonJS exports not tracked | TypeScript/JS | `module.exports` assignments not extracted |
| Plugin systems not detected | All | Runtime-loaded modules invisible to static analysis |
| String-computed imports | All | Import paths constructed at runtime cannot be resolved |
| No macro expansion | Rust | `macro_rules!` and procedural macros not analyzed |
| No annotation processing | Java | Annotation-based dependencies not detected |
