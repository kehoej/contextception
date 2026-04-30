# Windsurf

Windsurf reads MCP configuration from a single global config file and project-scoped instructions from `.windsurf/rules/`.

## MCP server

The MCP config snippet ships next to this README at [`mcp_config.json`](mcp_config.json). Copy or merge it into:

```
~/.codeium/windsurf/mcp_config.json
```

```bash
mkdir -p ~/.codeium/windsurf
cp ../mcp_config.json ~/.codeium/windsurf/mcp_config.json   # from this directory
```

If you already have other MCP servers configured, merge the `contextception` entry into the existing `mcpServers` object rather than overwriting the file.

Restart Windsurf and contextception's tools will be available in Cascade.

## Agent instructions

Either run `contextception setup --instructions` from inside your project (it upserts via begin/end markers and is safe to re-run), or copy [`../AGENTS.md`](../AGENTS.md) verbatim to:

```
.windsurf/rules/contextception.md
```

Windsurf's modern rules format lives under `.windsurf/rules/`, with each `.md` file scoped to the project. The legacy single-file convention `.windsurfrules` at the project root also works if you'd rather paste the body there.
