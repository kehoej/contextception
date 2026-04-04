# Contextception Integration for OpenAI Codex / Agents

OpenAI Codex and custom agents can use contextception as an MCP server over stdio transport.

## Setup

### 1. Install contextception

```bash
go install github.com/kehoej/contextception/cmd/contextception@latest
```

Or via Homebrew:

```bash
brew install kehoej/tap/contextception
```

### 2. Configure MCP in your agent

Add contextception as an MCP tool server in your agent configuration. The server uses stdio transport:

```
Command: contextception
Args: mcp
Transport: stdio
```

The server auto-detects the repository root from the current working directory using git.

### 3. Agent instructions

Add the following to your agent's system prompt or instructions file:

```
Before modifying any source file, call the `get_context` MCP tool with the file path.
Read the returned `must_read` files to understand dependencies before making changes.
Check `blast_radius` to assess the risk level of the modification.
Check `tests` to identify which test files cover the subject.
```

## Available MCP Tools

### get_context

Analyze a file's dependency context. Auto-indexes the repository on first call.

**Parameters:**
- `file` (required) — repo-relative or absolute path, or array of paths for multi-file analysis
- `mode` — workflow mode: `plan`, `implement`, or `review` (adjusts output caps)
- `token_budget` — target token budget for output (adjusts caps automatically)
- `omit_external` — omit external dependencies from output
- `include_signatures` — include code signatures for must_read symbols
- `max_must_read` — max must_read entries (default 10)
- `max_related` — max related entries (default 10)
- `max_likely_modify` — max likely_modify entries (default 15)
- `max_tests` — max test entries (default 5)

### index

Build or update the repository index. Uses incremental indexing when possible.

**Parameters:** none

### status

Return index diagnostics: file count, edge count, staleness, last indexed commit.

**Parameters:** none

### search

Search the index for files by path pattern or symbol name.

**Parameters:**
- `query` (required) — search query string
- `type` — `path` (default) or `symbol`
- `limit` — max results (default 50, max 100)

### get_entrypoints

Return the project's entrypoint files and foundation files (most depended-upon). Use for initial project orientation.

**Parameters:**
- `limit` — max foundation files to return (default 10)

### get_structure

Return directory structure with file counts and language distribution. Use as the first call when exploring an unfamiliar project.

**Parameters:** none

### get_archetypes

Detect representative files across architectural layers. Returns one file per archetype category.

**Parameters:**
- `categories` — optional list of categories to filter. Available: Service/Controller, Model/Schema, Middleware/Plugin, High Fan-in Utility, Page/Route/Endpoint, Auth/Security, Leaf Component, Config/Constants, Barrel/Index, Test File, Database/Migration, Serialization/Validation, Error Handling, CLI/Command, Event/Message, Interface/Contract, Orchestrator, Hotspot

### analyze_change

Analyze the impact of a git diff (PR or branch). Returns blast radius, test gaps, and coupling signals.

**Parameters:**
- `base` — base git ref (auto-detects merge-base if omitted)
- `head` — head git ref (defaults to HEAD)

## Recommended agent workflow

1. On project start: call `get_structure` then `get_entrypoints` for orientation
2. Before each file modification: call `get_context` on the target file
3. Read `must_read` files before making changes
4. After a set of changes: call `analyze_change` to verify impact
