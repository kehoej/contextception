# Tutorial: How to Add Context Intelligence to Your AI Agent

This guide walks through integrating contextception's MCP server with any AI coding agent. By the end, your agent will automatically understand dependency context before making code changes.

## What you get

Contextception tells your AI agent which files matter before it touches code. Instead of guessing or reading everything, the agent gets a ranked list of files it must understand, files it will likely need to modify, relevant tests, and a blast radius assessment — all from static analysis of your actual dependency graph.

## Prerequisites

### Install contextception

```bash
# Pick one:
go install github.com/kehoej/contextception/cmd/contextception@latest
brew install kehoej/tap/contextception
curl -fsSL https://raw.githubusercontent.com/kehoej/contextception/main/install.sh | sh
```

Verify:

```bash
contextception --version
```

### Supported languages

Your project must use one or more of: Python, TypeScript/JavaScript, Go, Java, Rust, C#.

### MCP-compatible agent

Any agent that supports the Model Context Protocol stdio transport: Claude Code, Cursor, Windsurf, or custom agents using an MCP client library.

## Step 1: Configure MCP for your tool

### Automatic setup (recommended)

The `setup` command configures everything in one step:

```bash
# Claude Code (adds MCP server + PreToolUse hooks)
contextception setup

# Cursor
contextception setup --editor cursor

# Windsurf
contextception setup --editor windsurf
```

For Claude Code, this also installs hooks that automatically remind the AI to call `get_context` before editing files. Use `--dry-run` to preview changes, or `--uninstall` to reverse.

### Manual configuration

If you prefer to configure manually:

<details>
<summary>Claude Code</summary>

Add to `~/.claude.json`:

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
</details>

<details>
<summary>Cursor</summary>

Add to `.cursor/mcp.json` in your project root:

```json
{
  "mcpServers": {
    "contextception": {
      "command": "contextception",
      "args": ["mcp"],
      "transportType": "stdio"
    }
  }
}
```
</details>

<details>
<summary>Windsurf</summary>

Add to `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "contextception": {
      "command": "contextception",
      "args": ["mcp"],
      "transportType": "stdio"
    }
  }
}
```
</details>

### Custom agents

Launch the MCP server and connect via stdio:

```bash
contextception mcp
```

Use any MCP client library to send tool calls over stdin/stdout.

## Step 2: First run — automatic indexing

The first time your agent calls any contextception tool, it automatically indexes the repository. This builds a dependency graph from your source files and stores it in `.contextception/index.db`.

Add `.contextception/` to your `.gitignore`:

```bash
echo '.contextception/' >> .gitignore
```

Subsequent calls use incremental indexing — only changed files are reprocessed.

## Step 3: The core workflow

### Analyze before modifying

The most important pattern: call `get_context` before modifying any file.

```
Agent receives task: "Add rate limiting to the login endpoint"

1. Agent calls get_context(file: "src/auth/login.ts")
2. Response includes:
   - must_read: ["src/auth/session.ts", "src/middleware/auth.ts"]
   - likely_modify: {high: ["src/auth/session.ts"]}
   - tests: {direct: ["tests/auth/login.test.ts"]}
   - blast_radius: {level: "medium", fragility: 0.45}
3. Agent reads must_read files to understand dependencies
4. Agent makes the change with full context
5. Agent knows to update tests/auth/login.test.ts
```

### Explore an unfamiliar project

When the agent encounters a new codebase:

```
1. get_structure → directory layout, language distribution
2. get_entrypoints → main modules and most-depended-upon files
3. get_archetypes → one representative file per architectural layer
```

This gives the agent a mental model of the codebase in three calls.

### Review a PR or branch

Before or during code review:

```
1. analyze_change(base: "main") → changed files with blast radius,
   aggregated must-read context, test gaps, coupling signals
2. Agent identifies untested changes and high-risk modifications
```

### Find files or symbols

```
search(query: "auth", type: "path")     → files matching "auth" in path
search(query: "validateToken", type: "symbol") → files exporting validateToken
```

## Step 4: Multi-file analysis

When modifying multiple related files, analyze them together for deduplicated context:

```
get_context(file: ["src/auth/login.ts", "src/auth/session.ts", "src/auth/types.ts"])
```

The merged result:
- Deduplicates `must_read` across all subjects
- Takes the most conservative (lowest) confidence score
- Reports the highest blast radius level
- Combines test coverage from all subjects

This is more efficient than calling `get_context` three times separately.

## Step 5: Token budget optimization

AI agents work within token budgets. Contextception provides two mechanisms to control output size.

### Workflow modes

The `mode` parameter adjusts output caps for common workflows:

| Mode | Use case | Behavior |
|------|----------|----------|
| `plan` | Task planning, architecture review | Broader context, larger caps |
| `implement` | Writing code | Focused output, smaller caps |
| `review` | Code review | Balanced |

```
get_context(file: "src/core/engine.ts", mode: "implement")
```

### Token budget

Set an explicit token target and contextception scales all caps proportionally:

```
get_context(file: "src/core/engine.ts", token_budget: 4000)
```

### Output cap parameters

For fine-grained control, set individual caps:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `max_must_read` | 10 | Maximum must_read entries |
| `max_related` | 10 | Maximum related entries |
| `max_likely_modify` | 15 | Maximum likely_modify entries |
| `max_tests` | 5 | Maximum test entries |

Entries that exceed the cap overflow to `related` (for must_read) or are dropped with a note.

## Step 6: Optional project configuration

For better results, create `.contextception/config.yaml` in your repo:

```yaml
version: 1
entrypoints:
  - cmd/server/main.go
  - src/index.ts
ignore:
  - vendor
  - node_modules
  - third_party
generated:
  - proto/generated
  - src/__generated__
```

- **entrypoints** — files treated as architectural entry points (boosted in scoring)
- **ignore** — directories excluded from analysis
- **generated** — directories whose files are treated as generated and excluded

## Tips

### Instruct your agent to always use get_context

Add to your project's agent instructions (CLAUDE.md, .cursorrules, etc.):

```
Before modifying any source file, call get_context to understand its dependencies.
Read the must_read files before making changes.
Check blast_radius to assess risk.
```

For a global instruction that applies to all projects, add to `~/.claude/CLAUDE.md`:

```
Use contextception MCP tools in repos with Python, TypeScript/JavaScript, Go, Java, Rust, or C# code.
Before modifying a file, call get_context on it to understand its dependency context.
```

### Enforce with a Claude Code hook (recommended)

Instructions can be ignored. A hook enforces the workflow by reminding the agent every time it tries to edit a code file without checking context first.

The hook dynamically queries `contextception extensions` for supported file types, so it automatically picks up new languages as they're added.

Create `~/.claude/hooks/contextception-remind.sh`:

```bash
#!/bin/bash
# PreToolUse hook for Edit/Write: remind to call get_context first

FILE_PATH=$(echo "$TOOL_INPUT" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(d.get('file_path', ''))
" 2>/dev/null)

[ -z "$FILE_PATH" ] && exit 0

# Get the file extension
EXT=".${FILE_PATH##*.}"
[ "$EXT" = ".$FILE_PATH" ] && exit 0

# Query contextception for supported extensions (cached per shell session)
if [ -z "$_CONTEXTCEPTION_EXTS" ]; then
  _CONTEXTCEPTION_EXTS=$(contextception extensions 2>/dev/null | tr '\n' '|')
fi

# Skip if contextception isn't installed or returned nothing
[ -z "$_CONTEXTCEPTION_EXTS" ] && exit 0

# Check if this file's extension is supported
echo "$_CONTEXTCEPTION_EXTS" | grep -q "$EXT|" || exit 0

echo "CONTEXTCEPTION REMINDER: Before modifying $(basename "$FILE_PATH"), did you call get_context on it? If not, call it now to understand the dependency context before making changes."
```

Make it executable:

```bash
chmod +x ~/.claude/hooks/contextception-remind.sh
```

Then register it in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/contextception-remind.sh"
          }
        ]
      },
      {
        "matcher": "Write",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/contextception-remind.sh"
          }
        ]
      }
    ]
  }
}
```

This fires before every file edit, reminding the agent to check dependencies first. It skips non-code files (markdown, JSON, config) and only triggers for the six supported languages.

### Use get_context for deleted files too

If you are removing a file, call `get_context` first to discover what depends on it. The `likely_modify` section shows files that import the subject and will break.

### Combine with analyze_change for PR safety

After a set of changes on a branch:

```
analyze_change() → identifies test gaps and unexpected coupling
```

This catches cases where a change in one file has ripple effects the agent did not anticipate.

## Tool reference

| Tool | Parameters | Description |
|------|-----------|-------------|
| `get_context` | `file` (required), `mode`, `token_budget`, `omit_external`, `include_signatures`, `max_must_read`, `max_related`, `max_likely_modify`, `max_tests` | Analyze file dependency context |
| `index` | none | Build/update repository index |
| `status` | none | Index diagnostics |
| `search` | `query` (required), `type`, `limit` | Search by path or symbol |
| `get_entrypoints` | `limit` | Entrypoint and foundation files |
| `get_structure` | none | Directory structure and languages |
| `get_archetypes` | `categories` | Representative files per architectural layer |
| `analyze_change` | `base`, `head` | PR/branch impact analysis |
| `rate_context` | `file` (required), `usefulness` (required, 1-5), `useful_files`, `unnecessary_files`, `missing_files`, `modified_files`, `notes` | Rate recommendation quality for feedback tracking |
