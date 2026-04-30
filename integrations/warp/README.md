# Warp

[Warp](https://warp.dev) configures MCP servers through its app UI and reads project rules from an all-caps `AGENTS.md` at the project root (or `WARP.md` for the legacy convention).

## MCP server

Warp does not read MCP config from a file in your repo. Instead, register the contextception MCP server through the Warp UI:

1. Open **Settings → Agents → MCP servers** (or run **Open MCP Servers** from the command palette).
2. Click **+ Add** and choose **CLI command**.
3. Configure:
   - **Name:** `contextception`
   - **Command:** `contextception`
   - **Args:** `mcp`
4. Save and restart the agent panel.

Once registered, the contextception tools are available across every Warp Agent session, not just one project.

## Agent instructions

Either run `contextception setup --instructions` from inside your project (it appends a marker-fenced block to existing `AGENTS.md` files, leaves your other rules alone, and is idempotent on re-runs), or copy [`../AGENTS.md`](../AGENTS.md) verbatim to your project root as:

```
AGENTS.md
```

Note: **Warp requires the filename to be in all caps** — `AGENTS.md` is recognized, `agents.md` is not. Warp also still honors the legacy `WARP.md` convention if you're migrating from one. Subdirectory-level `AGENTS.md` files are merged automatically with the project-root one.
