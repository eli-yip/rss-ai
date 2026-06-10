# Progress

Current state of the `rss-ai` gateway. Update as milestones land.

## Status: plan-1 complete — gateway passthrough + handler resolution in place

## Done

- [x] Design spec written and approved — `docs/spec/2026-06-09-rss-ai-gateway-design.md`
- [x] Repository scaffolding: docs layout, `AGENTS.md`, git init
- [x] plan-0 dev environment — runnable skeleton (config, mlog, store, server, /healthz, /readyz)
- [x] plan-1 proxy + handler resolution — three-layer resolver (`pkg/handler`), reverse-proxy passthrough (`pkg/proxy`), catch-all gateway (`pkg/gateway`) mounted on the server. `format=json` passthrough handled; "handled" branch is a seam (transparent passthrough) for plan-2.

## Next

- [ ] plan-2: surgical XML title rewrite (etree) + AI client + title cache + singleflight + rate limiter, filling the handled seam

## Notes

- Integration tests run the gateway against a real RSSHub (`compose.test.yaml`,
  RSSHub `/test/*` routes). Gated on `RSS_AI_TEST_UPSTREAM`; `just integration`
  brings the stack up, runs the suite, and tears it down.
