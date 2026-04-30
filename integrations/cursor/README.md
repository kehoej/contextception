# Cursor

Cursor reads MCP configuration from a JSON file and project-scoped instructions from `.cursor/rules/`.

## MCP server

The MCP config snippet ships next to this README at [`mcp.json`](mcp.json). Place it at one of:

- `.cursor/mcp.json` (per project — recommended)
- `~/.cursor/mcp.json` (global, applies to every project)

```bash
mkdir -p .cursor
cp ../mcp.json .cursor/mcp.json   # from this directory
```

Then restart Cursor — contextception's nine tools will appear in the MCP tool list.

## Agent instructions

Either run `contextception setup --instructions` from inside your project (it writes the file with the right frontmatter the first time and upserts via begin/end markers on later runs), or copy [`../AGENTS.md`](../AGENTS.md) verbatim to:

```
.cursor/rules/contextception.mdc
```

Cursor's `.mdc` rule files take precedence when they include a frontmatter block. Use this header to ensure Cursor always loads the rule:

```mdc
---
description: Tells Cursor when to call contextception MCP tools
alwaysApply: true
---

<paste body of ../AGENTS.md here>
```

If you're on an older Cursor that still uses the legacy single-file format, `.cursorrules` at the project root also works — paste the body of `AGENTS.md` directly into it (no frontmatter needed).
