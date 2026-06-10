# Progress

Current state of the `rss-ai` gateway. Update as milestones land.

## Status: plan-2 complete — handled branch rewrites titles end to end

## Done

- [x] Design spec written and approved — `docs/spec/2026-06-09-rss-ai-gateway-design.md`
- [x] Repository scaffolding: docs layout, `AGENTS.md`, git init
- [x] plan-0 dev environment — runnable skeleton (config, mlog, store, server, /healthz, /readyz)
- [x] plan-1 proxy + handler resolution — three-layer resolver (`pkg/handler`), reverse-proxy passthrough (`pkg/proxy`), catch-all gateway (`pkg/gateway`) mounted on the server. `format=json` passthrough handled; "handled" branch is a seam (transparent passthrough) for plan-2.
- [x] plan-2 title rewrite + cache — filled the handled seam. New packages: `pkg/feed` (surgical etree title rewrite, RSS 2.0 only), `pkg/aiclient` (any-llm-go wrapper + fake), `pkg/rewrite` (cache-first + singleflight + rate limit + timeout fallback), `pkg/upstream` (buffered resty fetch). `pkg/store` gained `LookupTitles`/`SaveTitle`; `Handler` gained `ComposePrompt`; config parses timeouts to `time.Duration`. Gateway emits the `request.done` aggregate; `?format=atom` joins json/unmatched as passthrough. Verified against real RSSHub: handled feed = upstream feed with only titles changed.

## Next

- [ ] Atom output rewrite (deferred this release — atom currently passes through).
- [ ] Production specialized handlers (the `Handler`/`ComposePrompt` seam is ready; general handler covers everything for now).

## Notes

- Integration tests run the gateway against a real RSSHub (`compose.test.yaml`,
  RSSHub `/test/*` routes). Gated on `RSS_AI_TEST_UPSTREAM`; `just integration`
  brings the stack up, runs the suite, and tears it down. Store tests are gated on
  `RSS_AI_TEST_DSN` (a reachable Postgres).
- spec was amended for plan-2 (RSS-2.0-only, AI reads item body, any-llm-go); see
  the plan-2 notes inside the design doc.
