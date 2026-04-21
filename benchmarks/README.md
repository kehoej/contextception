# How Contextception Compares

A context quality comparison between Contextception, [Aider's repo-map](https://aider.chat/docs/repomap.html), and [Repomix](https://github.com/yamadashy/repomix), tested across 7 repos, 68 files, and 5 languages.

## TL;DR

- On httpx (independent fixture ground truth): **97% recall vs. Aider@4K's 83%** (Aider@8K: 90%), at 5x fewer tokens
- Aider's recall drops from **97% → 0%** as repos grow from 60 → 7,978 files
- Contextception averages **1,174 tokens** per analysis; Aider@4K averages **3,600 tokens** with lower recall

## Limitations

Read these first — they're the reason you should (or shouldn't) trust these numbers.

1. **Aider's repo-map serves a different purpose.** It's designed as internal LLM context for Aider's own editing workflow, not as a standalone dependency analysis tool. We're evaluating it outside its intended use case.
2. **Independent ground truth exists only for httpx.** The httpx comparisons use 5 expert-verified [fixture files](data/fixtures/) with hand-curated `must_read` lists. For other repos, ground truth is Contextception's own output, validated at grade A (3.76–3.97) across 23 evaluation rounds.
3. **Aider gets 0–3% on Go, Java, Rust, and C#** because its tree-sitter parser doesn't resolve module imports for these languages. This is a real limitation of the tool, not a testing artifact — but it means the comparison is lopsided for 4 of 7 repos.
4. **Sample size: 68 files across 7 repos**, selected by archetype diversity (one file per structural role per repo). Not exhaustive.

## What We Measured

**The question:** Given a file you want to change, which tool surfaces the right dependencies?

**Tools tested:**

| Tool | Configuration |
|------|--------------|
| Contextception | `contextception analyze --json <file>` (default settings) |
| Aider repo-map | `RepoMap(map_tokens=N)` via Python API, budgets: 2K, 4K, 8K tokens |
| Repomix | `repomix --style xml .` (full repo dump) |

**Metrics:** Recall (% of true dependencies found), precision (% of output that's actually relevant), output tokens.

## Results

### Scale Degradation

The key finding: Aider's recall degrades as repository size increases, while Contextception's output stays targeted.

| Repo | Language | Files | Aider@4K Recall | Aider@8K Recall | CC Tokens (avg) |
|------|----------|------:|----------------:|----------------:|----------------:|
| httpx | Python | 60 | 84% | 97% | 748 |
| Excalidraw | TypeScript | 603 | 19% | 25% | 990 |
| Tokio | Rust | 763 | 0% | 1% | 812 |
| Terraform | Go | 1,885 | 3% | 8% | 941 |
| Zulip | Python/TS | 2,638 | 21% | 27% | 837 |
| EF Core | C# | 5,708 | 2% | 3% | 1,420 |
| Spring Boot | Java | 7,978 | 0% | 0% | 2,109 |

**Why this happens:**

- **Python (httpx → Zulip):** Aider uses PageRank on a global definition graph. In a small repo, globally important files overlap with local dependencies. In a large repo, globally popular files (models.py, utils.py) crowd out the specific imports that matter for a given file.
- **Go, Java, Rust, C#:** Aider's tree-sitter parser doesn't resolve module specifiers to file paths. `import "internal/tfdiags"` doesn't create an edge to any file — it just notes that symbols are referenced. Contextception resolves these via go.mod, Java package conventions, Cargo workspaces, and .csproj project detection.
- **TypeScript:** Aider doesn't resolve tsconfig paths, workspace packages, or barrel exports. `@excalidraw/utils` doesn't map to `packages/utils/src/index.ts`.

### httpx Deep Dive (Fixture Ground Truth)

httpx is the strongest comparison because ground truth is fully independent — 5 hand-verified fixture files, not Contextception's own output.

| File | CC Recall | CC Precision | Aider@4K Recall | Aider@4K Precision | Aider@8K Recall | Aider@8K Precision |
|------|----------:|-------------:|----------------:|-------------------:|----------------:|-------------------:|
| `_client.py` | **100%** | **100%** | 70% | 37% | 80% | 32% |
| `_models.py` | **90%** | **90%** | 80% | 42% | 90% | 36% |
| `_auth.py` | **100%** | **100%** | 100% | 32% | 100% | 24% |
| **Average** | **97%** | **97%** | 83% | 37% | 90% | 31% |

Contextception matches or exceeds Aider's best recall while maintaining near-perfect precision. Aider@8K reaches 90% recall but at 31% precision — meaning 69% of its suggested files are irrelevant.

### Token Efficiency

| Tool | httpx | Zulip | Excalidraw | Terraform | Tokio | EF Core | Spring Boot |
|------|------:|------:|-----------:|----------:|------:|--------:|------------:|
| **Contextception** | 748 | 837 | 990 | 941 | 812 | 1,420 | 2,109 |
| **Aider@4K** | ~3,300 | ~4,200 | ~3,300 | ~3,700 | ~3,200 | ~3,400 | ~4,200 |
| **Aider@8K** | ~6,700 | ~7,200 | ~6,900 | ~6,900 | ~7,000 | ~7,100 | ~8,400 |
| **Repomix** | 198K | 17.6M | 2.5M | 5.6M | 1.4M | 23.2M | 9.8M |

*Values are average tokens per file analysis.*

Contextception produces 3–5x less output than Aider@4K. Repomix outputs the entire repo (200x–20,000x more), without any relevance signal.

### Per-Repo Detail

<details>
<summary>Excalidraw (TypeScript, 603 files) — Aider avg recall: 25% @8K</summary>

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall |
|------|-----------|-------------:|----------------:|----------------:|
| `element/src/store.ts` | Service | 9 | 22% | 22% |
| `excalidraw/types.ts` | Model | 10 | 20% | 30% |
| `TTDDialog/.../useMermaidRenderer.ts` | Plugin | 10 | 30% | 30% |
| `common/src/index.ts` | Utility | 10 | 20% | 50% |
| `excalidraw-app/app_constants.ts` | Config | 10 | 0% | 10% |
| `element/src/index.ts` | Barrel | 10 | 10% | 10% |
| `with-script-in-browser/utils.ts` | Leaf | 1 | 0% | 0% |
| `tests/LanguageList.test.tsx` | Test | 4 | 50% | 50% |

Aider struggles with TypeScript monorepos. Its tree-sitter approach parses individual files but doesn't resolve workspace packages or tsconfig paths.

</details>

<details>
<summary>Zulip (Python/TypeScript, 2,638 files) — Aider avg recall: 27% @8K</summary>

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall |
|------|-----------|-------------:|----------------:|----------------:|
| `web/src/click_handlers.ts` | Controller | 10 | 10% | 10% |
| `zerver/models/__init__.py` | Model | 10 | 10% | 20% |
| `zerver/decorator.py` | Middleware | 10 | 40% | 50% |
| `webhooks/statuspage/view.py` | Endpoint | 7 | 43% | 57% |
| `zerver/views/auth.py` | Auth | 10 | 20% | 30% |
| `tools/setup/emoji/emoji_setup_utils.py` | Leaf | 1 | 0% | 0% |
| `web/src/settings_config.ts` | Config | 8 | 25% | 25% |

At 8K tokens, Aider outputs ~117 files per analysis but still finds only 27% of actual dependencies. The global PageRank ranking drowns out local import relationships.

</details>

<details>
<summary>Terraform (Go, 1,885 files) — Aider avg recall: 8% @8K</summary>

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall |
|------|-----------|-------------:|----------------:|----------------:|
| `builtin/.../data_source_state.go` | Service | 10 | 0% | 0% |
| `providers/addressed_types.go` | Model | 9 | 11% | 11% |
| `stackruntime/hooks/callbacks.go` | Plugin | 10 | 0% | 0% |
| `tfdiags/compare.go` | Utility | 10 | 20% | 50% |
| `rpcapi/dependencies.go` | Endpoint | 10 | 0% | 0% |
| `backend/remote-state/oci/auth.go` | Auth | 10 | 0% | 0% |
| `configs/util.go` | Leaf | 10 | 0% | 10% |
| `command/workdir/backend_config_state.go` | Config | 10 | 0% | 0% |
| `addrs/action_test.go` | Test | 1 | 0% | 0% |

Aider's tree-sitter doesn't resolve Go module imports. `import "internal/tfdiags"` doesn't create an edge to any file.

</details>

<details>
<summary>Tokio (Rust, 763 files) — Aider avg recall: 1% @8K</summary>

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall |
|------|-----------|-------------:|----------------:|----------------:|
| `sync/rwlock/write_guard.rs` | Plugin | 7 | 0% | 0% |
| `io/mod.rs` | Utility | 10 | 0% | 10% |
| `sync/cancellation_token.rs` | Auth | 9 | 0% | 0% |
| `cfg.rs` | Leaf | 7 | 0% | 0% |
| `fs/open_options/mock_open_options.rs` | Config | 3 | 0% | 0% |
| `loom/mod.rs` | Barrel | 2 | 0% | 0% |
| `tests-build/.../macros_core_no_default.rs` | Test | 1 | 0% | 0% |

Rust's module system (crate paths, `mod` declarations, `use` re-exports) is invisible to Aider's tree-sitter parser.

</details>

<details>
<summary>EF Core (C#, 5,708 files) — Aider avg recall: 3% @8K</summary>

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall |
|------|-----------|-------------:|----------------:|----------------:|
| `SqlServerServiceCollectionExtensions.cs` | Service | 10 | 0% | 0% |
| `CountryRegion.cs` | Model | 10 | 0% | 10% |
| `IMemberTranslatorPlugin.cs` | Plugin | 10 | 0% | 0% |
| `IJsonValueReaderWriterSource.cs` | Utility | 10 | 0% | 0% |
| `ViewColumnBuilder.cs` | Endpoint | 10 | 0% | 0% |
| `SessionTokenStorageFactory.cs` | Auth | 10 | 10% | 10% |
| `RelationalConverterMappingHints.cs` | Leaf | 0 | 0% | 0% |
| `ConfigurationSourceExtensions.cs` | Config | 10 | 0% | 0% |
| `DbSetOperationTests.cs` | Test | 1 | 0% | 0% |
| `MigrationsOperations.cs` | Migration | 10 | 10% | 10% |
| `AdHocMapper.cs` | Serialization | 10 | 10% | 10% |
| `DefaultValueBinding.cs` | Error | 10 | 10% | 10% |
| `CosmosClientWrapper.cs` | CLI | 10 | 0% | 0% |
| `CommandErrorEventData.cs` | Event | 10 | 0% | 0% |
| `CSharpDbContextGenerator.Interfaces.cs` | Interface | 10 | 0% | 0% |
| `ITableBasedExpression.cs` | Orchestrator | 10 | 0% | 0% |
| `ComplexTypesTrackingSqlServerTest.cs` | Hotspot | 8 | 0% | 0% |

C# uses namespace-level `using` directives (`using Microsoft.EntityFrameworkCore;`), which Aider's tree-sitter parser cannot resolve to specific files. At 5,708 files, Aider outputs 130–160 files per query but finds only 2–3% of actual dependencies. Contextception resolves namespaces via `.csproj` project detection, namespace-to-directory mapping, and same-namespace sibling discovery.

</details>

<details>
<summary>Spring Boot (Java, 7,978 files) — Aider avg recall: 0% @8K</summary>

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall |
|------|-----------|-------------:|----------------:|----------------:|
| `maven/CustomLayersProvider.java` | Service | 10 | 0% | 0% |
| `logback/ElasticCommon...Formatter.java` | Model | 10 | 0% | 0% |
| `maven/AbstractDependencyFilterMojo.java` | Plugin | 10 | 0% | 0% |
| `autoconfigure/AutoConfiguration.java` | Utility | 10 | 0% | 0% |
| `webmvc/.../WelcomePageNot...Mapping.java` | Endpoint | 10 | 0% | 0% |
| `actuator/endpoint/SecurityContext.java` | Auth | 10 | 0% | 0% |
| `gradle/util/package-info.java` | Leaf | 1 | 0% | 0% |
| `http/client/HttpClientSettings.java` | Config | 10 | 0% | 0% |
| `gradle/.../BootBuildImageIntTests.java` | Test | 10 | 0% | 0% |

Aider outputs only 10–19 files per analysis (vs. 60+ in other repos) because its tree-sitter parser produces very few edges for Java. Combined with the multi-module Gradle structure, zero dependencies are found.

</details>

## Ground Truth

### Tier 1: Independent Fixtures (httpx)

Five expert-verified JSON fixture files defining exact `must_read` and `must_read_forbidden` lists:

- [`client.json`](data/fixtures/client.json) — `httpx/_client.py` (orchestrator, 10 imports)
- [`models.json`](data/fixtures/models.json) — `httpx/_models.py` (central hub)
- [`auth.json`](data/fixtures/auth.json) — `httpx/_auth.py` (mid-level module)
- [`urls.json`](data/fixtures/urls.json) — `httpx/_urls.py`
- [`transports_default.json`](data/fixtures/transports_default.json) — `httpx/_transports/default.py`

These fixtures were written by inspecting httpx source code directly — not derived from any tool's output.

### Tier 2: Validated CC Output (Other Repos)

For the remaining 6 repos, ground truth is Contextception's own `must_read` output, independently validated at grade A across evaluation rounds. Validation grades:

| Repo | Grade | Evaluation Rounds |
|------|-------|-------------------|
| Excalidraw | A (3.97) | Rounds 1–9 |
| Zulip | A (3.96) | Rounds 1–9 |
| Terraform | A (3.78) | Rounds 13–18 |
| Tokio | A (3.79) | Rounds 11, 20–23 |
| Spring Boot | A (3.76) | Rounds 13, 19 |
| EF Core | A (3.85) | C# Rounds 5–6 |

This creates a circular validation concern: Contextception gets 100% recall against its own output by definition. The comparison is still meaningful because Aider's recall is measured against the same ground truth — but "CC recall = 100%" should be read as "CC's output was validated as grade A" rather than "CC found everything."

## Reproduce It Yourself

```bash
# Clone and build
git clone https://github.com/kehoej/contextception.git
cd contextception

# The comparison scripts
ls scripts/compare/
#   run_comparison.sh       — runs all tools against all repos
#   extract_aider_map.py    — extracts file lists from Aider output
#   results.json            — raw results (also at benchmarks/data/results.json)

# Requirements: contextception binary, aider (pip install aider-chat), repomix (npm install -g repomix)
# Target repos must be cloned locally with full git history

# Run individual analyses
contextception analyze httpx/_client.py --json   # ~50ms
```

Full reproduction takes ~10 minutes across all repos.

## Methodology

See [methodology.md](methodology.md) for:
- Scoring rubric (4 dimensions, weighted)
- Per-file tables with F1 scores for all 51 files
- Tool versions and configurations
- Fairness considerations (expanded)
- Why Aider gets 0% on Go/Java/Rust
- Token efficiency derived metrics

## Raw Data

- [`data/results.json`](data/results.json) — Complete results for all 7 repos, 68 files
- [`data/fixtures/`](data/fixtures/) — httpx fixture ground truth files
- [`scripts/compare/`](../scripts/compare/) — Comparison scripts
