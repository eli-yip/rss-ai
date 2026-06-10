# Proxy + Handler Resolution (plan-1) Implementation Plan

> **For agentic workers:** implement task-by-task, test-first. Mark each step
> `[x]` as it lands and keep **Status** current. Append reflections to the
> matching lessons file as you go.

**Spec:** `docs/spec/2026-06-09-rss-ai-gateway-design.md` (read §2, §3, §4, §9, §11.7)
**Lessons:** `docs/lessons/2026-06-10-proxy-resolution-lessons.md` (append during execution)
**Status:** done

## Overview

plan-0 left a runnable skeleton (config, logger, store, Echo server with
`/healthz` + `/readyz`). plan-1 puts the **gateway** in front of it: a catch-all
route that, for every incoming path, decides **handled vs passthrough** and —
for now — reverse-proxies both to the upstream RSSHub instance unchanged.

The core deliverable is the **three-layer handler resolution** of spec §4:

1. **config** — the only on/off switch + allowlist. `config.C.Handlers` maps a
   prefix → `{enabled, prompt}`. Resolution matches a path to the **longest
   enabled** prefix.
2. **registry (in-code)** — specialized handlers self-register from `init()`,
   keyed by prefix. Looked up with the **same** key config matched. plan-1 ships
   the registry mechanism but no production specialized handler yet.
3. **general handler (in-code)** — the fallback used when a prefix is enabled but
   has no specialized handler. Carries the built-in default prompt.

The actual surgical XML title rewrite, AI client, title cache, singleflight, rate
limiter, and the full `request.done` aggregate event are **out of scope** — they
belong to plan-2+. plan-1 leaves a clean seam: the "handled" branch resolves the
handler and records `mode=handled`, then (for now) transparently proxies to
upstream exactly like passthrough. plan-2 fills that seam with fetch → rewrite →
cache → modified XML.

New packages: `pkg/handler` (interface, registry, resolver, general handler),
`pkg/proxy` (reverse-proxy passthrough), `pkg/gateway` (catch-all that ties
resolve → passthrough/handled). Wiring goes through `cmd/rss-ai/main.go` onto the
existing `server.Echo()`.

### Package / file layout

| File                          | Responsibility                                                     |
| ----------------------------- | ------------------------------------------------------------------ |
| `pkg/handler/handler.go`      | `Handler` interface; the resolution seam.                          |
| `pkg/handler/registry.go`     | Global registry; `Register` / internal `lookup`.                   |
| `pkg/handler/general.go`      | `generalHandler` fallback + built-in default prompt.               |
| `pkg/handler/resolver.go`     | `Resolver`, `Resolution`, longest-prefix match, effective prompt.  |
| `pkg/handler/*_test.go`       | Registry + resolver tests (the bulk of plan-1's tests).            |
| `pkg/proxy/proxy.go`          | `Proxy` over `httputil.ReverseProxy`; `Passthrough(c)`.            |
| `pkg/proxy/proxy_test.go`     | Passthrough fidelity + upstream-error (502) tests.                 |
| `pkg/gateway/gateway.go`      | `Gateway.Handle(c)`: resolve → passthrough or handled-seam.        |
| `pkg/gateway/gateway_test.go` | End-to-end resolve/passthrough/json behavior with a fake upstream. |
| `cmd/rss-ai/main.go` (modify) | Build resolver + proxy + gateway; mount catch-all on `srv.Echo()`. |

Tests live beside each package (`*_test.go`).

---

## Steps

### 1. Handler interface + registry — [x]

- **Goal:** A process-global registry that specialized handlers self-register
  into from `init()`, keyed by prefix, with a typed `Handler` interface. plan-1
  needs only the resolution seam, so the interface exposes the built-in default
  `Prompt()` (config prompt overrides it later per §4); the rewrite method is
  added in plan-2.
- **Changes:** Create `pkg/handler/handler.go`, `pkg/handler/registry.go`.
  - Interface shape:
    ```go
    // Handler is specialized per-prefix logic. plan-1 only needs the prompt
    // seam; plan-2 adds the title-rewrite method.
    type Handler interface {
        Prompt() string // built-in default prompt; config prompt overrides it
    }
    ```
  - Registry shape:
    ```go
    var registry = map[string]Handler{}

    // Register binds a specialized handler to an exact prefix key. Called from
    // init(). Panics on empty prefix or duplicate registration (programming
    // error, caught at startup).
    func Register(prefix string, h Handler)

    // lookup is unexported; the resolver queries it with the matched prefix key.
    func lookup(prefix string) (Handler, bool)
    ```
- **Tests:** `pkg/handler/registry_test.go` (internal `package handler` so it can
  call `lookup`): `Register` then `lookup` returns the handler; missing prefix
  returns `(nil,false)`; duplicate `Register` of the same prefix panics; empty
  prefix panics. Use unique prefixes (e.g. `/__test_a`) — the registry is
  process-global, so tests must not collide.
- **Done when:** `go test ./pkg/handler/ -run TestRegistry -v` passes; the
  registry round-trips and rejects dup/empty.

### 2. General (fallback) handler + default prompt — [x]

- **Goal:** The fallback handler used when a prefix is enabled but has no
  specialized handler. Holds the built-in default rewrite prompt (§4 prompt
  priority).
- **Changes:** Create `pkg/handler/general.go`.
  ```go
  // defaultPrompt is the built-in rewrite instruction used when neither config
  // nor a specialized handler supplies one.
  const defaultPrompt = "Rewrite this feed item title to be clear and readable, preserving meaning."

  type generalHandler struct{}

  func (generalHandler) Prompt() string { return defaultPrompt }
  ```
  The general handler is **not** registered (it is the fallback, not a
  prefix-keyed entry); the resolver holds one instance.
- **Tests:** `pkg/handler/general_test.go`: `generalHandler{}.Prompt()` equals
  `defaultPrompt` and is non-empty.
- **Done when:** `go test ./pkg/handler/ -run TestGeneral -v` passes.

### 3. Resolver — longest-prefix match + three-layer resolution — [x]

- **Goal:** The heart of plan-1. Given a request path, decide: matched an enabled
  prefix or not; if matched, which prefix key, whether a specialized handler
  exists, and the effective prompt. Implements spec §3/§4 exactly.
- **Changes:** Create `pkg/handler/resolver.go`.
  ```go
  type Resolution struct {
      Matched     bool                 // matched an enabled prefix
      Prefix      string               // the matched prefix key ("" if none)
      Specialized bool                 // a registry handler exists for Prefix
      Handler     Handler              // specialized if Specialized, else general
      Config      config.HandlerConfig // the matched prefix's config entry
  }

  // EffectivePrompt applies §4 priority: config prompt > handler built-in.
  func (r Resolution) EffectivePrompt() string {
      if r.Config.Prompt != "" {
          return r.Config.Prompt
      }
      if r.Handler != nil {
          return r.Handler.Prompt()
      }
      return ""
  }

  type Resolver struct {
      prefixes []string // enabled prefixes, sorted longest-first
      general  Handler
  }

  // NewResolver captures the enabled prefixes from config (Enabled==true only)
  // and sorts them so the longest match wins.
  func NewResolver(handlers map[string]config.HandlerConfig) *Resolver

  func (r *Resolver) Resolve(path string) Resolution
  ```
  Resolution algorithm:
  1. Walk enabled prefixes longest-first; the first that matches on a **segment
     boundary** wins. Boundary match: `path == prefix || strings.HasPrefix(path,
     prefix+"/")` — so `/github` matches `/github` and `/github/issue/123` but
     **not** `/githubfoo`.
  2. No match → `Resolution{Matched:false}`.
  3. Match → look the matched prefix key up in the registry (`lookup`). Present →
     `Specialized:true, Handler:specialized`. Absent → `Specialized:false,
     Handler:general`. Either way carry `Config:handlers[prefix]`.
- **Tests:** `pkg/handler/resolver_test.go` — the spec §13 resolution matrix:
  - Longest prefix: config has `/github` and `/github/issue` both enabled →
    `/github/issue/123` resolves to `/github/issue`; `/github/pulls/1` resolves
    to `/github`.
  - Segment boundary: `/githubfoo` does **not** match enabled `/github`
    (unmatched → passthrough).
  - Exact match: path exactly `/github` matches enabled `/github`.
  - Disabled ignored: a prefix present in config but `enabled=false` is never
    matched.
  - Unmatched: an unregistered path → `Matched:false`.
  - Specialized vs general: register a specialized handler at the matched key →
    `Specialized:true`; without one → `Specialized:false` and `Handler` is the
    general handler.
  - Same-key rule: specialized handler registered at `/github/issue` but config
    only enables `/github` → `/github/issue/1` matches `/github`, lookup uses
    `/github`, so `Specialized:false` (the dormant handler is not used).
  - Effective prompt priority: config prompt non-empty → wins; config prompt
    empty + general handler → `defaultPrompt`; config prompt empty + specialized
    handler → specialized's `Prompt()`.
- **Done when:** `go test ./pkg/handler/ -v` is green across the full matrix.

### 4. Reverse-proxy passthrough — [x]

- **Goal:** Transparently reverse-proxy a request to `Upstream.BaseURL`,
  preserving method, path, raw query, request/response headers, status code, and
  body byte-for-byte. No caching, no body parsing.
- **Changes:** Create `pkg/proxy/proxy.go`.
  ```go
  type Proxy struct {
      rp     *httputil.ReverseProxy
      logger *mlog.Logger
  }

  // New builds a passthrough proxy for the upstream base URL. Returns an error
  // if baseURL is unparseable.
  func New(baseURL string, logger *mlog.Logger) (*Proxy, error)

  // Passthrough forwards c's request to upstream and streams the response back
  // unchanged. Used for unregistered paths and explicit ?format=json.
  func (p *Proxy) Passthrough(c *echo.Context) error
  ```
  - Build with `httputil.NewSingleHostReverseProxy(base)` (or a `Rewrite` func):
    set target scheme + host from `baseURL`, keep the incoming path and raw
    query untouched. If `baseURL` carries a path, join it as a prefix
    (single-host instance → typically host-only; support a base path defensively).
  - Set `rp.ErrorHandler` to log and write `502 Bad Gateway` when upstream is
    unreachable (so a dead RSSHub does not 500 the gateway).
  - `Passthrough` calls `p.rp.ServeHTTP(c.Response(), c.Request())` and returns
    `nil` (the proxy writes the response directly).
- **Tests:** `pkg/proxy/proxy_test.go` with an `httptest.Server` as fake upstream:
  - Fidelity: upstream echoes method/path/query/a custom header and returns a set
    status + body; assert the gateway response reproduces status, body, the
    upstream response header, and that upstream saw the original path + query.
  - Error path: point the proxy at a closed/invalid address → response is `502`.
- **Done when:** `go test ./pkg/proxy/ -v` passes both.

### 5. Gateway catch-all — resolve → passthrough / handled-seam — [x]

- **Goal:** Tie resolver + proxy into one `echo.HandlerFunc` that implements the
  §3 decision flow. plan-1: passthrough and handled both proxy to upstream; the
  difference is only the resolved `mode` (the seam plan-2 fills).
- **Changes:** Create `pkg/gateway/gateway.go`.
  ```go
  type Gateway struct {
      resolver *handler.Resolver
      proxy    *proxy.Proxy
      logger   *mlog.Logger
  }

  func New(resolver *handler.Resolver, proxy *proxy.Proxy, logger *mlog.Logger) *Gateway

  func (g *Gateway) Handle(c *echo.Context) error
  ```
  `Handle` logic:
  1. `res := g.resolver.Resolve(c.Request().URL.Path)`.
  2. `!res.Matched` → `mode=passthrough`, `return g.proxy.Passthrough(c)`.
  3. `c.QueryParam("format") == "json"` → `mode=passthrough` (json), proxy. (§3:
     explicit JSON Feed is never modified, even on an enabled prefix.)
  4. Otherwise (matched, XML) → `mode=handled`. **plan-1 seam:** log the resolved
     handler/prefix and, for now, `return g.proxy.Passthrough(c)`. A `// TODO
     plan-2: fetch upstream → rewrite titles via res.Handler → cache → return
     modified XML` marks where the rewrite goes.
  - Emit one debug/info line per request carrying `path`, `handler`(=prefix),
    `mode`, `specialized`. The full `request.done` aggregate (§11.4) is plan-2+;
    keep this light so it can be subsumed later.
- **Tests:** `pkg/gateway/gateway_test.go` with a fake upstream and a resolver
  built from a small config map:
  - Unregistered path → reaches upstream, response passed through (mode
    passthrough). Assert upstream received the path.
  - Enabled prefix, default format → handled branch still reaches upstream
    (plan-1 passthrough); assert it was resolved as handled (e.g. via an injected
    test logger/observer or by exposing the resolution — see Open questions).
  - Enabled prefix + `?format=json` → passthrough (assert upstream saw
    `format=json` and body is unchanged).
- **Done when:** `go test ./pkg/gateway/ -v` passes; all three branches behave
  per §3.

### 6. Wire the catch-all into the server + route precedence — [x]

- **Goal:** The running binary serves the gateway for all non-health paths while
  `/healthz` and `/readyz` keep working.
- **Changes:** Modify `cmd/rss-ai/main.go`:
  - After `server.New(...)`, build the gateway from config:
    ```go
    resolver := handler.NewResolver(cfg.Handlers)
    px, err := proxy.New(cfg.Upstream.BaseURL, logger)
    // ... handle err
    gw := gateway.New(resolver, px, logger)
    srv.Echo().Any("/*", gw.Handle) // catch-all; static /healthz,/readyz win
    ```
  - Confirm the Echo v5 wildcard route syntax (`Any("/*", …)` / `c.Param("*")`);
    adjust if v5 differs and record it in lessons.
  - Keep `server.New`'s signature unchanged; mount via the public `Echo()`.
    (Alternative considered in Open questions: a `server.Mount` method.)
- **Tests:** `pkg/server` tests already cover health routes. Add a precedence
  check (in `cmd` or via a small gateway/server integration test) that with the
  catch-all mounted, `GET /healthz` still returns 200 and a non-health path is
  routed to the gateway. Then build + manual smoke:
  ```bash
  go build ./...
  go run ./cmd/rss-ai -c config.toml &   # config points upstream at a reachable RSSHub (or a local stub)
  sleep 2
  curl -fsS localhost:8080/healthz                 # {"message":"ok"}
  curl -fsS -o /dev/null -w '%{http_code}\n' localhost:8080/some/unregistered/path  # proxied → upstream status (or 502 if upstream down)
  kill %1
  ```
- **Done when:** `go build ./...` is clean; health endpoints unaffected;
  unregistered paths reach the proxy. If no reachable upstream is handy, a `502`
  with a logged upstream error still confirms wiring (note it in lessons).

### 7. Lint pass + progress update + lessons consolidation — [x]

- **Goal:** Green tree and updated docs.
- **Changes:** `docs/PROGRESS.md` (mark plan-1 done, point to plan-2),
  `docs/lessons/2026-06-10-proxy-resolution-lessons.md` (consolidate).
- **Tests / commands:**
  - `just lint` (or `go vet ./...` + `dprint check` + `go mod tidy -diff` if tools
    missing — note any gap in lessons).
  - `go test ./...` → PASS (store test SKIPs without `RSS_AI_TEST_DSN`).
- **Done when:** lint clean, full suite green, `PROGRESS.md` reflects plan-1
  complete, lessons consolidated per `docs/lessons/README.md`.

> Commit per task using Conventional Commits, e.g.
> `feat(handler): registry + longest-prefix resolver`,
> `feat(proxy): reverse-proxy passthrough to upstream`,
> `feat(gateway): catch-all resolve/passthrough wiring`,
> `docs: mark plan-1 complete and consolidate lessons`.

---

## Open questions

1. **Mounting the catch-all.** Mount via the public `srv.Echo().Any("/*", …)` in
   main (proposed — zero change to the `server` package), or add an explicit
   `server.Mount(gw)` method for intent/encapsulation? Recommend the former for
   plan-1; revisit if the server grows more routes.
2. **Echo v5 wildcard syntax.** Confirm v5 uses `Any("/*", …)` + `c.Param("*")`
   and that static routes (`/healthz`) still take precedence over the wildcard.
   Likely fine (router prioritizes static), but verify and note in lessons —
   plan-0 already found v5 dropped `HideBanner`.
3. **Asserting "handled" in the gateway test.** Since plan-1's handled branch is
   transparent passthrough, a black-box test can't distinguish handled from
   passthrough by the HTTP response alone. Options: (a) inject a test observer /
   capture the resolution-mode log; (b) expose a thin seam (e.g. a package-level
   hook or have `Handle` return/record the `Resolution`) so tests can assert the
   chosen mode; (c) test the resolver directly (step 3) and treat the gateway
   test as passthrough-fidelity only. Recommend (c) + a light (a) for the json
   vs handled distinction.
4. **Upstream base URL shape.** Spec uses a host-only base (`https://rsshub.example.com`).
   Should the proxy support a base URL with a path prefix (joined ahead of the
   request path)? Recommend supporting it defensively but documenting host-only
   as the expected case.
5. **`mode` logging now vs deferring to `request.done`.** plan-1 emits a light
   per-request line (`path/handler/mode/specialized`); §11.4's full aggregate
   (`item_count`, `cache_hits`, …) lands in plan-2. Confirm we don't want the
   full event shape stubbed now. Recommend light-now, full-later.
6. **Registry global state in tests.** The registry is process-global and
   `init()`-populated in production. plan-1 tests register under unique throwaway
   prefixes to avoid cross-test pollution. Is a test-only `reset()` worth adding,
   or are unique prefixes enough? Recommend unique prefixes for now.
7. **Trailing slash.** Should `/github/` (path with a trailing slash) match
   enabled `/github`? The boundary rule (`prefix+"/"`) makes `/github/` match.
   Confirm that is the intended behavior (it matches RSSHub path conventions).
