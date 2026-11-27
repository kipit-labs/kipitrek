# kipitrek

All your hikes on one map. Go API + embedded React app, PostgreSQL + PostGIS. Part of kipit-labs; prod runs on
the homelab k3s cluster (ArgoCD). `README.md` has the architecture, data model, API and configuration: keep it current with every change.

## Rules
- Clean architecture, dependencies point inward: `domain` (entities, ports, errors) has no framework imports;
  `usecase` holds the logic; `infrastructure` the adapters (GORM/PostGIS, GPX, SMTP, Open-Meteo, Overpass);
  `delivery/http` the chi handlers and DTOs. Wiring only in `cmd/kipitrek/main.go`. Handlers never touch GORM.
  depguard in `.golangci.yml` enforces it: never `//nolint` it, fix the dependency.
- Interfaces only where the code meets the outside (database, mail, HTTP APIs, files, clock when it matters), declared
  by the side that uses them. No interface for code with one implementation inside the app.
- Tests: use cases with in-memory fakes (`fakes_test.go`), adapters against the real thing (`*_integration_test.go`
  on PostGIS, `httptest` servers for HTTP APIs), handlers with fake services through `call` (`handler/helpers_test.go`).
  A new API route goes into `routeAccess` (`delivery/http/routes_test.go`) with who may call it. A bug fix starts
  with a test that fails. `COVERAGE_FLOOR` in the Makefile only goes up.
- Files stay under 500 lines (revive, oxlint); split by concern, not by size. `web/src/components/ui/` is vendored
  (mapcn, shadcn): edit sparingly, don't restructure.
- Schema is GORM AutoMigrate: additive changes only, no destructive migration without asking.
- Stats derived from the stored GPX are stamped with `hike.DerivedVersion`: bump it when adding or changing one, a
  background job recomputes older hikes.
- A planned hike stays out of stats, records, tiles, summits and repeated routes; a planned GPX keeps the route only
  (no times, no sensor data).
- UI components come from `@kipit-labs/kipitidy` (imported per file: `@kipit-labs/kipitidy/button`), themed by its
  `theme.css`. `web/src/components/ui/` only holds what the kit lacks (map, chart, command, popover…); a component
  every kipit-labs app could use goes into the kit instead.
- Every UI string goes through react-i18next, in both `web/src/locales/en.json` and `fr.json` (French plurals need
  `_many` too); API errors carry a `code` with a translation. Tests enforce both.
- Mobile first: the PWA runs on iPhone (WebKit). No feature that needs a native wrapper.
- Outbound calls are optional: each external service has an env var that turns it off when empty.

## Commands
`make dev-db` / `make dev-api` / `make dev-web` / `make test` / `make test-integration` / `make cover` / `make lint` /
`make build`. `make cover` (unit + integration + floor) is what CI runs.
`go:embed` needs `web/dist` (`web/dist/.gitkeep` is committed). Skills: `feature`, `fix`, `release`.

## Workflow
Conventional commits, one PR per step; multi-step work is a stack (`gh stack`). Releases are a `chore(release): X.Y.Z`
PR bumping `VERSION`; deploys bump the image in the homelab ops repo (see the `release` skill).
