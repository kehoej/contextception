# CLAUDE.md — Contextception integration template

Copy this file to your project root as `CLAUDE.md` (or append to your existing one) to enable contextception for Claude Code. This stub configures the MCP server; the actual instruction body lives in [`../AGENTS.md`](../AGENTS.md) and is shared across every supported coding tool.

## MCP server configuration

Add to `~/.claude.json` (global) or your project's `.claude/settings.json`:

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

`contextception setup` (with no args) writes this for you, plus the `/pr-risk` and `/pr-fix` slash commands. Older versions also installed a PreToolUse hook; if your settings still contain one, the next `setup` run silently strips it.

## Agent instructions

The instruction body tells Claude Code **when** calling `get_context` / `analyze_change` is worth doing — and, just as importantly, when it isn't.

Two ways to install it:

1. **Automatic (recommended):** from inside your project, run `contextception setup --instructions`. It upserts a marker-fenced block into `CLAUDE.md`, preserving any other content. Safe to re-run; `--uninstall --instructions` strips just the block.
2. **Manual:** append the body of [`../AGENTS.md`](../AGENTS.md) to your project's `CLAUDE.md` (or to `~/.claude/CLAUDE.md` for a global install).
