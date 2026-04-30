# OpenAI Codex / generic-MCP agents

OpenAI Codex (and any custom agent that speaks the Model Context Protocol) can use contextception as an MCP server over stdio.

This file configures the server. The actual agent instructions — when to reach for contextception and when to skip it — live in the canonical snippet at [`../AGENTS.md`](../AGENTS.md).

## Install

```bash
go install github.com/kehoej/contextception/cmd/contextception@latest
```

Or via Homebrew:

```bash
brew install kehoej/tap/contextception
```

## MCP configuration

Configure your agent to launch contextception over stdio:

```
Command: contextception
Args:    mcp
Transport: stdio
```

The server auto-detects the repository root from the current working directory using git, and auto-indexes the repo on the first call.

## Agent instructions

Drop [`../AGENTS.md`](../AGENTS.md) at your project root as `AGENTS.md`. Codex (and most other AGENTS.md-aware tools) will pick it up automatically.

## Tool reference (quick)

| Tool | Purpose |
|---|---|
| `get_context(file)` | Dependency context for a file: `must_read`, `likely_modify`, `tests`, `blast_radius`. Accepts a string or array of paths. |
| `analyze_change(base, head)` | Diff-level risk: blast radius per file, test gaps, coupling signals, hotspots. Best tool for PR review. |
| `get_structure()` | Directory layout + language distribution. First call when orienting on an unfamiliar repo. |
| `get_entrypoints()` | Entrypoint files (main, CLI) and most-depended-on foundation files. |
| `get_archetypes()` | One representative file per architectural layer (Service/Controller, Auth, Hotspot, etc., 18 categories). |
| `search(query, type)` | Find files by path pattern or symbol name (`type: "symbol"`). |
| `index()`, `status()` | Index management. The server auto-indexes; you rarely call these directly. |
| `rate_context(file, usefulness, ...)` | Feedback (1–5) on a previous `get_context` result, with `useful_files` / `unnecessary_files` / `missing_files`. Improves accuracy over time. |

For full parameter documentation, see [`../../docs/features.md`](../../docs/features.md).
