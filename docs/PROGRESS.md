# Progress

Current state of the `rss-ai` gateway. Update as milestones land.

## Status: deployed to production (linkerlab-us-2) — release 26.6.0 live

## Done

- [x] Design spec written and approved — `docs/spec/2026-06-09-rss-ai-gateway-design.md`
- [x] Repository scaffolding: docs layout, `AGENTS.md`, git init
- [x] plan-0 dev environment — runnable skeleton (config, mlog, store, server, /healthz, /readyz)
- [x] plan-1 proxy + handler resolution — three-layer resolver (`pkg/handler`), reverse-proxy passthrough (`pkg/proxy`), catch-all gateway (`pkg/gateway`) mounted on the server. `format=json` passthrough handled; "handled" branch is a seam (transparent passthrough) for plan-2.
- [x] plan-2 title rewrite + cache — filled the handled seam. New packages: `pkg/feed` (surgical etree title rewrite, RSS 2.0 only), `pkg/aiclient` (any-llm-go wrapper + fake), `pkg/rewrite` (cache-first + singleflight + rate limit + timeout fallback), `pkg/upstream` (buffered resty fetch). `pkg/store` gained `LookupTitles`/`SaveTitle`; `Handler` gained `ComposePrompt`; config parses timeouts to `time.Duration`. Gateway emits the `request.done` aggregate; `?format=atom` joins json/unmatched as passthrough. Verified against real RSSHub: handled feed = upstream feed with only titles changed.
- [x] plan-3 Telegram handler — first specialized handler on the `Handler` seam. New package `pkg/handler/telegram`: self-registers under `/telegram/channel` via `init()` (blank-imported in `cmd/rss-ai`), flattens the message HTML to text (`x/net/html`), and composes a body-first prompt (synthetic title demoted to a hint, dropped when it's a prefix of the body). `config.example.toml` enables `/telegram/channel`. Establishes the sub-package + blank-import pattern for future specialized handlers.
- [x] Packaging — `docker/Dockerfile.goreleaser` (alpine + tzdata, copies the prebuilt binary) and `scripts/build-docker.sh` (cross-compiles `linux/{amd64,arm64}`, builds + optionally pushes `eliyip/rss-ai:{TAG}` and `:latest`), wired as `just build-docker`. Mirrors the rss-zero workflow.
- [x] Release 26.6.0 (CalVer) — tagged on `master`; image `eliyip/rss-ai:26.6.0` + `:latest` pushed to Docker Hub.
- [x] Production deploy on `linkerlab-us-2` — added as a service in `~/services/rsshub/compose.yaml` (internal-only, no Traefik route yet). `config.toml` mounted; upstream `http://rsshub:1200`, DB `rss_ai` on shared `onedb`. Verified: `/healthz`, `/readyz`, byte-transparent passthrough (`/test/1`), and live Telegram title rewrite (`/telegram/channel/durov`).
- [x] Log shipping — rss-ai stdout (JSON) scraped by Alloy via the Docker socket and pushed to Loki under `{app="rss-ai"}`. Added a `discovery.docker` → `discovery.relabel` → `loki.source.docker` pipeline to `~/services/loki/config-alloy.alloy` (no change to rss-ai's compose).

## Next

- [ ] Grafana dashboard from `request.done` events (spec §11.6): cache-hit rate, AI call/error rate, p50/p95/p99 latency, timeout-fallback rate, handled-vs-passthrough mix, upstream latency/errors.
- [ ] Expose rss-ai externally when ready (own host `rss-ai.darkeli.com`, or take over `rsshub.darkeli.com`).
- [ ] Image-only Telegram posts rewrite to a garbled title (e.g. `Image</Image>`) — fix the body-first handler's empty-body edge case in `pkg/handler/telegram`.
- [ ] Atom output rewrite (deferred this release — atom currently passes through).
- [ ] More specialized handlers as needed (pattern set by plan-3; general handler covers the rest).

## Notes

- Integration tests run the gateway against a real RSSHub (`compose.test.yaml`,
  RSSHub `/test/*` routes). Gated on `RSS_AI_TEST_UPSTREAM`; `just integration`
  brings the stack up, runs the suite, and tears it down. Store tests are gated on
  `RSS_AI_TEST_DSN` (a reachable Postgres).
- spec was amended for plan-2 (RSS-2.0-only, AI reads item body, any-llm-go); see
  the plan-2 notes inside the design doc.
- Deploy topology (`linkerlab-us-2`): rss-ai shares the external `traefik` Docker
  network with `rsshub` (upstream), `onedb` (Postgres, DB `rss_ai`), and `loki`.
  Files live in `~/services/rsshub/` (compose + `config.toml`); the Alloy log
  pipeline lives in `~/services/loki/config-alloy.alloy`. No git remote on the
  dev box — `master` and tag `26.6.0` are local only.
