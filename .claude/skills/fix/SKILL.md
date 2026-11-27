---
name: fix
description: Fix a kipitrek bug end to end — reproduce it with a failing test, fix the root cause, test for real in the app, report in the PR. Takes a GitHub issue (number or URL) or a description. Use when the user asks to fix a bug or a regression, or invokes /fix <issue or description>.
---

# Fix

Input: a GitHub issue (`gh issue view <n> --comments`) or a description. Five phases, in order.

1. **Understand.** Restate what happens, what should happen and what says so (README, CLAUDE.md), and the code
   involved. Ask only when the expected behavior is genuinely unclear.
2. **Reproduce.** Write the regression test first, next to the existing tests of that code, and see it fail for the
   reason found. Can't reproduce: say what you tried and ask before changing code.
3. **Fix** the root cause once, in the shared code: grep every caller of what you change. Branch `fix/<short-name>`
   from `main`. The test passes, and still fails with the fix reverted. `make cover` and `make lint` green.
4. **Test in real usage** as in the feature skill's phase 3: the reproduction is fixed and the normal path the fix
   touches still works, as the owner and as a friend or a signed-out visitor where visibility is involved.
5. **Report.** PR body: `Fixes #<n>`, a short **What** (the bug and its root cause) and the feature skill's
   `## Testing` table. Final message: the PR link and the table summary.
