# Changelog

All notable changes to Contextception will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.8] - 2026-04-21

### Added

- **C# first-class language support:** full extractor for C# covering classes, interfaces, structs, enums, methods, properties, fields, events, delegates, and namespace resolution — bringing supported languages to 6
- **Auto-reindexing in `analyze`:** the `analyze` command now automatically reindexes stale or missing files before analysis, ensuring results always reflect the current state of the codebase

## [1.0.7] - 2026-04-12

### Added

- **Per-file risk scoring:** every changed file in `analyze-change` gets a 0-100 risk score with tier (SAFE/REVIEW/TEST/CRITICAL), contributing factors, and a human-readable narrative explaining why
- **Aggregate risk and triage:** change reports now include an overall risk assessment with risk triage grouping files by tier, plus historical percentile ranking ("riskier than 80% of past changes")
- **Test suggestions:** change reports automatically suggest which tests to write or run based on risk analysis and coverage gaps
- **`rate_context` MCP tool:** LLMs can submit structured feedback (usefulness 1-5, useful/unnecessary/missing files) on `get_context` results, enabling closed-loop accuracy measurement
- **`contextception gain` command:** usage analytics dashboard showing analysis counts, top files, confidence trends, and blast radius distribution over time (daily/weekly/monthly breakdowns, JSON export)
- **`contextception discover` command:** scans Claude Code sessions to find files edited without `get_context` being called first — surfaces missed adoption opportunities
- **`contextception accuracy` command:** precision, recall, and usefulness metrics computed from LLM feedback submitted via `rate_context`
- **`contextception session` command:** adoption tracking across Claude Code sessions showing per-session coverage percentages
- **Compact output mode:** `--compact` flag on `analyze` produces a token-optimized text summary (~60-75% fewer tokens than JSON)
- **Usage analytics tracking:** all CLI and MCP operations record duration, mode, and result metrics to the history database
- **Working tree fallback for `analyze-change`:** diffs uncommitted working tree changes when no committed changes exist between refs, supporting pre-PR workflows
- **Auto-reindex on `analyze-change`:** ensures the index reflects files on disk before analysis

### Changed

- **Change reports sorted by risk:** changed files are now sorted by risk score descending

### Fixed

- **Cross-platform file path normalization:** file paths are normalized to forward slashes, fixing path resolution on Windows (#8)

## [1.0.6] - 2026-04-09

### Fixed

- **`contextception setup` upgrades stale hooks:** running `setup` now detects and replaces old hook variants (e.g. `hook-check` from v1.0.4) with the current `hook-context` command, instead of silently skipping when any contextception hook is already configured

## [1.0.5] - 2026-04-09

### Added

- **`contextception hook-context` command:** PreToolUse hook that runs the full analyzer and injects dependency context directly into Claude's context window via the `additionalContext` JSON protocol — Claude automatically sees must-read files, blast radius, and test coverage before every edit
- **Hook analyzer mode:** new `"hook"` mode preset with tight caps (5 must_read, 0 likely_modify, 2 tests, 0 related) optimized for low-latency hook invocations
- **`/release` command:** Claude Code slash command for AI-powered release automation — generates changelog entries from commits, updates CHANGELOG.md, commits, tags, and pushes to trigger the CI/CD pipeline
- **`make release` target:** shows release info (latest tag, next version, pending commits)

### Changed

- **Setup installs `hook-context` instead of `hook-check`:** new installations get the context-injecting hook by default; `hook-check` remains for backward compatibility
- **Mode presets respect zero caps:** analyzer mode presets can now set caps to 0 without being overridden by defaults

## [1.0.4] - 2026-04-09

### Added

- **`contextception setup` command:** one-command configuration for Claude Code, Cursor, and Windsurf — adds MCP server config and PreToolUse hooks automatically
- **`contextception hook-check` subcommand:** native Go replacement for the shell-based hook script — zero external dependencies (no python3 required)
- **Multi-editor support:** `--editor claude|cursor|windsurf` flag configures the correct config file for each editor
- **Setup reversibility:** `--uninstall` flag removes all contextception configuration, `--dry-run` previews changes
- **Surgical JSON editing:** uses tidwall/sjson to modify config files without reordering keys

## [1.0.3] - 2026-04-08

### Added

- **Automatic update notifications:** checks for new versions once per day (cached) and prints a one-line notification to stderr when an update is available
- **`contextception update` command:** detects install method (Homebrew, go install, direct download) and updates accordingly
- **Minisign release signing:** release checksums are signed with minisign; self-update requires a valid signature
- **Global configuration:** platform-native config at `os.UserConfigDir()/contextception/config.yaml` for update settings
- **Update suppression:** `--no-update-check` flag, `CONTEXTCEPTION_NO_UPDATE_CHECK=1` env var, or `update.check: false` in global config

## [1.0.2] - 2026-04-07

### Changed

- Upgrade Go from 1.24 to 1.25
- Migrate golangci-lint from v1 to v2 (v2.11.4)
- Bump go-sdk from v1.3.0 to v1.5.0
- Bump modernc.org/sqlite from v1.45.0 to v1.48.1
- Replace `WriteString(Sprintf(...))` with `fmt.Fprintf` for efficiency
- Use tagged switch statements where appropriate

## [1.0.1] - 2026-04-06

### Changed

- Fix gofmt -s formatting across 30 files

## [1.0.0] - 2026-03-25

### Added

- **6-language support:** Python, TypeScript/JavaScript, Go, Java, Rust, C#
- **CLI with 10 commands:** analyze, analyze-change, search, archetypes, history, index, reindex, extensions, status, mcp
- **MCP server** with 8 tools for integration with Claude Code, Cursor, Windsurf, and other AI tools
- **Schema 3.2 output** with confidence scoring, role classification, code signatures, and direction field
- **Incremental indexing** with concurrent file processing and schema-aware reindex
- **Git history signals:** co-change detection, churn tracking, hotspot flagging
- **Blast radius analysis** with fragility metric and hidden coupling detection
- **Circular dependency detection** via bounded DFS
- **Branch diff analysis** (`analyze-change`) for PR-level impact reports
- **Index search** by path pattern or symbol name
- **AI workflow modes** (`--mode plan|implement|review`) for context-aware output shaping
- **Token budget awareness** (`--token-budget N`) for output size control
- **CI integration** (`--ci --fail-on high|medium`) with deterministic exit codes
- **Configuration system** (`.contextception/config.yaml`) for entrypoints, ignore patterns, and generated file markers
- **Multi-file analysis** (`AnalyzeMulti`) for analyzing multiple files in a single call
- **Topo-sorted must_read** with foundational-first ordering
- Validated across 16 real-world repositories (419 files evaluated, overall grade A/3.85)
