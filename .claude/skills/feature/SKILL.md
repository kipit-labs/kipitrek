---
name: feature
description: Build a kipitrek feature end to end — explore, plan and ask, implement, test for real in the app, report coverage in the PR. Use when the user asks for a new feature or invokes /feature <description>.
---

# Feature

Four phases, in order. Never skip the questions or the real-usage tests.

## 1. Explore and plan

- Read `README.md` and the code the feature touches: domain, usecase, infrastructure, handlers, the web feature folder,
  both locale catalogs.
- Write a plan: what changes where, the PR split (multi-step work = a `gh stack`, one `feat/...` branch per step),
  and the real-usage test list for phase 3.
- **Ask before implementing.** List every open question, ambiguity and contradiction (with CLAUDE.md rules, existing
  behavior, or the request itself) with AskUserQuestion, wait for the answers, then start. A plan with zero questions
  is suspicious: say why nothing is unclear.

## 2. Implement

Follow CLAUDE.md. Tests for new logic (Go next to the code, `*_integration_test.go` for SQL; vitest in `web/`),
both locales, README (API table, data, configuration) when behavior changes. New routes go into `routeAccess`.
`make cover` and `make lint` green before phase 3.

## 3. Test in real usage

- `make dev-db && make web && make dev-api`, then drive http://localhost:8080 through the API and a real browser
  (playwright-core with the cached Chromium), at phone width (390×844) too. Emails land in Mailpit (:8025).
- Use a test account (`claude-test@test.local`, verified through psql); never sessions of real users.
- Another project's database may hold :5432: set `DB_PORT` in `.env` rather than stopping it.

For each case record what you did, expected and actual. Failures are fixed and retested, not just reported.

## 4. Report

In the PR description (the last PR of a stack), under `## Testing`:

```markdown
## Testing

`make cover` / `make lint`: green (coverage X%).

| # | Case | How | Expected | Result |
|---|------|-----|----------|--------|
| 1 | Import a planned GPX | POST /api/hikes?planned=true | no times kept | ✅ |

Not covered: <case> — <why>.
```

✅ works, ❌ broken, ⚠️ works with a caveat. In the final message, repeat only the counts, the ⚠️/❌ rows and the PR link.
