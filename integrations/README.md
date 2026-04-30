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
# Auto-detect every supported editor and configure all of them.
contextception setup

# Or target one explicitly.
contextception setup --editor claude       # Claude Code
contextception setup --editor cursor       # Cursor
contextception setup --editor windsurf     # Windsurf
contextception setup --editor opencode     # opencode
contextception setup --editor vscode       # VSCode (Copilot Chat MCP)
contextception setup --editor warp         # Warp (prints manual UI steps)
```

Use `--dry-run` to preview changes, or `--uninstall` to reverse. The setup command writes:

| Editor | What gets written |
|---|---|
| Claude Code | MCP server in `~/.claude.json`, `/pr-risk` + `/pr-fix` slash commands, silent cleanup of any legacy PreToolUse hook in `~/.claude/settings.json` |
| Cursor | MCP server in `~/.cursor/mcp.json`, `/pr-risk` + `/pr-fix` rules in `~/.cursor/rules/` |
| Windsurf | MCP server in `~/.codeium/windsurf/mcp_config.json`, `/pr-risk` + `/pr-fix` rules in `~/.windsurf/rules/` |
| opencode | MCP server in `~/.config/opencode/opencode.json` (uses opencode's `mcp.<name>` schema) |
| VSCode | MCP server in the platform-specific user `mcp.json` (Copilot Chat schema with `servers.<name>`) |
| Warp | Nothing — Warp registers MCP servers via its app UI. `setup` prints the manual steps. |

### Optional: also write the agent instruction snippet into the current project

Pass `--instructions` to `setup` and it will upsert the contextception block into the right per-editor instruction file at the **current working directory**, using begin/end markers (`<!-- contextception:begin -->` … `<!-- contextception:end -->`) so any user-authored content above or below is preserved character-for-character. Run it twice and the second run is a no-op. Run with `--uninstall --instructions` to strip just the block, leaving your other rules intact.

```bash
cd /path/to/your/project
contextception setup --instructions             # upserts CLAUDE.md / AGENTS.md / .cursor/rules/contextception.mdc / etc.
```

Per-editor target file (relative to the project root):

| Editor | Instruction file |
|---|---|
| Claude Code | `CLAUDE.md` |
| Cursor | `.cursor/rules/contextception.mdc` (with `alwaysApply: true` frontmatter on first write) |
| Windsurf | `.windsurf/rules/contextception.md` |
| GitHub Copilot (VSCode) | `.github/copilot-instructions.md` |
| opencode, Warp | `AGENTS.md` |

When several editors share `AGENTS.md` (opencode + Warp), the file is written once. Existing files are appended to, not overwritten — never run with sudo.

Earlier versions of `setup` also installed a PreToolUse hook for Claude Code that injected dependency context on every edit. It proved too noisy and was removed. Running `setup` against a settings file that still contains the legacy hook entry will silently strip it.

## Agent Instructions

Setting up the MCP server gives the agent the **tools**. To make the agent reach for them at the right time, it also needs a short **instruction snippet** describing when contextception is and isn't worth calling. The canonical snippet lives at [`AGENTS.md`](AGENTS.md) and is identical across tools — only the destination filename changes.

| Tool | Where to drop it | MCP setup |
|---|---|---|
| Claude Code | `CLAUDE.md` (project root) — or `~/.claude/CLAUDE.md` global | [`claude-code/`](claude-code/) |
| Cursor | `.cursor/rules/contextception.mdc` (with `alwaysApply: true` frontmatter) | [`cursor/`](cursor/) |
| Windsurf | `.windsurf/rules/contextception.md` | [`windsurf/`](windsurf/) |
| GitHub Copilot (VSCode) | `.github/copilot-instructions.md` | [`vscode-copilot/`](vscode-copilot/) (Copilot Chat in recent VSCode supports MCP; older builds use the CLI fallback) |
| OpenAI Codex | `AGENTS.md` (project root) | [`codex/`](codex/) |
| opencode | `AGENTS.md` (project root) | [`opencode/`](opencode/) |
| warp | `AGENTS.md` (project root) | [`warp/`](warp/) |

Each tool's subdirectory has a one-page README with the exact placement path and any tool-specific notes. The body of every snippet is the same — copy [`AGENTS.md`](AGENTS.md) verbatim.

## Manual MCP Configuration

If you prefer to configure manually, or need per-project setup:

### Claude Code

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

2. Copy [`AGENTS.md`](AGENTS.md) to your project root as `CLAUDE.md` (or append its contents to your existing `CLAUDE.md`).

3. Restart Claude Code. Contextception tools will appear in the MCP tool list.

---

### Cursor

**Files:** [`cursor/mcp.json`](cursor/mcp.json), [`cursor/README.md`](cursor/README.md)

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

2. Copy [`AGENTS.md`](AGENTS.md) to `.cursor/rules/contextception.mdc`. See [`cursor/README.md`](cursor/README.md) for the required frontmatter.

3. Restart Cursor. The contextception tools will be available to Cursor's AI agent.

---

### Windsurf

**Files:** [`windsurf/mcp_config.json`](windsurf/mcp_config.json), [`windsurf/README.md`](windsurf/README.md)

Windsurf reads MCP configuration from `~/.codeium/windsurf/mcp_config.json`.

**Setup:**

1. Copy or merge the config:

```bash
mkdir -p ~/.codeium/windsurf
cp integrations/windsurf/mcp_config.json ~/.codeium/windsurf/mcp_config.json
```

If you already have MCP servers configured, merge the `contextception` entry into your existing `mcpServers` object.

2. Copy [`AGENTS.md`](AGENTS.md) to `.windsurf/rules/contextception.md`.

3. Restart Windsurf. Contextception tools will be available in Cascade.

---

### OpenAI Codex / opencode / warp / Custom Agents

For OpenAI Codex, opencode, warp, custom agents, or any MCP-compatible client, contextception runs as a stdio-based MCP server.

**Setup:**

```bash
contextception mcp
```

This starts the MCP server on stdin/stdout. Configure your agent to launch this command and communicate via the MCP stdio transport. Drop [`AGENTS.md`](AGENTS.md) at your project root so the agent knows when to use the tools. See the per-tool READMEs ([`codex/`](codex/), [`opencode/`](opencode/), [`warp/`](warp/)) for tool-specific MCP config snippets.

---

### GitHub Copilot in VSCode

**Files:** [`vscode-copilot/README.md`](vscode-copilot/README.md)

Recent VSCode (1.99+) added MCP support to Copilot Chat. `contextception setup --editor vscode` writes the user-level `mcp.json` for those builds. On older Copilot builds without MCP, contextception is consumed via the CLI instead — the instruction snippet stays the same and the agent invokes `contextception analyze <file> --compact` (and other CLI commands) in place of `get_context`. See [`vscode-copilot/README.md`](vscode-copilot/README.md) for details.

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

Contextception supports repositories using: Python, TypeScript/JavaScript, Go, Java, Rust, C#.

## Slash Commands

Two slash commands are included for AI-assisted PR review. These are installed automatically by `contextception setup` for Claude Code.

| Command | File | Description |
|---------|------|-------------|
| `/pr-risk` | [`claude-code/pr-risk.md`](claude-code/pr-risk.md) | Run risk analysis and present a human-friendly review with verdicts, test coverage, and next steps |
| `/pr-fix` | [`claude-code/pr-fix.md`](claude-code/pr-fix.md) | Analyze risk, then build an ordered fix plan for every issue (test gaps, coupling, fragility) |

For Cursor/Windsurf, place the command files in `.cursor/rules/` or `.windsurf/rules/` respectively. For other agents, see [`pr-risk-review.md`](pr-risk-review.md) for the full prompt template.

## Further Reading

- [MCP Tutorial](../docs/mcp-tutorial.md) — step-by-step guide to adding context intelligence to any AI agent
- [Feature Reference](../docs/features.md) — full schema and parameter documentation
- [Configuration](../README.md#configuration) — optional `.contextception/config.yaml` setup
