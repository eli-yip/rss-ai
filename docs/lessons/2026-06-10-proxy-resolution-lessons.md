# Proxy + Handler Resolution — Lessons

Reflections from building the catch-all gateway: three-layer handler resolution
(spec §4) and reverse-proxy passthrough (§2/§3). Grouped by theme.

## Echo v5 API

- **`c.Response()` returns `http.ResponseWriter`, `c.Request()` returns
  `*http.Request`** — both can be passed straight to
  `httputil.ReverseProxy.ServeHTTP(c.Response(), c.Request())`. No unwrap needed
  for proxying (unwrap is only for inspecting `.Status`/`.Committed`).
- **Catch-all is `e.Any("/*", h)`.** Static routes (`/healthz`, `/readyz`) take
  precedence over the `/*` wildcard, so mounting the gateway does not shadow the
  health endpoints — confirmed by an integration test. (Echo also has a dedicated
  `RouteNotFound("/*", …)` for true 404 handling, but `Any` is the right fit for a
  transparent catch-all proxy.)
- `e.NewContext(req, rec)` builds a context for in-process handler tests, same as
  plan-0.

## Reverse proxy

- Use `ReverseProxy.Rewrite` (the modern `*httputil.ProxyRequest` hook), not the
  deprecated `Director`. `pr.SetURL(base)` sets scheme/host and **joins** the
  base path ahead of the inbound path; set `pr.Out.Host = base.Host` so routing
  uses the upstream host, not the inbound `Host` header.
- A base URL of `https://host` (no path) leaves the inbound path untouched. The
  only double-slash risk is a base path of exactly `/` (`"/" + "/twitter"` →
  `"//twitter"`); a small path-collapse guard handles it. Single-host RSSHub is
  host-only in practice, so this is defensive.
- Set `ReverseProxy.ErrorHandler` to log and write `502 Bad Gateway`; otherwise a
  dead upstream surfaces as a bare 502 with no log line. `Passthrough` returns
  `nil` because the proxy writes the response directly — don't also write via
  echo.

## Handler resolution (spec §4)

- **Segment-boundary matching is the subtle bit.** `path == prefix ||
  strings.HasPrefix(path, prefix+"/")` — a plain `HasPrefix` would wrongly match
  `/githubfoo` against `/github`. Dedicated test for it.
- **Longest-prefix = sort enabled prefixes by length desc, first match wins.**
  Built once in `NewResolver`, not per request.
- **The registry is queried with the prefix config matched, not the registry's
  own keys.** A specialized handler registered at `/github/issue` is dormant when
  config only enables `/github`. Encoding this as "match enabled prefix first,
  then `lookup` that exact key" makes the rule fall out naturally.
- The registry is process-global and `init()`-populated in production. Tests use
  unique throwaway prefixes (`/__test_*`) to avoid cross-test pollution rather
  than adding a reset hook.

## Testing the plan-1 seam

- plan-1's "handled" branch is transparent passthrough, so it is
  indistinguishable from passthrough by HTTP response alone. Factoring the branch
  decision into an unexported `decide(c) (Resolution, mode)` and testing it via an
  **internal** test (`package gateway`) cleanly asserts the mode without poking at
  the response. Black-box tests then only assert passthrough fidelity.
- An external `gateway_test` package can import `pkg/server` to verify route
  precedence without an import cycle (server does not import gateway).

## Tooling

- `dprint` globs all `*.md`, including new plan/lessons files — `just lint` fails
  on an unformatted markdown table. Run `dprint fmt` after editing docs. (Same
  gotcha noted in plan-0.)
- No reachable Postgres locally, so `go run ./cmd/rss-ai` still dies at
  `store.New` (which runs before proxy/gateway in `main`). Runtime gateway
  behavior is therefore covered by the integration test, not the binary smoke —
  the binary smoke only confirms config → logger wiring, same as plan-0.
