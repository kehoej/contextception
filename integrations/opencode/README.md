# opencode

[opencode](https://opencode.ai) supports MCP servers via its `opencode.json` (or `opencode.jsonc`) project config and reads project rules from `AGENTS.md`.

## MCP server

Add the following to `opencode.json` at your project root (or merge into your existing config):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "contextception": {
      "type": "local",
      "command": ["contextception", "mcp"],
      "enabled": true
    }
  }
}
```

Restart opencode — `get_context`, `analyze_change`, etc. will appear as available tools.

## Agent instructions

Either run `contextception setup --instructions` from inside your project (it appends a marker-fenced block to existing `AGENTS.md` files, leaves your other rules alone, and is idempotent on re-runs), or copy [`../AGENTS.md`](../AGENTS.md) verbatim to your project root as:

```
AGENTS.md
```

opencode auto-loads `AGENTS.md` and incorporates it into every agent prompt.
