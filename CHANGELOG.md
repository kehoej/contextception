# Changelog

All notable changes to Contextception will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

- **5-language support:** Python, TypeScript/JavaScript, Go, Java, Rust
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
