Run a risk analysis on the current branch and present an actionable review.

## Instructions

Run this command to get the raw risk data:

```
contextception analyze-change --json
```

If contextception is not installed or the command fails, tell the user to install it (`brew install kehoej/tap/contextception` or `go install github.com/kehoej/contextception/cmd/contextception@latest`) and stop.

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
- Never say "risk_score", "risk_tier", "risk_factors", or "fragility" — translate everything into plain language
- Keep the entire review under 40 lines
- Use the file's basename (not full path) when it's unambiguous
- If the PR is all SAFE files, keep the review to 3-4 lines total
