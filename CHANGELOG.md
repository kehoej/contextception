# Changelog

All notable changes to Contextception will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
