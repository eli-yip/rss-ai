# Progress

Current state of the `rss-ai` gateway. Update as milestones land.

## Status: plan-3 complete — first specialized handler (Telegram) on the seam

## Done

- [x] Design spec written and approved — `docs/spec/2026-06-09-rss-ai-gateway-design.md`
- [x] Repository scaffolding: docs layout, `AGENTS.md`, git init
- [x] plan-0 dev environment — runnable skeleton (config, mlog, store, server, /healthz, /readyz)
- [x] plan-1 proxy + handler resolution — three-layer resolver (`pkg/handler`), reverse-proxy passthrough (`pkg/proxy`), catch-all gateway (`pkg/gateway`) mounted on the server. `format=json` passthrough handled; "handled" branch is a seam (transparent passthrough) for plan-2.
- [x] plan-2 title rewrite + cache — filled the handled seam. New packages: `pkg/feed` (surgical etree title rewrite, RSS 2.0 only), `pkg/aiclient` (any-llm-go wrapper + fake), `pkg/rewrite` (cache-first + singleflight + rate limit + timeout fallback), `pkg/upstream` (buffered resty fetch). `pkg/store` gained `LookupTitles`/`SaveTitle`; `Handler` gained `ComposePrompt`; config parses timeouts to `time.Duration`. Gateway emits the `request.done` aggregate; `?format=atom` joins json/unmatched as passthrough. Verified against real RSSHub: handled feed = upstream feed with only titles changed.
- [x] plan-3 Telegram handler — first specialized handler on the `Handler` seam. New package `pkg/handler/telegram`: self-registers under `/telegram/channel` via `init()` (blank-imported in `cmd/rss-ai`), flattens the message HTML to text (`x/net/html`), and composes a body-first prompt (synthetic title demoted to a hint, dropped when it's a prefix of the body). `config.example.toml` enables `/telegram/channel`. Establishes the sub-package + blank-import pattern for future specialized handlers.

## Next

- [ ] Atom output rewrite (deferred this release — atom currently passes through).
- [ ] More specialized handlers as needed (pattern set by plan-3; general handler covers the rest).

## Notes

- Integration tests run the gateway against a real RSSHub (`compose.test.yaml`,
  RSSHub `/test/*` routes). Gated on `RSS_AI_TEST_UPSTREAM`; `just integration`
  brings the stack up, runs the suite, and tears it down. Store tests are gated on
  `RSS_AI_TEST_DSN` (a reachable Postgres).
- spec was amended for plan-2 (RSS-2.0-only, AI reads item body, any-llm-go); see
  the plan-2 notes inside the design doc.
