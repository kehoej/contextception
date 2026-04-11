# Contextception MCP Integrations

Contextception exposes its analysis engine as an [MCP (Model Context Protocol)](https://modelcontextprotocol.io/) server, making it available as a tool for AI coding agents. This directory contains ready-to-use configuration files for popular AI development tools.

## Prerequisites

Install contextception via one of:

```bash
# Go install
go install github.com/kehoej/contextception/cmd/contextception@latest

# Homebrew
brew install kehoej/tap/contextception

# Shell script
curl -fsSL https://raw.githubusercontent.com/kehoej/contextception/main/install.sh | sh
```

Verify installation:

```bash
contextception --version
```

## Quick Setup

The `setup` command configures everything automatically:

```bash
# Claude Code (MCP server + PreToolUse hooks)
contextception setup

# Cursor
contextception setup --editor cursor

# Windsurf
contextception setup --editor windsurf
```

Use `--dry-run` to preview changes, or `--uninstall` to reverse. For Claude Code, this also installs hooks that remind the AI to call `get_context` before editing files.

## Manual Configuration

If you prefer to configure manually, or need per-project setup:

### Claude Code

**Files:** [`claude-code/CLAUDE.md`](claude-code/CLAUDE.md)

Claude Code discovers MCP servers from `~/.claude.json` or project-level `.claude/settings.json`.

**Setup:**

1. Add to `~/.claude.json` (global) or `.claude/settings.json` (per-project):

```json
{
  "mcpServers": {
    "contextception": {
      "command": "contextception",
      "args": ["mcp"]
    }
  }
}
```

2. Optionally copy `claude-code/CLAUDE.md` to your project root (or append its contents to your existing `CLAUDE.md`) to instruct Claude Code to automatically call `get_context` before modifying files.

3. Restart Claude Code. Contextception tools will appear in the MCP tool list.

---

### Cursor

**Files:** [`cursor/mcp.json`](cursor/mcp.json)

Cursor reads MCP configuration from `.cursor/mcp.json` in your project root or `~/.cursor/mcp.json` globally.

**Setup:**

1. Copy the config file:

```bash
# Per-project
mkdir -p .cursor
cp integrations/cursor/mcp.json .cursor/mcp.json

# Or global
cp integrations/cursor/mcp.json ~/.cursor/mcp.json
```

2. Restart Cursor. The contextception tools will be available to Cursor's AI agent.

---

### Windsurf

**Files:** [`windsurf/mcp_config.json`](windsurf/mcp_config.json)

Windsurf reads MCP configuration from `~/.codeium/windsurf/mcp_config.json`.

**Setup:**

1. Copy or merge the config:

```bash
mkdir -p ~/.codeium/windsurf
cp integrations/windsurf/mcp_config.json ~/.codeium/windsurf/mcp_config.json
```

If you already have MCP servers configured, merge the `contextception` entry into your existing `mcpServers` object.

2. Restart Windsurf. Contextception tools will be available in Cascade.

---

### OpenAI Codex / Custom Agents

**Files:** [`codex/agents.md`](codex/agents.md)

For OpenAI Codex, custom agents, or any MCP-compatible client, contextception runs as a stdio-based MCP server.

**Setup:**

```bash
contextception mcp
```

This starts the MCP server on stdin/stdout. Configure your agent to launch this command and communicate via the MCP stdio transport. See `codex/agents.md` for full tool documentation and recommended agent instructions.

---

## Available MCP Tools

All integrations expose the same nine tools:

| Tool | Description |
|------|-------------|
| `get_context` | Analyze a file's dependency context (auto-indexes). Accepts single path or array for multi-file analysis. |
| `index` | Build or update the repository index incrementally. |
| `status` | Return index diagnostics (file count, edge count, staleness). |
| `search` | Search the index by path pattern or symbol name. |
| `get_entrypoints` | Return entrypoint and foundation files for project orientation. |
| `get_structure` | Return directory structure with file counts and language distribution. |
| `get_archetypes` | Detect representative files across architectural layers (one per category). |
| `analyze_change` | Analyze the impact of a git diff / PR. Returns blast radius, test gaps, coupling signals. |
| `rate_context` | Rate how useful a previous `get_context` result was. Structured feedback for accuracy tracking. |

## Supported Languages

Contextception supports repositories using: Python, TypeScript/JavaScript, Go, Java, Rust.

## Further Reading

- [MCP Tutorial](../docs/mcp-tutorial.md) — step-by-step guide to adding context intelligence to any AI agent
- [Feature Reference](../docs/features.md) — full schema and parameter documentation
- [Configuration](../README.md#configuration) — optional `.contextception/config.yaml` setup
