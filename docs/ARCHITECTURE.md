# Architecture

Contextception is a code context intelligence engine. It builds a dependency graph of your repository and uses it to answer: "What code must be understood before making a safe change?"

## High-Level Pipeline

```
Repository
    |
    v
+-------------------+     +-------------------+     +-------------------+
|    Scanner        | --> | Language Extractors| --> |    Resolvers      |
| (file discovery)  |     | (import parsing)  |     | (path resolution) |
+-------------------+     +-------------------+     +-------------------+
                                                            |
                                                            v
+-------------------+     +-------------------+     +-------------------+
|  Context Bundle   | <-- | Analysis Engine   | <-- | SQLite Index      |
| (JSON output)     |     | (scoring/ranking) |     | (graph + signals) |
+-------------------+     +-------------------+     +-------------------+
```

**Data flows left to right during indexing, right to left during analysis.**

## Components

### Indexer (`internal/indexer/`)

Orchestrates the scan-extract-resolve-store pipeline. Supports two modes:

- **Full index** — scans all files, extracts imports, resolves to file paths, stores edges, computes structural and git signals
- **Incremental index** — detects changed files via content hashes, re-processes only those files, then recomputes signals

Files are processed in parallel using a worker pool. Each worker clones the appropriate language extractor for thread safety.

```
Phase 1: Scan files, insert file records
Phase 2: Parallel extraction + resolution (worker pool)
Phase 3: Store edges, compute indegree/outdegree, git signals
```

### Language Extractors (`internal/extractor/`)

Each language has a pluggable extractor implementing the `Extractor` interface:

```go
type Extractor interface {
    Language() string
    Extensions() []string
    Extract(filePath string, content []byte) ([]ImportFact, error)
    Clone() Extractor
}
```

An `ImportFact` represents one import statement: the raw specifier, line number, and optionally which symbols were imported.

| Language | Approach | Notes |
|----------|----------|-------|
| Python | Regex | `import x`, `from x import y`, relative imports |
| TypeScript/JS | Tree-sitter (CGO) | AST-based, falls back to regex without CGO |
| Go | Regex | Standard library filtering via known packages |
| Java | Regex | Package imports, standard library filtering |
| Rust | Regex | `use`, `mod`, `extern crate`, multi-line imports |

**Adding a new language:**

1. Create `internal/extractor/<lang>/<lang>.go` implementing `Extractor`
2. Create `internal/resolver/<lang>/<lang>.go` implementing `Resolver`
3. Register in `internal/indexer/indexer.go` (extractor + resolver maps)
4. Add test fixtures in `testdata/`

### Resolver Layer (`internal/resolver/`)

Resolvers map import specifiers to repo-relative file paths:

```go
type Resolver interface {
    Resolve(srcFile string, fact extractor.ImportFact, repoRoot string) (string, error)
}
```

Each language has unique resolution rules:

- **Python** — package hierarchy, `__init__.py`, pyproject.toml
- **TypeScript/JS** — tsconfig paths + extends chains, workspace monorepos, barrel files, subpath resolution
- **Go** — go.mod + go.work, same-package sibling discovery
- **Java** — package-to-directory mapping, mirror-directory test discovery
- **Rust** — Cargo workspaces, mod.rs, crate/super/self paths

Unresolved imports (external packages, dynamic imports) are stored separately and contribute to the confidence score.

### Analysis Engine (`internal/analyzer/`)

Given a subject file, the analyzer:

1. **Collects candidates** — BFS traversal of the dependency graph (distance 1 and 2)
2. **Scores** — combines structural signals (indegree, distance, entrypoint status) with historical signals (co-change frequency, churn)
3. **Categorizes** — assigns each candidate to must_read, likely_modify, related, or tests based on score thresholds and distance
4. **Enriches** — topo-sorts must_read (foundational first), adds symbol tracking, role classification, code signatures, hotspot detection, circular dependency detection

```
Scoring Formula (per candidate):
  structural = indegree(normalized) * 4.0
             + distance_weight * 3.0
             + entrypoint_bonus * 1.0
             + api_surface_bonus * 1.5
             + proximity_bonus * 2.0

  historical = co_change * 2.0 + churn * 1.0
  historical = min(historical, structural * cap)

  score = structural + historical
```

The historical cap prevents co-change signals from overwhelming structural evidence. Distance-2 candidates get a higher cap (1.5x) so co-change can promote distant but frequently co-modified files.

### Change Analysis (`internal/change/`)

Analyzes the impact of a git diff (PR or branch):

1. Diffs `base..head` to find changed files
2. Analyzes each changed file independently
3. Detects coupling between changed files (structural edges)
4. Identifies test gaps (changed files with no test coverage)
5. Surfaces hidden coupling (co-change partners not in the diff)
6. Aggregates blast radius across all changed files

### Database Layer (`internal/db/`)

SQLite with WAL mode. Core tables:

| Table | Purpose |
|-------|---------|
| `files` | Indexed files with content hash, language, size |
| `edges` | Dependency edges with type, specifier, imported symbols |
| `signals` | Structural signals: indegree, outdegree, is_entrypoint |
| `git_signals` | Churn data per file (normalized 0-1) |
| `co_change_pairs` | Historical co-change frequency between file pairs |
| `unresolved` | External/failed import resolution |

Schema migrations are versioned (currently v4, 4 migrations). The indexer checks schema version and triggers full reindex when the schema changes.

### MCP Server (`internal/mcpserver/`)

Exposes contextception as an MCP server over stdio transport. Eight tools:

| Tool | Purpose |
|------|---------|
| `get_context` | Analyze file dependencies |
| `index` | Build/update index |
| `status` | Index diagnostics |
| `search` | Find files by path or symbol |
| `get_entrypoints` | Discover entry points and foundations |
| `get_structure` | Directory structure + language distribution |
| `get_archetypes` | Detect representative files per layer |
| `analyze_change` | PR/branch impact analysis |

The server lazy-initializes the index on first tool call and runs migrations if needed.

## Output Schema

Analysis produces a JSON context bundle (schema 3.2):

```
AnalysisOutput
  ├── confidence (0.0-1.0)
  ├── must_read[] — files to understand before changing (topo-sorted)
  ├── likely_modify{high,medium,low} — files that probably need changes
  ├── tests{direct,indirect} — test coverage
  ├── related{} — nearby context grouped by relationship
  ├── blast_radius{level,detail,fragility}
  ├── hotspots[] — high-churn + high-fan-in files
  └── circular_deps[] — import cycles
```

Formal JSON Schema definitions are in `protocol/analysis-schema.json` and `protocol/change-schema.json`.

## Key Design Decisions

1. **Deterministic scoring** — no ML, no LLM calls in the core pipeline. Every recommendation is explainable from graph structure and git history.
2. **Lazy indexing** — MCP tools auto-index on first call. Subsequent calls are incremental (content-hash based change detection).
3. **Parallel extraction** — worker pool processes files concurrently. Extractors implement `Clone()` for thread-safe copies.
4. **Language-agnostic core** — extractors emit `ImportFact`, resolvers emit file paths. The analysis engine never sees language-specific details.
5. **Capped historical signals** — co-change and churn can boost scores but never dominate structural evidence.
