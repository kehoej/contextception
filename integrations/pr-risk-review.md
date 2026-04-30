# /pr-risk — AI-Powered PR Risk Review

A slash command / skill that runs contextception's `analyze-change` and presents the results as a conversational, actionable review instead of raw data.

Works with: Claude Code, Cursor, Windsurf, Codex, or any LLM-powered editor that supports custom prompts or slash commands.

## Setup

### Claude Code

Add to your project's `.claude/commands/pr-risk.md`:

```markdown
Run a risk analysis on the current branch and present an actionable review.

## Instructions

Run this command to get the raw risk data:

```
contextception analyze-change --json
```

If contextception is not installed, tell the user to install it and stop.

If the command succeeds, parse the JSON output and present a review following this structure:

### 1. One-Sentence Verdict

Start with a single sentence that answers: "Is this PR safe to merge?"

Use plain language based on the aggregate risk score:
- Score 0-20: "This PR is low risk — safe to merge with standard review."
- Score 21-50: "This PR has moderate risk — a few files deserve closer attention."
- Score 51-75: "This PR is high risk — targeted testing recommended before merging."
- Score 76-100: "This PR has critical risk — careful review required, regressions likely without it."

### 2. Files That Need Attention

Only show files in the REVIEW, TEST, or CRITICAL tiers. Skip SAFE files entirely.

For each file worth discussing, explain in plain language:
- **What it does** (infer from the file path and name)
- **Why it's flagged** (translate risk_factors into human language — don't say "fragility 0.83", say "this file imports many packages but few things depend on it, making it sensitive to upstream changes")
- **What to check** (specific advice: "verify the output format didn't change", "check that the new flag is handled in all code paths", etc.)

Group related files together. If 5 CLI handlers all have the same risk profile, summarize them as a group instead of listing each one.

### 3. What's Safe to Skip

One line: "N files are low risk (new files with tests, documentation updates, etc.) — no special attention needed."

### 4. Test Coverage Assessment

If test_coverage_ratio < 0.5, flag it: "Less than half the changed code files have direct tests."
If there are test_gaps, name the most important untested files (not all of them).
If there are test_suggestions, present them as actionable items.

### 5. Offer Next Steps

End with concrete options the user can choose:

- "Want me to look at [highest-risk file] more closely?"
- "Should I check [file with test gap] for missing test coverage?"
- "Want me to review the coupling between [coupled files]?"
- "Should I run the tests to verify nothing broke?"

Only offer options that make sense for the specific results. Don't offer to check test coverage if coverage is already good.

### Formatting Rules

- Never show raw JSON, risk scores, or factor lists to the user
- Never say "risk_score", "risk_tier", "risk_factors", or "fragility" — translate everything
- Keep the entire review under 40 lines
- Use the file's basename (not full path) when it's unambiguous
- If the PR is all SAFE files, keep the review to 3 lines total
```

### Cursor / Windsurf

Add the same content to your `.cursor/rules/pr-risk.md` or `.windsurf/rules/pr-risk.md`. The LLM will pick it up as a custom rule.

Alternatively, paste the instructions section into your editor's custom instructions or system prompt.

### Codex / Other Agents

Add to your agent's system prompt or AGENTS.md:

```
When asked to review a PR for risk, run `contextception analyze-change --json` and follow the pr-risk review format from integrations/pr-risk-review.md.
```

## Example Output

Here's what the LLM would present to a user after running `/pr-risk` on a 28-file PR:

---

**This PR has moderate risk — a few files deserve closer attention.**

**Files to review:**

The MCP server and CLI analysis handlers are the highest-risk area. `tools.go`, `analyze.go`, and `server.go` all have mutual dependencies and circular coupling with each other. They're sensitive to upstream changes since they import many packages. Check that the new analytics wiring doesn't change the output format that existing MCP consumers expect.

`history.go` is a foundation file — 10 other files import it. The changes here are relatively safe (low fragility, good test coverage), but because so many things depend on it, verify the migration schema is backward-compatible.

`root.go`, `analyze_change.go`, and `compact.go` are standard CLI wiring changes — lower risk since they have test coverage.

**19 files are low risk** (new CLI commands with tests, documentation updates) — no special attention needed.

**Test coverage:** 57% of changed code files have direct tests. `analyze.go` and `tools.go` don't have dedicated test files, though they're covered by `cli_test.go` and `server_test.go` respectively.

**Want me to:**
- Look at the coupling between `tools.go` and `server.go` more closely?
- Check `history.go`'s schema migration for backward compatibility?
- Run the test suite to verify nothing broke?

---

## How It Works

The skill is purely a prompt — it tells the LLM how to interpret contextception's JSON output and present it as a human-friendly review. The deterministic analysis (risk scoring, triage, coupling detection) is done by contextception. The LLM's job is translation and presentation.

```
contextception (deterministic) → JSON risk data → LLM (interpretation) → human-friendly review
```

This keeps the separation clean: contextception never hallucinates about code structure, and the LLM never computes risk scores. Each does what it's good at.
