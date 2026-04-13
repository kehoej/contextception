Analyze a PR or current branch for risk, then build a plan to fix every issue found.

Use: `/pr-fix` (current branch vs main) or `/pr-fix 123` (PR #123)

## Instructions

### Step 1: Get the diff

If the user provided a PR number:

```
gh pr view <number> --json baseRefName,headRefName,url,title
```

Extract the base and head refs, then run:

```
contextception analyze-change --json <base>..<head>
```

If no PR number was provided (current branch mode):

```
contextception analyze-change --json
```

If contextception is not installed, tell the user to install it and stop.

### Step 2: Present the Risk Assessment

Follow the same format as /pr-risk:

1. One-sentence verdict based on aggregate_risk.score
2. Files that need attention (REVIEW/TEST/CRITICAL tiers only)
3. Safe files (one-line count)
4. Test coverage assessment

But keep this section brief — the plan is the main event.

### Step 3: Build the Fix Plan

This is the core value. Analyze every issue found and create a concrete, ordered plan to resolve them.

For each issue category, generate specific fix tasks:

**Test gaps** (from test_gaps and test_suggestions):
- For each high-risk file without tests, suggest exactly what test file to create and what to test
- Use the risk_factors and coupling data to know what behaviors matter
- Example: "Create handler_test.go — test the new analytics wiring by verifying RecordUsage is called with correct parameters"

**Coupling risks** (from coupling where direction is "mutual"):
- For mutual dependencies, suggest whether to break the cycle or add integration tests
- Example: "server.go and tools.go have a mutual dependency — add a test that exercises the full MCP request path through both files"

**High-fragility files** (from risk_factors containing "fragility"):
- For files with high fragility (many imports, few importers), suggest interface boundaries or reducing imports
- Only suggest this for files scoring in TEST or CRITICAL tiers — don't over-optimize REVIEW files

**Missing coverage for foundation files** (files with many importers):
- If a file has 5+ importers and no direct tests, prioritize testing it
- Example: "history.go has 10 importers — add migration tests to verify schema changes are backward-compatible"

### Step 4: Order the Plan

Sort tasks by impact:
1. Test gaps for CRITICAL/TEST tier files (highest value — prevents regressions)
2. Test gaps for foundation files (high importer count)
3. Coupling fixes (mutual deps, circular deps)
4. Everything else

Number each task. Keep descriptions actionable — "Create X file that tests Y behavior" not "Consider adding tests."

### Step 5: Present the Plan

Format:

```
## Fix Plan (N tasks)

Addressing these will move the PR from [CURRENT LEVEL] to lower risk.

1. **[File]**: [What to do and why]
2. **[File]**: [What to do and why]
...
```

### Step 6: Offer to Execute

After presenting the plan, ask:

"Want me to start working through this plan? I'll tackle them in order and check each fix as I go."

If the user says yes, work through the tasks one at a time:
- Before each task, say what you're about to do
- After each task, run `contextception analyze-change --json` again to verify the score improved
- Report progress: "Task 1/N complete — risk score dropped from X to Y"

### Rules

- Never show raw JSON or risk scores to the user
- Translate all technical factors into plain language
- Keep the assessment section under 15 lines — the plan is the focus
- Each plan task should be specific enough to execute without further research
- Don't suggest fixes for SAFE files
- Don't suggest architectural refactors for REVIEW files — save those for CRITICAL/TEST
- If the PR is all SAFE files, say "This PR looks clean — no fixes needed" and stop
- When in PR mode, note the PR title and URL for context
