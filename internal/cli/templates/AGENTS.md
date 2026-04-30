# Contextception — agent instructions

This repository is indexed by **contextception**, a code context intelligence engine that maps each file's dependency context. It is exposed to AI coding agents through an MCP server (or, where MCP is unavailable, the `contextception` CLI).

## What contextception gives you

An MCP server with these tools:

- `get_context(file)` — what to read before safely modifying `file`. Returns `must_read`, `likely_modify`, `tests`, `blast_radius`.
- `analyze_change(base, head)` — diff-level risk: blast radius per file, test gaps, coupling signals, hotspots.
- `get_structure()`, `get_entrypoints()`, `get_archetypes()` — orientation tools for unfamiliar repos.
- `search(query, type)` — find files by path pattern or symbol (`type: "symbol"`).
- `index()`, `status()` — index management. The MCP server auto-indexes; you rarely call these.
- `rate_context(file, usefulness, ...)` — feedback channel that improves accuracy over time.

Supported languages: Python, TypeScript/JavaScript, Go, Java, Rust, C#. Files outside these languages are not in the index and won't return useful results.

---

## Reach for it when

- **Modifying a hub-shaped file** you don't fully understand. Hub-shaped means: imported by ≥3 other files, lives under a load-bearing path (`internal/`, `core/`, `lib/`, `auth/`, `db/`, `migrations/`, `pkg/`, `app/`), or its name suggests it's foundational (`*_base.*`, `client.*`, `store.*`, `schema.*`, `config.*`, `types.*`, `errors.*`).
  → call `get_context` on the file. Read the `must_read` files before editing. Honor `likely_modify` and run the listed `tests`.

- **First touch on an unfamiliar repo.**
  → call `get_structure`, then `get_entrypoints`, then `get_archetypes`. That's typically enough to pick the right entry points without reading dozens of files.

- **About to land or review a branch / PR.**
  → call `analyze_change`. The diff-level view (blast radius per file, test gaps, coupling, hotspots) is more actionable than per-file `get_context` for changesets.

- **"Where is X defined / used?"**
  → call `search` with `type: "symbol"`. Faster and more accurate than grep when the symbol could be in any of several places.

- **Removing or renaming a file or symbol.**
  → call `get_context` on the target first. `likely_modify` lists the dependents that will break.

## Skip it for

- Typo fixes, comment edits, formatting-only changes.
- New files (nothing in the index to map yet).
- Leaf-shaped files: tests, README/markdown, fixtures, generated code, files that import a lot but nothing imports them.
- A file you already pulled context for in this session — reusing the previous result is fine.
- Files outside the supported languages (Python, TS/JS, Go, Java, Rust, C#).
- Single-line config tweaks, dependency bumps, lock-file regenerations.

If you're unsure, ask: *"would the result of `get_context` actually change how I make this edit?"* If the answer is no, skip it.

## After using it

- If `must_read` actually changed how you made the edit, call `rate_context(file, usefulness: 4-5, useful_files: [...])` and name the load-bearing files.
- If contextception suggested files you ignored, call `rate_context(file, usefulness: 1-2, unnecessary_files: [...])`. The feedback closes an accuracy loop — please don't skip it.
- If you needed a file that wasn't suggested, list it in `missing_files`.

## How it stays current

The MCP server auto-indexes on first call and re-indexes incrementally as files change. You don't need to run `index` manually unless `status` reports the index is stale.

If `.contextception/` is not in `.gitignore`, add it.

## CLI fallback (when MCP isn't available)

When the agent does not speak MCP (e.g. GitHub Copilot in VSCode today), use the CLI instead. The output of `--compact` is token-optimized and well-suited for agent consumption.

```
contextception analyze <file> --compact          # equivalent of get_context
contextception analyze-change --compact          # equivalent of analyze_change
contextception search "<query>" --type symbol    # equivalent of search(symbol)
contextception status                            # index health
```
