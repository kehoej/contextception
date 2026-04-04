# CLAUDE.md — Contextception Integration Template

Copy this file to your project root as `CLAUDE.md` (or append to your existing one) to enable automatic context intelligence in Claude Code.

## MCP Server Configuration

Add to your `~/.claude.json` or project `.claude/settings.json`:

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

## Instructions for Claude Code

Use contextception MCP tools in repos with Python, TypeScript/JavaScript, Go, Java, or Rust code. Before modifying a file, call `get_context` on it to understand its dependency context. The tool auto-indexes and stays current automatically. If `.contextception/` is not in `.gitignore`, add it.

### When to use each tool

- **Before modifying any file:** Call `get_context` with the file path. Read the `must_read` files before making changes. Check `blast_radius` to understand risk.
- **When exploring an unfamiliar project:** Call `get_structure` first for an overview, then `get_entrypoints` to find key files, then `get_archetypes` to see representative files across architectural layers.
- **When reviewing a PR or branch:** Call `analyze_change` to see blast radius, test gaps, and coupling signals across all changed files.
- **When searching for a file or symbol:** Call `search` with a path pattern or symbol name (use `type: "symbol"` for symbol search).

### Example workflow: modifying a file

1. Call `get_context` on the target file
2. Read the `must_read` files to understand dependencies
3. Check `likely_modify` to see what else may need changes
4. Check `tests` to know which tests to run
5. Make the change
6. Verify by re-running `get_context` if the change affected imports

### Multi-file analysis

When modifying multiple related files, pass them all at once:

```
get_context with file: ["src/auth/login.ts", "src/auth/session.ts"]
```

This produces a merged analysis with deduplicated must_read, combined blast radius, and unified test coverage.

### Token budget optimization

For large codebases or when working within tight token limits, use the `token_budget` parameter:

```
get_context with file: "src/core/engine.ts", token_budget: 4000
```

Or use workflow modes that automatically adjust output caps:

```
get_context with file: "src/core/engine.ts", mode: "implement"
```

Available modes: `plan` (broad context), `implement` (focused, smaller output), `review` (balanced).
