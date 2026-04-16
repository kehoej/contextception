# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Contextception** is a code context intelligence engine written in **Go**. It answers: *"What code must be understood before making a safe change?"* It is not a code generator, AI assistant, or IDE — it determines what matters, not what to do.

Supports 6 languages: Python, TypeScript/JavaScript, Go, Java, Rust, and C#. Available as a CLI (16 commands) and MCP server (9 tools).

## Tech Stack

- **Language:** Go (single binary, concurrency for indexing)
- **Storage:** SQLite via `modernc.org/sqlite` (pure-Go, embedded)
- **CLI:** `spf13/cobra`
- **TS/JS Extraction:** `smacker/go-tree-sitter` (CGO) with regex fallback
- **MCP Server:** `modelcontextprotocol/go-sdk` (stdio transport)
- **Config:** `gopkg.in/yaml.v3`
- **Semver:** `golang.org/x/mod/semver`
- **Release Signing:** `jedisct1/go-minisign` (signature verification for self-update)

## Architecture

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full architecture guide with diagrams.

```
Repository → Language Extractors → SQLite Index → Analysis Engine → Context Bundle (JSON)
```

Key components: Indexer (`internal/indexer/`), Language Extractors (`internal/extractor/`), Resolvers (`internal/resolver/`), Analysis Engine (`internal/analyzer/`), Change Analysis (`internal/change/`), MCP Server (`internal/mcpserver/`), Update (`internal/update/`).

## Key Design Principles

1. **Deterministic first** — outputs are explainable and reproducible; no model-dependent core logic
2. **Static truth before AI** — signals from dependency graphs, repo structure, change patterns
3. **Read-only by design** — never modifies source, never executes arbitrary commands
4. **Incremental** — only changed files reprocessed; schema migrations are versioned
5. **Pluggable extractors** — language-specific extractors emit facts; core is language-agnostic

## Development

```bash
make build      # Build binary to ./bin/contextception
make test       # Run all tests
make lint       # Run golangci-lint
make check      # Run vet + lint + test
make coverage   # Generate HTML coverage report
make install    # go install to $GOPATH/bin
make generate   # Regenerate protocol JSON schemas
make release    # Show release info (latest tag, next version, pending commits)
make help       # List all targets
/release        # Full release: generate changelog, commit, tag, push (Claude Code)

```

## Project Structure

```
cmd/contextception/    CLI entrypoint
cmd/gen-schema/        JSON schema generator
internal/
  analyzer/            Core analysis engine (scoring, categorization, risk triage, cycles)
  change/              PR/branch diff analysis
  classify/            File role classification
  cli/                 Command handlers (cobra subcommands)
  config/              Configuration parsing (per-repo + global config)
  db/                  SQLite layer (migrations, store, search)
  extractor/           Language extractors (python, typescript, golang, java, rust, csharp)
  git/                 Git history signal extraction
  grader/              Internal quality evaluation framework
  history/             Historical analysis, usage tracking, and feedback storage
  indexer/             Incremental indexing pipeline
  mcpserver/           MCP server (tools, stdio transport)
  model/               Shared data types
  resolver/            Module resolution (per-language)
  session/             Claude Code session JSONL parser (discover, adoption)
  update/              Version check, self-update, install method detection
  validation/          Fixture-based validation framework
  version/             Version injection (set via ldflags)
protocol/              JSON Schema specifications
testdata/              Test fixtures (synthetic repos + expected outputs)
benchmarks/            Head-to-head comparisons with methodology
integrations/          MCP config examples for Claude Code, Cursor, Windsurf, Codex
```

## Key Reference Documents

- `docs/ARCHITECTURE.md` — architecture guide with diagrams
- `docs/features.md` — feature reference (schema fields, CLI flags, MCP parameters)
- `docs/mcp-tutorial.md` — MCP integration tutorial
- `protocol/` — formal JSON Schema specifications
- `benchmarks/` — benchmark methodology and results
