# GitHub Copilot in VSCode

Recent VSCode releases (1.99+) added Model Context Protocol support to Copilot Chat. If you're on a recent build, both the MCP path and the instruction-file path work. On older builds, contextception is still consumable via its CLI — the agent runs `contextception analyze <file> --compact` instead of `get_context`.

## MCP server (recent VSCode + Copilot Chat)

Add to `.vscode/mcp.json` at your project root (or use the GUI: **MCP: Add Server** from the command palette):

```json
{
  "servers": {
    "contextception": {
      "type": "stdio",
      "command": "contextception",
      "args": ["mcp"]
    }
  }
}
```

Reload the window. The Copilot Chat agent picker will list `contextception` as an available tool source.

## Agent instructions (always)

Either run `contextception setup --instructions` from inside your project (it appends a marker-fenced block and is safe to re-run), or copy [`../AGENTS.md`](../AGENTS.md) verbatim to:

```
.github/copilot-instructions.md
```

Copilot Chat auto-loads this file repo-wide and prepends its body to every agent prompt.

## CLI fallback (no MCP available)

If your Copilot Chat build does not yet support MCP, the agent can still call contextception directly via shell:

```
contextception analyze <file> --compact          # equivalent of get_context
contextception analyze-change --compact          # equivalent of analyze_change
contextception search "<query>" --type symbol    # equivalent of search(symbol)
contextception status                            # index health
```

The `--compact` output is token-optimized and well-suited for paste-back into the chat. The CLI fallback is documented at the bottom of [`../AGENTS.md`](../AGENTS.md) so the agent will reach for it automatically when MCP isn't available.
