# Methodology

Detailed methodology for the [context quality comparison](README.md).

*Date: February 23, 2026*

## Tool Versions

| Tool | Version | Notes |
|------|---------|-------|
| **Contextception** | Schema 3.2, Go binary (built from source) | Default settings, no flags |
| **Aider** | v0.86.2 (`pip install aider-chat`) | `RepoMap(map_tokens=N)` via Python API |
| **Repomix** | latest (`npm install -g repomix`) | XML output mode, full repo |
| **Tokenizer** | gpt-4o (Aider default) | Used for Aider token budget |

Repos were cloned with full git history on February 23, 2026.

## Repository Details

| Repo | Language | Files Indexed | Edges | Grade | Ground Truth |
|------|----------|:------------:|:-----:|:-----:|:-------------|
| [httpx](https://github.com/encode/httpx) | Python | 60 | 122 | A (3.96+) | Tier 1: fixture files |
| [Excalidraw](https://github.com/excalidraw/excalidraw) | TypeScript | 603 | 2,962 | A (3.97) | Tier 2: validated CC output |
| [Tokio](https://github.com/tokio-rs/tokio) | Rust | 763 | 1,454 | A (3.79) | Tier 2: validated CC output |
| [Terraform](https://github.com/hashicorp/terraform) | Go | 1,885 | 94,417 | A (3.78) | Tier 2: validated CC output |
| [Zulip](https://github.com/zulip/zulip) | Python/TS | 2,638 | 10,087 | A (3.96) | Tier 2: validated CC output |
| [EF Core](https://github.com/dotnet/efcore) | C# | 5,708 | 47,206 | A (3.85) | Tier 2: validated CC output |
| [Spring Boot](https://github.com/spring-projects/spring-boot) | Java | 7,978 | 15,658 | A (3.76) | Tier 2: validated CC output |

## File Selection

Files were selected by **archetype diversity**: one file per structural role per repo, chosen to cover the full range of dependency patterns. Archetypes:

| Archetype | Description | Example |
|-----------|-------------|---------|
| Service/Controller | High outdegree orchestrator | `httpx/_client.py` |
| Model/Schema | Central data type | `zerver/models/__init__.py` |
| Middleware/Plugin | Intercept/transform layer | `zerver/decorator.py` |
| High Fan-in Utility | Many dependents | `tfdiags/compare.go` (434 indegree) |
| Page/Route/Endpoint | Entry point | `httpx/_api.py` |
| Auth/Security | Authentication module | `zerver/views/auth.py` |
| Leaf Component | Few or no dependents | `emoji_setup_utils.py` |
| Config/Constants | Configuration | `HttpClientSettings.java` |
| Barrel/Index | Re-export aggregator | `httpx/__init__.py` |
| Test File | Test suite | `addrs/action_test.go` |

Files with zero `must_read` entries were excluded from recall/precision calculations (they have null ground truth).

## Scoring Rubric

### 4-Dimension Weighted Score

| Dimension | Weight | A (4) | B (3) | C (2) | D (1) |
|-----------|:------:|-------|-------|-------|-------|
| **Recall** | 35% | ≥90% | 60–89% | 30–59% | <30% |
| **Precision** | 30% | ≥50% | 25–49% | 10–24% | <10% |
| **Explainability** | 20% | Per-file rationale, direction, symbols | Some explanation | File names only | No context |
| **Actionability** | 15% | Tiered output, blast radius, tests, topo sort | Some structure | Grouped output | Flat list |

**Composite score** = 0.35×Recall + 0.30×Precision + 0.20×Explain + 0.15×Action

Note: This rubric inherently favors tools that provide structured analysis (like Contextception) over tools designed as flat file lists (like Aider's repo-map). The recall and precision dimensions are tool-neutral; the explainability and actionability dimensions are not.

### Per-Repo Composite Scores

#### httpx (60 files, fixture ground truth)

| Tool | Recall | Precision | Explain | Action | **Composite** |
|------|:------:|:---------:|:-------:|:------:|:-------------:|
| **Contextception** | A (4) | A (4) | A (4) | A (4) | **4.00** |
| Aider@4K | B (3) | C (2) | D (1) | D (1) | **1.95** |
| Aider@8K | A (4) | C (2) | D (1) | D (1) | **2.30** |
| Repomix | A (4) | D (1) | D (1) | D (1) | **1.95** |

#### Excalidraw (603 files, CC ground truth)

| Tool | Recall | Precision | Explain | Action | **Composite** |
|------|:------:|:---------:|:-------:|:------:|:-------------:|
| **Contextception** | A (4)† | A (4)† | A (4) | A (4) | **4.00** |
| Aider@4K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Aider@8K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Repomix | A (4) | D (1) | D (1) | D (1) | **1.95** |

#### Tokio (763 files, CC ground truth)

| Tool | Recall | Precision | Explain | Action | **Composite** |
|------|:------:|:---------:|:-------:|:------:|:-------------:|
| **Contextception** | A (4)† | A (4)† | A (4) | A (4) | **4.00** |
| Aider@4K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Aider@8K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Repomix | A (4) | D (1) | D (1) | D (1) | **1.95** |

#### Terraform (1,885 files, CC ground truth)

| Tool | Recall | Precision | Explain | Action | **Composite** |
|------|:------:|:---------:|:-------:|:------:|:-------------:|
| **Contextception** | A (4)† | A (4)† | A (4) | A (4) | **4.00** |
| Aider@4K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Aider@8K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Repomix | A (4) | D (1) | D (1) | D (1) | **1.95** |

#### Zulip (2,638 files, CC ground truth)

| Tool | Recall | Precision | Explain | Action | **Composite** |
|------|:------:|:---------:|:-------:|:------:|:-------------:|
| **Contextception** | A (4)† | A (4)† | A (4) | A (4) | **4.00** |
| Aider@4K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Aider@8K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Repomix | A (4) | D (1) | D (1) | D (1) | **1.95** |

#### Spring Boot (7,978 files, CC ground truth)

| Tool | Recall | Precision | Explain | Action | **Composite** |
|------|:------:|:---------:|:-------:|:------:|:-------------:|
| **Contextception** | A (4)† | A (4)† | A (4) | A (4) | **4.00** |
| Aider@4K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Aider@8K | D (1) | D (1) | D (1) | D (1) | **1.00** |
| Repomix | A (4) | D (1) | D (1) | D (1) | **1.95** |

† CC recall/precision = 100% by definition (CC output IS the ground truth). See [Fairness Considerations](#fairness-considerations).

#### Aggregate Composite Scores

| Tool | httpx | Excalidraw | Tokio | Terraform | Zulip | Spring Boot | **Average** |
|------|:-----:|:---------:|:-----:|:---------:|:-----:|:-----------:|:-----------:|
| **Contextception** | 4.00 | 4.00 | 4.00 | 4.00 | 4.00 | 4.00 | **4.00** |
| Aider@4K | 1.95 | 1.00 | 1.00 | 1.00 | 1.00 | 1.00 | **1.16** |
| Aider@8K | 2.30 | 1.00 | 1.00 | 1.00 | 1.00 | 1.00 | **1.22** |
| Repomix | 1.95 | 1.95 | 1.95 | 1.95 | 1.95 | 1.95 | **1.95** |

**Why we don't lead with these scores:** The 4.00 vs 1.16 gap overstates the practical difference. Explainability and actionability dimensions inherently favor Contextception's structured output. The recall/precision numbers tell a more honest story.

---

## Detailed Recall & Precision

### httpx (Python, 60 files) — Fixture Ground Truth

These are the most credible numbers in this comparison. Ground truth comes from hand-verified fixture files.

| File | Tool | Recall | Precision | F1 | Output Files | Tokens |
|------|------|-------:|----------:|---:|-----------:|-------:|
| `_client.py` | **CC** | **100%** (10/10) | **100%** (10/10) | **1.00** | 10 | ~700 |
| | Aider@4K | 70% (7/10) | 37% (7/19) | 0.48 | 19 | ~3,700 |
| | Aider@8K | 80% (8/10) | 32% (8/25) | 0.46 | 25 | ~7,000 |
| `_models.py` | **CC** | **90%** (9/10) | **90%** (9/10) | **0.90** | 10 | ~1,275 |
| | Aider@4K | 80% (8/10) | 42% (8/19) | 0.55 | 19 | ~3,340 |
| | Aider@8K | 90% (9/10) | 36% (9/25) | 0.51 | 25 | ~6,590 |
| `_auth.py` | **CC** | **100%** (6/6) | **100%** (6/6) | **1.00** | 6 | ~686 |
| | Aider@4K | 100% (6/6) | 32% (6/19) | 0.48 | 19 | ~3,090 |
| | Aider@8K | 100% (6/6) | 24% (6/25) | 0.39 | 25 | ~6,590 |

**httpx fixture summary:**

| Metric | CC | Aider@4K | Aider@8K |
|--------|---:|--------:|---------:|
| Avg recall | **97%** | 83% | 90% |
| Avg precision | **97%** | 37% | 31% |
| Avg F1 | **0.97** | 0.51 | 0.45 |
| Avg tokens | **887** | 3,377 | 6,727 |

Aider@8K achieves higher recall than @4K but *lower* F1 because the additional files are mostly noise.

### httpx (Results.json — Full Archetype Set)

The archetype-based comparison tests 6 httpx files (different selection than the 3 fixture files above). Ground truth here is CC's own `must_read` output.

| File | Archetype | CC must_read | Aider@4K Recall | Aider@4K F1 | Aider@8K Recall | Aider@8K F1 |
|------|-----------|:-------:|:----------:|:-----:|:----------:|:-----:|
| `_models.py` | Model | 10 | 70% | 0.12 | 90% | 0.09 |
| `_types.py` | Utility | 10 | 70% | 0.12 | 100% | 0.10 |
| `_api.py` | Endpoint | 6 | 100% | 0.10 | 100% | 0.06 |
| `_auth.py` | Auth | 6 | 100% | 0.10 | 100% | 0.06 |
| `_config.py` | Config | 7 | 86% | 0.11 | 100% | 0.07 |
| `__init__.py` | Barrel | 10 | 80% | 0.13 | 90% | 0.09 |
| **Average** | | **8.2** | **84%** | **0.11** | **97%** | **0.08** |

Note: F1 is very low despite high recall because Aider outputs 120–230 files per analysis in httpx, yielding precision of 3–7%. The file_count in results.json includes all files in Aider's repo-map output, not just source files matching CC's scope.

### Excalidraw (TypeScript, 603 files)

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall | CC Tokens |
|------|-----------|:-------:|:----------:|:----------:|:--------:|
| `element/src/store.ts` | Service | 9 | 22% | 22% | 1,031 |
| `excalidraw/types.ts` | Model | 10 | 20% | 30% | 1,556 |
| `TTDDialog/.../useMermaidRenderer.ts` | Plugin | 10 | 30% | 30% | 956 |
| `common/src/index.ts` | Utility | 10 | 20% | 50% | 1,097 |
| `excalidraw-app/app_constants.ts` | Config | 10 | 0% | 10% | 1,166 |
| `element/src/index.ts` | Barrel | 10 | 10% | 10% | 2,185 |
| `with-script-in-browser/utils.ts` | Leaf | 1 | 0% | 0% | 296 |
| `tests/LanguageList.test.tsx` | Test | 4 | 50% | 50% | 453 |
| **Average** | | **8.0** | **19%** | **25%** | **1,093** |

Aider@8K outputs 78 files per analysis but finds only 25% of dependencies. The fundamental issue: `@excalidraw/utils` and tsconfig path aliases are opaque to tree-sitter.

### Tokio (Rust, 763 files)

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall | CC Tokens |
|------|-----------|:-------:|:----------:|:----------:|:--------:|
| `sync/rwlock/write_guard.rs` | Plugin | 7 | 0% | 0% | 857 |
| `io/mod.rs` | Utility | 10 | 0% | 10% | 1,467 |
| `sync/cancellation_token.rs` | Auth | 9 | 0% | 0% | 860 |
| `cfg.rs` | Leaf | 7 | 0% | 0% | 457 |
| `fs/open_options/mock_open_options.rs` | Config | 3 | 0% | 0% | 657 |
| `loom/mod.rs` | Barrel | 2 | 0% | 0% | 1,029 |
| `tests-build/.../macros_core_no_default.rs` | Test | 1 | 0% | 0% | 357 |
| **Average** | | **5.6** | **0%** | **1%** | **812** |

Aider's tree-sitter parser produces definitions/references but doesn't understand Rust's `mod` declarations, `crate::` paths, or `use` re-exports. Result: near-complete failure.

### Terraform (Go, 1,885 files)

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall | CC Tokens |
|------|-----------|:-------:|:----------:|:----------:|:--------:|
| `builtin/.../data_source_state.go` | Service | 10 | 0% | 0% | 894 |
| `providers/addressed_types.go` | Model | 9 | 11% | 11% | 1,103 |
| `stackruntime/hooks/callbacks.go` | Plugin | 10 | 0% | 0% | 1,190 |
| `tfdiags/compare.go` | Utility | 10 | 20% | 50% | 1,103 |
| `rpcapi/dependencies.go` | Endpoint | 10 | 0% | 0% | 1,096 |
| `backend/remote-state/oci/auth.go` | Auth | 10 | 0% | 0% | 996 |
| `configs/util.go` | Leaf | 10 | 0% | 10% | 635 |
| `command/workdir/backend_config_state.go` | Config | 10 | 0% | 0% | 1,174 |
| `addrs/action_test.go` | Test | 1 | 0% | 0% | 275 |
| **Average** | | **8.9** | **3%** | **8%** | **941** |

The only non-zero results are for `tfdiags/compare.go` (a high fan-in utility with 434 dependents) and `addressed_types.go` — files that happen to contain symbols referenced globally. Same-package Go files are invisible to Aider.

### Zulip (Python/TypeScript, 2,638 files)

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall | CC Tokens |
|------|-----------|:-------:|:----------:|:----------:|:--------:|
| `web/src/click_handlers.ts` | Controller | 10 | 10% | 10% | 596 |
| `zerver/models/__init__.py` | Model | 10 | 10% | 20% | 1,545 |
| `zerver/decorator.py` | Middleware | 10 | 40% | 50% | 1,611 |
| `webhooks/statuspage/view.py` | Endpoint | 7 | 43% | 57% | 439 |
| `zerver/views/auth.py` | Auth | 10 | 20% | 30% | 1,592 |
| `tools/setup/emoji/emoji_setup_utils.py` | Leaf | 1 | 0% | 0% | 162 |
| `web/src/settings_config.ts` | Config | 8 | 25% | 25% | 1,267 |
| **Average** | | **8.0** | **21%** | **27%** | **1,030** |

Zulip is a mixed Python/TypeScript repo. Aider performs better on the Python files (which use standard imports that produce tree-sitter edges) than the TypeScript ones. Best result: `statuspage/view.py` at 57%, a small webhook handler with well-known imports.

### EF Core (C#, 5,708 files)

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall | CC Tokens |
|------|-----------|:-------:|:----------:|:----------:|:--------:|
| `SqlServerServiceCollectionExtensions.cs` | Service | 10 | 0% | 0% | 1,317 |
| `CountryRegion.cs` | Model | 10 | 0% | 10% | 2,224 |
| `IMemberTranslatorPlugin.cs` | Plugin | 10 | 0% | 0% | 2,151 |
| `IJsonValueReaderWriterSource.cs` | Utility | 10 | 0% | 0% | 1,585 |
| `ViewColumnBuilder.cs` | Endpoint | 10 | 0% | 0% | 1,160 |
| `SessionTokenStorageFactory.cs` | Auth | 10 | 10% | 10% | 1,080 |
| `RelationalConverterMappingHints.cs` | Leaf | 0 | 0% | 0% | 138 |
| `ConfigurationSourceExtensions.cs` | Config | 10 | 0% | 0% | 1,705 |
| `DbSetOperationTests.cs` | Test | 1 | 0% | 0% | 398 |
| `MigrationsOperations.cs` | Migration | 10 | 10% | 10% | 1,140 |
| `AdHocMapper.cs` | Serialization | 10 | 10% | 10% | 1,490 |
| `DefaultValueBinding.cs` | Error | 10 | 10% | 10% | 1,759 |
| `CosmosClientWrapper.cs` | CLI | 10 | 0% | 0% | 2,036 |
| `CommandErrorEventData.cs` | Event | 10 | 0% | 0% | 2,051 |
| `CSharpDbContextGenerator.Interfaces.cs` | Interface | 10 | 0% | 0% | 1,942 |
| `ITableBasedExpression.cs` | Orchestrator | 10 | 0% | 0% | 1,277 |
| `ComplexTypesTrackingSqlServerTest.cs` | Hotspot | 8 | 0% | 0% | 700 |
| **Average** | | **8.8** | **2%** | **3%** | **1,420** |

C# uses namespace-level `using` directives that Aider's tree-sitter parser cannot resolve to files. At 5,708 files, Aider outputs 126–161 files per query (its tree-sitter-based global ranking) but only 2–3% overlap with actual dependencies. The few hits (10% recall on 4 files) come from files that happen to appear in Aider's global ranking by coincidence, not because it traced the import path. Contextception resolves namespaces via `.csproj` project detection, namespace-to-directory mapping, and same-namespace sibling discovery.

### Spring Boot (Java, 7,978 files)

| File | Archetype | CC must_read | Aider@4K Recall | Aider@8K Recall | CC Tokens |
|------|-----------|:-------:|:----------:|:----------:|:--------:|
| `maven/CustomLayersProvider.java` | Service | 10 | 0% | 0% | 1,158 |
| `logback/ElasticCommon...Formatter.java` | Model | 10 | 0% | 0% | 3,233 |
| `maven/AbstractDependencyFilterMojo.java` | Plugin | 10 | 0% | 0% | 2,760 |
| `autoconfigure/AutoConfiguration.java` | Utility | 10 | 0% | 0% | 2,883 |
| `webmvc/.../WelcomePageNot...Mapping.java` | Endpoint | 10 | 0% | 0% | 1,843 |
| `actuator/endpoint/SecurityContext.java` | Auth | 10 | 0% | 0% | 1,960 |
| `gradle/util/package-info.java` | Leaf | 1 | 0% | 0% | 385 |
| `http/client/HttpClientSettings.java` | Config | 10 | 0% | 0% | 3,363 |
| `gradle/.../BootBuildImageIntTests.java` | Test | 10 | 0% | 0% | 1,396 |
| **Average** | | **9.0** | **0%** | **0%** | **2,109** |

Complete failure. Aider outputs only 10–19 files per analysis (compared to 60–120 in Python repos) because its tree-sitter parser produces very few cross-file edges in Java. Java's `import org.springframework.boot.maven.CacheInfo` is a package-qualified class reference that tree-sitter parses syntactically but doesn't resolve to a file path.

---

## Derived Metrics

### Recall by Repo Size

| Repo | Files | Aider@4K Mean | Aider@4K Median | Aider@8K Mean | Aider@8K Median |
|------|------:|---------:|--------:|---------:|--------:|
| httpx | 60 | 84% | 83% | 97% | 100% |
| Excalidraw | 603 | 19% | 20% | 25% | 26% |
| Tokio | 763 | 0% | 0% | 1% | 0% |
| Terraform | 1,885 | 3% | 0% | 8% | 0% |
| Zulip | 2,638 | 21% | 20% | 27% | 25% |
| EF Core | 5,708 | 2% | 0% | 3% | 0% |
| Spring Boot | 7,978 | 0% | 0% | 0% | 0% |

Medians tell a starker story than means: outside httpx, Aider's median recall is 0–25%.

### Token Efficiency

| Repo | CC Avg Tokens | Aider@4K Avg Tokens | Aider@8K Avg Tokens | CC Tokens/must_read File | Repomix Total |
|------|:---:|:---:|:---:|:---:|:---:|
| httpx | 748 | 3,256 | 6,694 | ~91 | 198K |
| Excalidraw | 990 | 3,303 | 6,891 | ~124 | 2.5M |
| Tokio | 812 | 3,236 | 7,039 | ~145 | 1.4M |
| Terraform | 941 | 3,671 | 6,891 | ~106 | 5.6M |
| Zulip | 837 | 4,173 | 7,237 | ~105 | 17.6M |
| Spring Boot | 2,109 | 4,223 | 8,431 | ~234 | 9.8M |

CC averages **91–234 tokens per relevant must_read file**. Total output is 3–5x smaller than Aider@4K, while recall is higher.

### Aggregate Summary

| Metric | CC | Aider@4K | Aider@8K |
|--------|---:|---------:|---------:|
| Avg recall (all 68 files) | 100%† | 14% | 18% |
| Avg precision (all 68 files) | 100%† | 1.6% | 1.2% |
| Avg tokens per analysis | 1,174 | 3,600 | 7,200 |
| Languages with >0% recall | 5/5 | 2/5 | 3/5 |

† Against own output; see Tier 2 ground truth caveat.

---

## Why Aider Gets 0% on Go, Java, Rust, and C#

Aider uses tree-sitter to parse source files and extract definitions (classes, functions, methods) and references (identifiers used). It then builds a graph and applies PageRank to select the most "important" files.

The critical gap: **tree-sitter parses syntax, not semantics**. It doesn't resolve:

| Language | What's Missing | Example |
|----------|---------------|---------|
| **Go** | Module path → file mapping | `import "internal/tfdiags"` → which files? |
| **Go** | Same-package resolution | Files in the same directory share a namespace without imports |
| **Java** | Package → directory mapping | `import org.springframework.boot.maven.CacheInfo` → which file? |
| **Java** | Multi-module project structure | Maven/Gradle modules, source sets |
| **Rust** | `mod` declarations | `mod write_guard;` → `write_guard.rs` |
| **Rust** | `crate::` path resolution | `use crate::sync::batch_semaphore` → which file? |
| **Rust** | Cargo workspace resolution | Cross-crate dependencies via `Cargo.toml` |
| **C#** | Namespace → file mapping | `using Microsoft.EntityFrameworkCore` → which files? |
| **C#** | .csproj project structure | Multi-project solutions with dotted directory names |
| **C#** | Same-namespace visibility | Files in the same directory share implicit type visibility |

Contextception resolves all of these via language-specific resolvers (Go: go.mod/go.work, Java: package conventions, Rust: Cargo.toml + mod tree, C#: .csproj detection + namespace-to-directory mapping).

For Python and TypeScript, Aider fares better because tree-sitter can extract import paths that are more directly mappable to files — though it still misses tsconfig paths, workspace packages, and Python package-relative imports.

---

## Fairness Considerations

1. **Aider's intended use case.** Aider's repo-map is designed as internal context for its own LLM-based editing workflow. It provides code signatures (class/function definitions) alongside file references, which is valuable for an LLM that needs to understand APIs. We're evaluating the file selection aspect only, ignoring the signature content that makes Aider's output useful within its own system.

2. **Contextception as ground truth.** For 6 of 7 repos, CC's output is the ground truth. This makes CC's recall/precision numbers meaningless in isolation. The comparison is still valid because Aider's recall is measured against the same ground truth — if CC's ground truth is wrong, Aider's numbers would also be wrong (potentially in Aider's favor if CC over-includes files).

3. **Composite score bias.** The 4-dimension rubric awards points for explainability and actionability. Aider's repo-map is a flat file list by design — it can't score well on these dimensions regardless of file selection quality. The recall/precision dimensions are the fair comparison.

4. **Token budget generosity.** Aider's default `map_tokens` is 1024. We tested at 2048, 4096, and 8192 — 2–8x the default. At 1024, results would be worse.

5. **Repo selection.** Repos were chosen for language diversity and scale variety, not to favor either tool. httpx (where Aider performs best) is featured prominently. Spring Boot and Tokio (where Aider performs worst) are included for completeness but don't drive the headline numbers.

6. **Aider version.** We tested Aider v0.86.2. Newer versions may improve tree-sitter language support, though the fundamental PageRank-vs-import-resolution difference is architectural.

---

## What Contextception's Output Includes (That Others Don't)

For completeness, here's what Contextception provides beyond a file list:

| Feature | CC | Aider | Repomix |
|---------|:--:|:-----:|:-------:|
| Dependency direction (imports/imported_by) | Yes | No | No |
| Per-file symbols used | Yes | Definitions only | Full source |
| Reading order (topo sort) | Yes | No | No |
| Likely-modify predictions | Yes | No | No |
| Test file discovery | Yes | No | No |
| Blast radius / risk score | Yes | No | No |
| Confidence score | Yes | No | No |
| Hotspot detection | Yes | No | No |
| Circular dependency detection | Yes | No | No |
| Hidden coupling (co-change) | Yes | No | No |

These features are why the composite score gap is larger than the recall gap. They also represent real engineering value — but measuring them comparatively is subjective, which is why we focus on recall/precision in the headline results.

---

## Reproducibility

All data is derived from [`data/results.json`](data/results.json). Scripts:

- `scripts/compare/run_comparison.sh` — orchestrates all tool runs
- `scripts/compare/extract_aider_map.py` — extracts file lists from Aider's raw output
- `scripts/compare/results.json` — canonical copy of results

Raw tool outputs were saved to `/tmp/compare-results/` during the comparison run:
- `cc_<repo>_<file>.json` — Contextception JSON output
- `aider_<repo>_<budget>t.txt` — Aider raw repo-map text
- `aider_<repo>_<budget>t_files.json` — Extracted file lists
- `repomix_<repo>.xml` — Full Repomix output
