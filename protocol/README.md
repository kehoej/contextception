# Contextception Protocol Specification

This directory contains the formal protocol specification for Contextception's JSON output formats. All CLI and MCP server outputs conform to these schemas.

## Schemas

| Schema | Version | File | Description |
|--------|---------|------|-------------|
| AnalysisOutput | 3.2 | [analysis-schema.json](analysis-schema.json) | Context bundle for single or multi-file analysis |
| ChangeReport | 1.0 | [change-schema.json](change-schema.json) | Impact analysis for branch diffs |
| RateContext | 1.0 | (MCP input only) | LLM feedback on context quality |

Both schemas use [JSON Schema draft 2020-12](https://json-schema.org/draft/2020-12/schema).

## Generating Schemas

The JSON Schema files are generated from the Go struct definitions in `schema/`:

```bash
go run ./cmd/gen-schema
```

This reads the Go types via reflection and writes the schema files to this directory. Regenerate after any changes to the schema Go types.

---

## AnalysisOutput (v3.2)

Produced by `contextception analyze <file>` and the `get_context` MCP tool.

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Always `"3.2"` for this version |
| `subject` | string | Repo-relative path of the analyzed file |
| `confidence` | number | Analysis reliability score, range `[0.0, 1.0]` |
| `external` | string[] | External/stdlib imports (not resolved to repo files) |
| `must_read` | MustReadEntry[] | Files required to safely understand the subject |
| `likely_modify` | object | Files likely needing modification, keyed by confidence tier (`"high"`, `"medium"`, `"low"`); values are LikelyModifyEntry[] |
| `tests` | TestEntry[] | Test files covering the subject |
| `related` | object | Nearby context worth reviewing, keyed by relationship type; values are RelatedEntry[] |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `confidence_note` | string | Human-readable explanation of the confidence score |
| `must_read_note` | string | Note when must_read list was capped (overflow goes to related) |
| `likely_modify_note` | string | Note about likely_modify filtering |
| `tests_note` | string | Note about test coverage |
| `related_note` | string | Note about related entries |
| `blast_radius` | BlastRadius or null | Overall risk profile of a change to this file |
| `hotspots` | string[] | High-churn files that are also structural bottlenecks |
| `circular_deps` | string[][] | Import cycles involving the subject file; each inner array is a cycle |
| `stats` | IndexStats or null | Index health metadata at analysis time |

### MustReadEntry

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | yes | Repo-relative file path |
| `symbols` | string[] | no | Imported symbol names used by the subject |
| `definitions` | string[] | no | Code signatures (function/class definitions) relevant to the subject |
| `stable` | boolean | no | If true, this file rarely changes (low modification risk) |
| `direction` | string | no | Dependency direction: `"imports"` (subject imports this), `"imported_by"` (this imports subject), `"mutual"` (both directions), `"same_package"` (same-package sibling) |
| `role` | string | no | Structural role classification (e.g., `"config"`, `"model"`, `"util"`, `"test"`, `"entrypoint"`) |
| `circular` | boolean | no | If true, this file is part of a circular dependency with the subject |

### LikelyModifyEntry

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | yes | Repo-relative file path |
| `confidence` | string | yes | Confidence tier: `"high"`, `"medium"`, or `"low"` |
| `signals` | string[] | yes | Reasons this file may need modification |
| `symbols` | string[] | no | Specific symbols that connect this file to the subject |
| `role` | string | no | Structural role classification |

### TestEntry

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | yes | Repo-relative path to the test file |
| `direct` | boolean | yes | `true` if this test directly covers the subject; `false` for transitive/dependency coverage |

### BlastRadius

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `level` | string | yes | Risk level: `"high"`, `"medium"`, or `"low"` |
| `detail` | string | yes | Human-readable explanation of the risk assessment |
| `fragility` | number | no | Instability metric (ratio of efferent to total coupling); range `[0.0, 1.0]` |

### RelatedEntry

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | yes | Repo-relative file path |
| `signals` | string[] | yes | Relationship signals (e.g., `"same_directory"`, `"hidden_coupling"`, `"co_change"`) |

### IndexStats

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `total_files` | integer | yes | Number of indexed files in the repository |
| `total_edges` | integer | yes | Number of dependency edges in the graph |
| `unresolved_count` | integer | yes | Number of imports that could not be resolved to files |

---

## ChangeReport (v1.0)

Produced by `contextception analyze-change [base..head]` and the `analyze_change` MCP tool.

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Always `"1.0"` for this version |
| `ref_range` | string | Git ref range analyzed (e.g., `"main..HEAD"`) |
| `changed_files` | ChangedFile[] | Files changed in the diff with per-file analysis |
| `summary` | ChangeSummary | Aggregate statistics about the change |
| `blast_radius` | BlastRadius | Aggregated risk profile across all changed files |
| `must_read` | MustReadEntry[] | Combined must-read files for all changes |
| `likely_modify` | object | Combined likely-modify files, keyed by confidence tier |
| `tests` | TestEntry[] | Combined test coverage for all changed files |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `coupling` | CouplingPair[] | Pairs of changed files that are structurally connected |
| `hotspots` | string[] | High-churn structural bottlenecks among changed files |
| `circular_deps` | string[][] | Circular dependencies involving changed files |
| `test_gaps` | string[] | Changed files with no direct test coverage |
| `hidden_coupling` | HiddenCouplingEntry[] | Co-change partners not in the diff |
| `stats` | IndexStats or null | Index health metadata |

### ChangedFile

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | yes | Repo-relative file path |
| `status` | string | yes | Git diff status: `"added"`, `"modified"`, `"deleted"`, `"renamed"` |
| `blast_radius` | BlastRadius or null | no | Per-file risk (null for deleted/unindexed files) |
| `indexed` | boolean | yes | Whether this file exists in the index |

### ChangeSummary

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `total_files` | integer | yes | Total number of changed files |
| `added` | integer | yes | Number of added files |
| `modified` | integer | yes | Number of modified files |
| `deleted` | integer | yes | Number of deleted files |
| `renamed` | integer | yes | Number of renamed files |
| `indexed_files` | integer | yes | Number of changed files that are indexed |
| `test_files` | integer | yes | Number of changed files that are tests |
| `high_risk_files` | integer | yes | Number of changed files with high blast radius |

### CouplingPair

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file_a` | string | yes | First file in the pair |
| `file_b` | string | yes | Second file in the pair |
| `direction` | string | yes | Dependency direction: `"a_imports_b"`, `"b_imports_a"`, or `"bidirectional"` |

### HiddenCouplingEntry

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `changed_file` | string | yes | The file in the diff |
| `partner` | string | yes | The co-change partner not in the diff |
| `frequency` | integer | yes | Number of times these files changed together historically |

---

## RateContext (MCP-only)

The `rate_context` MCP tool accepts structured LLM feedback about a previous `get_context` result. It does not have a corresponding CLI output schema since it is an input-only tool.

### Input Parameters

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `file` | string | yes | Repo-relative path of the file that was analyzed |
| `usefulness` | integer | yes | Rating from 1 (not useful) to 5 (essential) |
| `useful_files` | string[] | no | Files from `must_read`/`related` that were actually useful |
| `unnecessary_files` | string[] | no | Files in `must_read` that were NOT needed |
| `missing_files` | string[] | no | Files you needed that were NOT suggested |
| `modified_files` | string[] | no | Files you actually modified during your work |
| `notes` | string | no | Brief explanation of the rating |

### Response

On success, returns a text content block: `"Feedback recorded. Thank you."`

On error (missing `file`, `usefulness` out of range), returns an error result with a descriptive message.

### Constraints

- `usefulness` must be between 1 and 5 (inclusive)
- `file` must be non-empty
- Feedback is linked to the most recent `get_context` or `analyze` usage for the given file (prefers context analyses over change analyses)

---

## Invariants

These properties hold across all schema versions:

1. **`must_read` is topologically sorted** -- foundational (upstream) files appear first.
2. **`likely_modify` keys are confidence tiers** -- always a subset of `{"high", "medium", "low"}`.
3. **`related` keys are relationship types** -- e.g., `"same_directory"`, `"hidden_coupling"`, `"overflow"`.
4. **`blast_radius.level`** is always one of `"high"`, `"medium"`, `"low"`.
5. **`confidence`** is always in the range `[0.0, 1.0]`.
6. **`blast_radius.fragility`** when present is always in the range `[0.0, 1.0]`.
7. **All file paths are repo-relative** -- no absolute paths, no leading `./`.
8. **`schema_version`** is always present as the first field in serialized output.

## Versioning

Contextception schema versions follow semantic versioning principles:

- **Major version bump** (e.g., 3.x to 4.0): Breaking changes -- fields removed, types changed, or semantic meaning altered. Consumers must update parsers.
- **Minor version bump** (e.g., 3.2 to 3.3): Additive changes only -- new optional fields. Existing consumers continue to work without modification.
- **No patch versions** -- the schema version tracks the output contract, not implementation fixes.

**Guarantees:**
- Within a major version, no fields will be removed or have their types changed.
- New required fields will only appear in major version bumps.
- Optional fields may be added in minor version bumps.
- Field ordering in JSON output is not guaranteed (consumers must not depend on key order).

## Example Usage

### CLI

```bash
# Single file analysis
contextception analyze src/server.ts

# Multi-file analysis
contextception analyze src/server.ts src/routes.ts

# Branch diff analysis
contextception analyze-change main..HEAD

# With AI workflow mode (adds mode-specific filtering)
contextception analyze --mode review src/server.ts

# With token budget awareness
contextception analyze --token-budget 8000 src/server.ts

# CI mode with risk exit codes
contextception analyze-change --ci --fail-on high
```

### MCP Server

The same schemas are returned by the MCP tools:
- `get_context` returns AnalysisOutput
- `analyze_change` returns ChangeReport
- `rate_context` accepts RateContext input, returns text confirmation

### Parsing Output

See [examples/parse_output.py](examples/parse_output.py) for Python validation against the JSON Schema, and [examples/parse_output.sh](examples/parse_output.sh) for shell-based field extraction with jq.

```python
import json, subprocess

result = subprocess.run(
    ["contextception", "analyze", "src/server.ts"],
    capture_output=True, text=True
)
bundle = json.loads(result.stdout)

# Access required fields
print(f"Subject: {bundle['subject']}")
print(f"Confidence: {bundle['confidence']}")
print(f"Must-read files: {len(bundle['must_read'])}")

# Check blast radius
if bundle.get("blast_radius"):
    print(f"Risk: {bundle['blast_radius']['level']}")
```
