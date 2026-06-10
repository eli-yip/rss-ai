# Lessons — plan-0 Dev Environment

Reflections from standing up the runnable service skeleton (config, mlog, store,
server, CLI wiring). Grouped by theme.

## Dependencies & `go mod`

- All six primary deps resolved cleanly with `@latest`. `echo/v5` landed on a real
  semver tag (`v5.1.1`), not a pseudo-version — no fallback needed.
- **`go mod tidy` removes deps no package imports _yet_.** Because each task adds
  the package that uses a dep, an intermediate `tidy` strips not-yet-referenced
  modules (`go-toml/v2`, `urfave/cli/v3`, gorm drivers, echo). They come back the
  moment the importing file lands and a build/test runs. Don't fight it per-task; a
  final `just lint` (`go mod tidy -diff`) confirms a clean graph at the end.
- testify pulls transitive `go.sum` entries (go-spew, go-difflib, yaml.v3) that
  only materialize when the first `_test.go` compiles — the first `go test` after
  adding a test triggers the download.
- `echo/v5/middleware` pulls `golang.org/x/time/rate`; its `go.sum` entry appears
  only after `tidy` runs with that middleware package imported.

## Echo v5 API differences (vs. the v4-era patterns in the plan)

- **`e.HideBanner` does not exist in v5.1.1.** The banner concept was removed; the
  `Echo` struct has no `HideBanner`/`HidePort` fields. Fix: drop the line — v5 prints
  no banner on its own, and we serve via our own `http.Server` anyway.
- Everything else the plan flagged as risky compiled verbatim on Go 1.26 + echo
  v5.1.1: `*echo.Context`, `echo.UnwrapResponse`, `e.NewContext`, `errors.AsType[T]`,
  and `middleware.Recover`.

## Tooling — dprint covers all markdown

- `dprint.json` globs `**/*.md`, so `just lint`'s `dprint check` gates **every**
  markdown file, including pre-existing docs (AGENTS.md, goal.md, spec, plans) that
  predated the formatter. To get a green lint, the consolidation step ran
  `dprint fmt` repo-wide — cosmetic only (table alignment, `*x*` → `_x_`, blank
  line after `**Files:**`). If you want to keep pre-existing docs untouched, scope
  the dprint `includes` instead; as configured, the formatter owns them all.

## End-to-end wiring verification

- No reachable Postgres was available (local `:5432` is open but password-protected
  with unknown creds — not appropriate to brute-force the user's DB). Followed the
  plan's documented fallback: ran with the placeholder DSN. The binary loaded config
  (dev → colored console logs), logged `starting rss-ai`, then failed at `store.New`
  with the connect error wrapped through our `fmt.Errorf("init store: …")`. That
  failure path confirms the config → logger → store wiring chain. `/healthz` and
  `/readyz` behavior is covered by the passing `pkg/server` unit tests.
- `go vet ./...` exits non-zero with "no packages to vet" when the module has zero
  `.go` files yet — expected during Task 1, not a setup error.
