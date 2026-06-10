# Title Rewrite + Cache (plan-2) Implementation Plan

> **For agentic workers:** implement task-by-task, test-first. Mark each step
> `[x]` as it lands and keep **Status** current. Append reflections to the
> matching lessons file as you go.

**Spec:** `docs/spec/2026-06-09-rss-ai-gateway-design.md` (read §3, §5, §6, §7, §8, §11.4, §13)
**Lessons:** `docs/lessons/2026-06-10-rewrite-cache-lessons.md` (append during execution)
**Status:** in progress

## Overview

plan-1 left a clean seam: the gateway resolves every request to **handled vs
passthrough** and, for `modeHandled`, transparently proxies to upstream with a
`// TODO plan-2` marker. plan-2 **fills that seam**: for an enabled prefix on the
default RSS 2.0 output, the gateway buffers the upstream feed, surgically rewrites
each `<item>/<title>` with an LLM (reading the item body for context), caches the
title mapping in Postgres, and returns the modified XML with every other byte
preserved. Passthrough (unregistered paths, `?format=json`, **and now
`?format=atom`** — see Spec deviations) is unchanged from plan-1.

The work splits into one **fidelity** core, one **concurrency** core, and the
plumbing that connects them:

- **Fidelity** (`pkg/feed`, spec §5.1): load the whole tree with `beevik/etree`,
  extract each item's `(id, title, body)`, replace only the title text, serialize
  back. enclosure / media / namespaces / category / author / pubDate / guid stay
  byte-identical. The real-RSSHub integration test ("only `<title>` bytes change")
  is the strongest assertion of this.
- **Concurrency** (`pkg/rewrite`, spec §7): cache-first per item; on a miss, one
  AI call per `(handler, id)` via `singleflight.DoChan`; an `x/time/rate` limiter
  on the AI path; and the **timeout-vs-cancel** rule — the AI call runs on a
  detached, longer-lived context (background + AI request timeout, same
  `trace_id`), while the request side waits only `wait_timeout` then falls back to
  the source title and lets the background goroutine finish and write the row.
- **Plumbing**: `pkg/aiclient` (any-llm-go wrapper + a fake for tests),
  `pkg/upstream` (buffered fetch), new `pkg/store` query/write methods, config
  Duration parsing, and the gateway handled branch + the `request.done` aggregate
  event (§11.4).

Only the **general** handler is wired end-to-end. Specialized handlers stay a
seam: plan-2 adds the rewrite-input method to the `Handler` interface, the general
handler implements it, and specialized handlers may override it later (spec §4).

### Package / file layout

| File                                              | Responsibility                                                                       |
| ------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `pkg/config/config.go` (modify)                   | Parse `AI.RequestTimeout` / `Gateway.WaitTimeout` strings → `time.Duration` at load. |
| `pkg/store/titles.go` (new)                       | `LookupTitles` (batch read) + `SaveTitle` (app-level update-or-insert).              |
| `pkg/store/titles_test.go` (new)                  | Env-gated (`RSS_AI_TEST_DSN`) round-trip + update-or-insert tests.                   |
| `pkg/feed/feed.go` (new)                          | etree parse, item `(id,title,body)` extraction, surgical title set, serialize.       |
| `pkg/feed/feed_test.go` (new)                     | Fidelity matrix: namespaces/enclosure/CDATA preserved, guid→link fallback.           |
| `pkg/aiclient/aiclient.go` (new)                  | `Client` interface + any-llm-go OpenAI provider impl from `config.AI`.               |
| `pkg/aiclient/fake.go` (new)                      | `FakeClient` test double (scriptable / blocking) used by rewrite + integration.      |
| `pkg/handler/handler.go` (modify)                 | Add `ComposePrompt(feed.Item) string` to the `Handler` interface.                    |
| `pkg/handler/general.go` (modify)                 | General handler `ComposePrompt`: source title + body as AI user content.             |
| `pkg/rewrite/rewrite.go` (new)                    | `Rewriter`: cache-first + singleflight + rate limit + timeout fallback.              |
| `pkg/rewrite/rewrite_test.go` (new)               | Cache hit/miss/changed, dedup, timeout fallback + background write, AI fail.         |
| `pkg/upstream/upstream.go` (new)                  | `Fetcher.Fetch`: buffered GET preserving path + raw query.                           |
| `pkg/upstream/upstream_test.go` (new)             | Fetch fidelity (path/query/status/body) against an `httptest` upstream.              |
| `pkg/gateway/gateway.go` (modify)                 | Handled branch: fetch → rewrite → write; `request.done` aggregate; atom passthrough. |
| `pkg/gateway/rsshub_integration_test.go` (modify) | Add a handled-branch case: only `<title>` bytes differ from upstream.                |
| `cmd/rss-ai/main.go` (modify)                     | Build aiclient + fetcher + rewriter; inject into `gateway.New`.                      |

Tests live beside each package (`*_test.go`).

---

## Steps

### 1. Parse config durations at load — [x]

- **Goal:** `AI.RequestTimeout` and `Gateway.WaitTimeout` (TOML strings like
  `"30s"`) become typed `time.Duration` available to consumers, validated once at
  startup. `time.Duration` has no `TextUnmarshaler`, so go-toml cannot decode the
  strings directly — parse them ourselves after unmarshal.
- **Changes:** `pkg/config/config.go`. After `toml.Unmarshal`, parse both strings
  with `time.ParseDuration` and store the result on exported sibling fields
  (`AI.RequestTimeoutDur`, `Gateway.WaitTimeoutDur` — `toml:"-"`), returning a
  wrapped error on a bad value so startup fails fast. Keep the string fields as
  the on-disk source of truth. Default config already yields `"30s"` / `"10s"`, so
  defaults parse cleanly.
- **Tests:** `pkg/config/config_test.go`: a valid config exposes
  `RequestTimeoutDur == 30*time.Second` and `WaitTimeoutDur == 10*time.Second`; an
  invalid duration string (`request_timeout = "nope"`) makes `Init` return an
  error.
- **Done when:** `go test ./pkg/config/ -v` passes; consumers read Durations, never
  re-parse strings.

### 2. Store query/write methods — [x]

- **Goal:** `pkg/store` gains the two methods the rewrite pipeline needs (spec
  §5.3/§6): a batch lookup and an application-level update-or-insert. **No DB-level
  upsert/conflict clause** — singleflight serializes writers per `(id, handler)`.
- **Changes:** `pkg/store/titles.go`.
  ```go
  // LookupTitles returns the cached rows for ids under handler, keyed by id.
  // Missing ids are simply absent from the map (one query, id IN (...)).
  func (s *Store) LookupTitles(ctx context.Context, handler string, ids []string) (map[string]ItemTitle, error)

  // SaveTitle inserts or updates the cached title for (id, handler) at the
  // application level: First by primary key, then Updates (source_title,
  // rewritten_title, updated_at) if present, else Create. Serialized per key by
  // singleflight, so no in-process race.
  func (s *Store) SaveTitle(ctx context.Context, it ItemTitle) error
  ```
  Use `WithContext(ctx)`. Empty `ids` short-circuits to an empty map (no query).
- **Tests:** `pkg/store/titles_test.go`, env-gated on `RSS_AI_TEST_DSN` exactly
  like the existing store test (skip when unset). Each test uses a unique
  `handler` value to stay isolated:
  - `SaveTitle` then `LookupTitles([id])` returns the row.
  - `SaveTitle` twice with a changed `rewritten_title`/`source_title` updates in
    place (still one row; `updated_at` advances).
  - `LookupTitles` of mixed present/absent ids returns only the present ones.
  - Empty `ids` returns an empty map without error.
- **Done when:** `go test ./pkg/store/ -v` passes with a DSN and SKIPs cleanly
  without one.

### 3. `pkg/feed` — surgical extraction + title rewrite (fidelity core) — [x]

- **Goal:** The etree layer. Parse raw RSS 2.0 bytes, expose each `<item>` as a
  `feed.Item{ID, Title, Body}`, let the caller set a new title on a specific item,
  and serialize back with **only** the touched titles changed. This is the spec
  §5.1 "surgical replacement" and §5.2 item id.
- **Changes:** `pkg/feed/feed.go`.
  ```go
  // Item is the handler-agnostic extracted view of one RSS <item>.
  type Item struct {
      ID    string // <guid> → <link> fallback (spec §5.2; no Atom this release)
      Title string // current upstream <title> text
      Body  string // <content:encoded> → <description> fallback, "" if absent
  }

  // Doc wraps a parsed feed so titles can be rewritten in place.
  type Doc struct{ /* *etree.Document + per-item <title> element handles */ }

  func Parse(raw []byte) (*Doc, error)         // etree.ReadFromBytes; permissive read settings
  func (d *Doc) Items() []Item                 // in document order; index aligns with SetTitle
  func (d *Doc) SetTitle(i int, title string)  // replace only item i's <title> text
  func (d *Doc) Bytes() (out []byte, err error)// serialize; untouched nodes byte-identical
  ```
  - Read with `etree.ReadSettings` tuned to **preserve CDATA** and keep the XML
    declaration; write with matching `WriteSettings` (no reindent/canonicalization)
    so untouched subtrees round-trip byte-for-byte.
  - `Body`: prefer the namespaced `<content:encoded>` child, else `<description>`,
    else `""`. Pass the element's text through as-is (no HTML stripping this
    release — see Open questions).
  - `SetTitle` mutates the cached `<title>` element's text only; never re-creates
    the element or reorders siblings.
- **Tests:** `pkg/feed/feed_test.go` — the fidelity matrix (spec §13). Use small
  hand-written RSS fixtures so byte assertions are exact:
  - **Round-trip identity:** `Parse` then `Bytes` with **no** `SetTitle` reproduces
    the input byte-for-byte (the canonicalization guard).
  - **Surgical change:** after `SetTitle` on one item, the output differs from the
    input **only** within that item's `<title>` (diff the rest).
  - **Preservation:** a fixture with `<enclosure>`, a custom namespace
    (`media:content`), `<category>`, `<author>`, `<pubDate>`, `<guid>` keeps all of
    them unchanged after a title rewrite.
  - **Item id:** `ID` = `<guid>` when present; falls back to `<link>` when guid is
    absent; both absent → `""` (caller skips rewrite — note in §rewrite).
  - **Body extraction:** `content:encoded` wins over `description`; CDATA body text
    is returned undecorated; missing both → `Body == ""`.
  - **CDATA / escaping:** a title and a description wrapped in `<![CDATA[...]]>`
    round-trip; rewriting the title does not corrupt the CDATA description.
- **Done when:** `go test ./pkg/feed/ -v` is green, especially the round-trip
  identity and "only the title changed" diff tests.

### 4. `pkg/aiclient` — any-llm-go wrapper + fake

- **Goal:** A thin, mockable AI transport. Production wraps `any-llm-go`'s OpenAI
  provider configured from `config.AI` (endpoint/key/model — no hardcoded vendor,
  spec §8). A `FakeClient` gives unit/integration tests deterministic, scriptable
  behavior. **Rate limiting and singleflight live in `pkg/rewrite`, not here** — this
  is pure transport.
- **Changes:** `pkg/aiclient/aiclient.go`, `pkg/aiclient/fake.go`. Add the
  `github.com/mozilla-ai/any-llm-go` dependency (`go get`).
  ```go
  // Client turns a system+user prompt into the model's text reply.
  type Client interface {
      Complete(ctx context.Context, system, user string) (string, error)
  }

  // New builds an OpenAI-compatible client from config (base_url, api_key, model).
  func New(cfg config.AI) (Client, error)
  ```
  Impl: `openai.New(anyllm.WithBaseURL(cfg.BaseURL), anyllm.WithAPIKey(cfg.APIKey))`;
  `Complete` sends `anyllm.CompletionParams{Model: cfg.Model, Messages: {{Role:System,
  Content:system},{Role:User,Content:user}}}` and returns
  `resp.Choices[0].Message.Content` (trimmed), erroring on an empty choices slice.
  The per-call timeout is applied by the caller's context (`pkg/rewrite`), not here.
  - `FakeClient`: fields for a canned reply function `func(system, user string)
    (string, error)`, an invocation counter (atomic), and an optional `block chan
    struct{}` so tests can hold a call open to exercise timeout/singleflight.
- **Tests:** `pkg/aiclient/fake_test.go`: `FakeClient.Complete` returns the scripted
  value, increments the counter, and (when configured) blocks until released. The
  real `New` is **not** unit-tested against a live endpoint (no network in CI);
  cover it only via the construction path (valid config → non-nil client, no error)
  and rely on the integration test's injected fake for behavior.
- **Done when:** `go test ./pkg/aiclient/ -v` passes; `Client` is satisfied by both
  the real wrapper and the fake.

### 5. Extend the `Handler` interface — read-the-body rewrite input

- **Goal:** Grow the `Handler` interface from "only `Prompt()`" to also produce the
  **AI user content** for an item, so the model reads the full content before
  retitling (spec §5.2/§8 refinement — see Spec deviations). The general handler
  implements it; specialized handlers can override later.
- **Changes:** `pkg/handler/handler.go`, `pkg/handler/general.go`.
  ```go
  type Handler interface {
      Prompt() string
      // ComposePrompt builds the AI user message for one item. The system prompt
      // is the Resolution's EffectivePrompt(); this returns the user content. The
      // general handler appends the item body so the model reads the full text;
      // specialized handlers may reshape it.
      ComposePrompt(item feed.Item) string
  }
  ```
  General impl:
  ```go
  func (generalHandler) ComposePrompt(item feed.Item) string {
      if item.Body == "" {
          return item.Title
      }
      return "Title: " + item.Title + "\n\nContent:\n" + item.Body
  }
  ```
  `pkg/handler` now imports `pkg/feed` (feed does **not** import handler → no
  cycle). Update the existing `Handler` test doubles in `pkg/handler/*_test.go` to
  add a no-op/`ComposePrompt` so they still satisfy the interface.
- **Tests:** `pkg/handler/general_test.go`: `ComposePrompt` with a body includes
  both title and body; with an empty body returns just the title. Existing resolver
  tests stay green after the test-double update.
- **Done when:** `go test ./pkg/handler/ -v` passes.

### 6. `pkg/rewrite` — cache + singleflight + rate limit + timeout fallback (concurrency core)

- **Goal:** The orchestrator that turns raw RSS + a `handler.Resolution` into
  rewritten XML, implementing spec §5.3 + §7 end to end. Depends on `feed`,
  `aiclient`, and a narrow cache interface (so unit tests need no Postgres).
- **Changes:** `pkg/rewrite/rewrite.go`.
  ```go
  // Cache is the subset of *store.Store the rewriter needs (consumer interface,
  // so tests inject an in-memory fake; *store.Store satisfies it).
  type Cache interface {
      LookupTitles(ctx context.Context, handler string, ids []string) (map[string]store.ItemTitle, error)
      SaveTitle(ctx context.Context, it store.ItemTitle) error
  }

  type Stats struct {
      ItemCount, CacheHits, CacheMisses, AICalls int
      TimedOut bool
  }

  type Rewriter struct{ /* cache, ai, *rate.Limiter, singleflight.Group, aiTimeout, waitTimeout, logger */ }

  func New(cache Cache, ai aiclient.Client, rpm int, aiTimeout, waitTimeout time.Duration, logger *mlog.Logger) *Rewriter

  // RewriteFeed parses raw, resolves every item title (cache-first, AI on miss),
  // and returns the modified XML plus per-feed stats. On a parse error it returns
  // the raw bytes unchanged so an unparseable feed is never corrupted.
  func (rw *Rewriter) RewriteFeed(ctx context.Context, res handler.Resolution, raw []byte) (out []byte, stats Stats, err error)
  ```
  Algorithm:
  1. `feed.Parse(raw)`; on error return `(raw, Stats{}, err)` (caller passes raw
     through). `items := doc.Items()`; `stats.ItemCount = len(items)`.
  2. Collect ids (skip items with empty id — leave their title as-is). One
     `cache.LookupTitles(ctx, res.Prefix, ids)`.
  3. For each item: **hit** when a row exists **and** `row.SourceTitle ==
     item.Title` → `SetTitle(i, row.RewrittenTitle)`, `CacheHits++`. Otherwise it's
     a **miss/changed** → register a rewrite future (below), `CacheMisses++`.
  4. **Rewrite future** per missing item, keyed `res.Prefix + "|" + id` via
     `group.DoChan`. The closure runs on a **detached** context — `bg :=
     mlog.CopyTraceID(ctx, context.Background())`, then
     `context.WithTimeout(bg, aiTimeout)` — **not** the request ctx:
     - `limiter.Wait(bg)` (token-bucket from `rpm`; emit `ratelimit.wait` on a
       non-trivial wait).
     - `title, err := ai.Complete(bg, res.EffectivePrompt(), res.Handler.ComposePrompt(item))`.
       Count `AICalls++` (atomic, since closures run in goroutines).
     - On success: `cache.SaveTitle(bg, ItemTitle{ID,Handler:res.Prefix,
       SourceTitle:item.Title, RewrittenTitle:title})`, return title. On **failure:
       return the error and do NOT write** (spec §7.4 — never cache a failed/source
       title).
  5. **Request-side wait:** one shared budget `deadline := time.Now().Add(waitTimeout)`.
     For each future, `select { case r := <-ch: ...; case <-time.After(time.Until(deadline)): timedOut }`.
     - Result OK → `SetTitle(i, r.Val)`. Result error → leave source title.
     - Timeout → leave source title, `stats.TimedOut = true`; the singleflight
       goroutine keeps running on `bg` and writes the row when it finishes
       (`rewrite.background_done`). A later request for the same key reuses the
       in-flight `DoChan` (no second AI call); once it completes the row is a cache
       hit.
  6. `doc.Bytes()` → out.
  - Use `singleflight.DoChan` (not `Do`) so the AI call runs in its own goroutine on
    `bg` and survives request return; the result channel is buffered, so the send
    never blocks even when the request already gave up.
- **Tests:** `pkg/rewrite/rewrite_test.go` with a `fakeCache` (in-memory map) and
  `aiclient.FakeClient`. A tiny RSS fixture with 1–2 items:
  - **Miss → AI → cache write:** empty cache, fake returns `"REWRITTEN"` → output
    title is `REWRITTEN`, `Stats{CacheMisses:1, AICalls:1}`, and the fake cache now
    holds the row.
  - **Hit:** pre-seed the cache with a matching `source_title` → no AI call
    (counter stays 0), output uses the cached `rewritten_title`, `CacheHits:1`.
  - **Source changed:** cached row whose `source_title` differs from upstream →
    treated as a miss, AI called, row updated.
  - **Dedup (singleflight):** two concurrent `RewriteFeed` of the same feed (same
    `(handler,id)`), fake configured to block until both callers arrive →
    `Complete` invoked **once**; both get the rewritten title.
  - **Timeout fallback + background write:** `waitTimeout` tiny, `aiTimeout` large,
    fake blocks past the wait → `RewriteFeed` returns with the **source** title and
    `TimedOut:true`; after releasing the fake, poll the fake cache until the row
    appears (background goroutine wrote it). A subsequent `RewriteFeed` is a cache
    hit with no new AI call.
  - **AI failure:** fake returns an error → output keeps the source title, cache
    stays empty (no write), `AICalls:1`.
  - **No id:** item with neither guid nor link → title untouched, not counted as a
    miss, no AI call.
- **Done when:** `go test ./pkg/rewrite/ -v -race` is green across the matrix
  (run with `-race` — this is the concurrency core).

### 7. `pkg/upstream` — buffered fetch

- **Goal:** Fetch the upstream feed into memory (the handled branch needs the whole
  body to parse with etree — unlike `pkg/proxy`'s streaming passthrough, which is
  why this is a separate client; see Open questions). Preserve the inbound path and
  raw query so RSSHub renders the same feed.
- **Changes:** `pkg/upstream/upstream.go`, using `resty/v3` (`go get
  resty.dev/v3`), per the spec stack and locked decision #2.
  ```go
  type Response struct {
      Status      int
      ContentType string
      Body        []byte
  }

  func New(baseURL string, timeout time.Duration) (*Fetcher, error) // parse+validate base
  func (f *Fetcher) Fetch(ctx context.Context, path, rawQuery string) (*Response, error)
  ```
  - Build the target URL as base + inbound path + `?rawQuery` (mirror the proxy's
    single-slash guard for a base path of `/`). Use the request context.
  - Return the upstream status, `Content-Type`, and buffered body. A transport
    error is returned as `err` (caller maps to 502); a non-2xx status is returned
    **as data** (caller passes it through unchanged — we don't rewrite an error
    body).
- **Tests:** `pkg/upstream/upstream_test.go` with an `httptest.Server`:
  - Fidelity: upstream echoes path + query and returns a known status/body/
    content-type → `Fetch` reproduces all of them; upstream saw the original path
    and raw query.
  - Error: point at a closed address → `Fetch` returns an error.
- **Done when:** `go test ./pkg/upstream/ -v` passes.

### 8. Gateway handled branch + `request.done` aggregate

- **Goal:** Replace the plan-1 `// TODO plan-2` seam with the real handled flow,
  add `?format=atom` to the passthrough set (Spec deviation), and upgrade the light
  resolve log to the §11.4 `request.done` aggregate event.
- **Changes:** `pkg/gateway/gateway.go`.
  - `New` gains the fetcher + rewriter:
    ```go
    func New(resolver *handler.Resolver, proxy *proxy.Proxy, fetcher *upstream.Fetcher, rewriter *rewrite.Rewriter, logger *mlog.Logger) *Gateway
    ```
  - `decide`: `c.QueryParam("format")` of `"json"` **or** `"atom"` →
    `modePassthrough` (only default RSS 2.0 is rewritten this release).
  - Handled branch (`modeHandled`):
    1. `resp, err := fetcher.Fetch(ctx, path, rawQuery)`; transport error → `502`
       (log `upstream.fetch` WARN) and emit `request.done`.
    2. Non-2xx upstream → write `resp.Status` + body unchanged (transparent), emit
       `request.done` (mode handled, zero rewrite stats).
    3. 2xx → `out, stats, err := rewriter.RewriteFeed(ctx, res, resp.Body)`. On a
       rewrite/parse error, `out` is the raw body (rewriter returns raw) → write it
       unchanged. Set `Content-Type` from `resp.ContentType`; write `200` + `out`.
    4. Emit `request.done` (INFO) with `path, handler(=res.Prefix), mode, status,
       duration_ms, item_count, cache_hits, cache_misses, ai_calls, timed_out`.
  - Passthrough branches: after `proxy.Passthrough(c)`, emit `request.done` with
    `mode=passthrough`, `status=c.Response().Status`, and zero rewrite stats.
  - Keep the per-item hit/miss detail at DEBUG (emitted inside `pkg/rewrite`); INFO
    carries only the one aggregate line (spec §11.4).
- **Tests:** `pkg/gateway/gateway_internal_test.go` / `gateway_test.go` with a fake
  upstream (`httptest`) and an injected `aiclient.FakeClient`:
  - `decide` returns `modePassthrough` for `format=json` **and** `format=atom`, and
    `modeHandled` for default format on an enabled prefix (internal test).
  - Handled branch end-to-end: enabled prefix, default format, fake AI returns a
    known title → response body has the rewritten title and is otherwise equal to
    the upstream body (in-process, deterministic; the byte-level "only title
    changed" assertion is the real-RSSHub test in step 9).
  - Upstream non-2xx → gateway returns the same status/body.
  - Build a `Rewriter` for these tests from the fake cache + fake AI (or the real
    store behind the env gate) so the gateway test stays hermetic.
- **Done when:** `go test ./pkg/gateway/ -v` passes (non-integration cases run
  without Docker/DB).

### 9. Real-RSSHub integration — only `<title>` bytes change

- **Goal:** The strongest end-to-end assertion of surgical fidelity: through the
  gateway with a **stub AI**, a handled feed differs from the direct upstream feed
  **only** in `<item>/<title>` text — every other byte is identical.
- **Changes:** `pkg/gateway/rsshub_integration_test.go` (extend), reusing the
  plan-1 infra (`compose.test.yaml`, `RSS_AI_TEST_UPSTREAM`, RSSHub `/test/*`). The
  gateway under test is built with an `aiclient.FakeClient` returning a
  deterministic transform of the source title (e.g. `"AI: " + title`) so the diff
  is predictable, and an in-memory `fakeCache` (or the real store when
  `RSS_AI_TEST_DSN` is also set) so the test needs no Postgres by default.
  - Fetch `/test/1` directly and through the gateway (prefix `/test` enabled,
    default format). Parse both with `pkg/feed`; assert: same item count, every
    non-title field equal item-by-item, and each gateway title equals the fake's
    transform of the corresponding upstream title. (Comparing via `pkg/feed`
    tolerates the title node's CDATA/escaping representation while still proving
    nothing else moved.)
  - Keep the plan-1 passthrough cases (`format=json`, `format=atom`, unmatched)
    byte-transparent.
- **Tests:** the above; env-gated, SKIPs without `RSS_AI_TEST_UPSTREAM`.
- **Done when:** `just integration` is green: handled feed = upstream feed except
  rewritten titles; passthrough/atom/json remain byte-identical.

### 10. Wire it together in `main`

- **Goal:** The running binary performs the full handled flow.
- **Changes:** `cmd/rss-ai/main.go`. After `store.New`: build
  `ai, err := aiclient.New(cfg.AI)`; `fetcher, err := upstream.New(cfg.Upstream.BaseURL,
  cfg.AI.RequestTimeoutDur)`; `rewriter := rewrite.New(st, ai, cfg.AI.RPM,
  cfg.AI.RequestTimeoutDur, cfg.Gateway.WaitTimeoutDur, logger)`; pass `fetcher` +
  `rewriter` into `gateway.New`. `*store.Store` satisfies `rewrite.Cache`.
- **Tests:** `go build ./...` clean. Binary smoke as in plan-1 still dies at
  `store.New` without Postgres — runtime behavior is covered by step 9, not the
  smoke (note in lessons if unchanged).
- **Done when:** `go build ./...` is clean and `gateway.New`'s new dependencies are
  satisfied.

### 11. Lint + progress + lessons consolidation

- **Goal:** Green tree, updated docs, one consolidation pass on the lessons file.
- **Changes:** `docs/PROGRESS.md` (mark plan-2 done; note the handled seam is
  filled), `docs/lessons/2026-06-10-rewrite-cache-lessons.md` (consolidate per
  `docs/lessons/README.md`). Run `dprint fmt` on edited docs (plan-1 gotcha).
- **Tests / commands:** `just lint`; `go test ./... -race` → PASS (store + rewrite
  - RSSHub integration SKIP without their env vars).
- **Done when:** lint clean, suite green, docs current, lessons consolidated.

> Commit per task using Conventional Commits, e.g.
> `feat(config): parse ai/gateway timeouts to durations`,
> `feat(store): batch title lookup + update-or-insert`,
> `feat(feed): surgical etree title rewrite`,
> `feat(aiclient): any-llm-go wrapper + fake`,
> `feat(handler): read-body ComposePrompt seam`,
> `feat(rewrite): cache + singleflight + timeout fallback`,
> `feat(upstream): buffered resty fetch`,
> `feat(gateway): handled branch + request.done event`,
> `test(gateway): handled-branch RSSHub fidelity`,
> `docs: mark plan-2 complete and consolidate lessons`.

---

## Seam signatures (the two the spec calls out)

**① `Handler` interface — from `Prompt()` to read-the-body rewrite input**

```go
// feed.Item — handler-agnostic extracted item (pkg/feed)
type Item struct {
    ID    string // <guid> → <link>
    Title string // source title
    Body  string // <content:encoded> → <description>, "" if absent
}

// pkg/handler
type Handler interface {
    Prompt() string                         // built-in default; config prompt overrides (§4)
    ComposePrompt(item feed.Item) string    // AI *user* content; system = Resolution.EffectivePrompt()
}
```

The general handler implements `ComposePrompt` (title + body). Specialized handlers
override it later for sources whose structure needs different framing. The AI
plumbing (rate limit, singleflight, timeout) stays in `pkg/rewrite`, so the handler
only decides _what_ to send.

**② `pkg/store` query/write methods**

```go
func (s *Store) LookupTitles(ctx context.Context, handler string, ids []string) (map[string]ItemTitle, error)
func (s *Store) SaveTitle(ctx context.Context, it ItemTitle) error
```

`LookupTitles` is a single `id IN (...)` read keyed by `(handler, ids)`; `SaveTitle`
is app-level update-or-insert (First → Updates|Create), relying on singleflight for
per-key serialization (spec §6 — no DB upsert). `pkg/rewrite` consumes them through
a narrow `Cache` interface so its unit tests need no Postgres.

---

## Spec deviations (sync back to the spec)

These three points refine the spec; the plan follows the **locked decisions**. Each
should be written back into `docs/spec/2026-06-09-...md` (or noted there as
amended):

1. **Atom is passthrough this release** (refines §3, §5.1, §13). The spec routes
   both default RSS 2.0 and `?format=atom` through the rewrite path; plan-2 supports
   **only RSS 2.0** and passes `?format=atom` through alongside `?format=json`.
   etree rewrite covers RSS 2.0 `<item>/<title>` only. → update §3's output-format
   table and §14 non-goals.
2. **AI input includes the item body** (refines §5.2/§8). The spec's §8 input is
   "原始标题 (+ prompt)"; plan-2 also feeds the item body (`<content:encoded>` /
   `<description>`) so the model reads the full content before retitling. Output is
   still only the rewritten title; no other field changes. → update §8 input line.
3. **Cache invalidation keys on `source_title` only** (already §5.3, made explicit).
   A changed body with an unchanged title is **not** re-rewritten (accepted
   trade-off). Only the title mapping is cached; bodies are fetched live and fed to
   the AI transiently on a miss, never stored (§6 table unchanged — no body column).

---

## Open questions

1. **Upstream fetch: reuse proxy or new client? → new client (resty/v3).** The
   proxy streams straight to the response writer; the handled branch must **buffer**
   the whole feed for etree, so the semantics don't overlap (locked decision #2's
   own reasoning). Recommendation: a small `pkg/upstream` over **resty/v3**, matching
   the documented stack and giving timeout/retry ergonomics for free. _Trade-off:_ a
   plain `net/http.Client` GET would also suffice for a single buffered request and
   avoids a dependency — but the spec already lists resty/v3, so use it. Revisit only
   if resty proves heavier than the one GET warrants.
2. **any-llm-go shape & config wiring.** Confirmed API: `openai.New(anyllm.WithBaseURL,
   anyllm.WithAPIKey)` → `provider.Completion(ctx, anyllm.CompletionParams{Model,
   Messages})` → `resp.Choices[0].Message.Content`. The per-call **timeout** is the
   caller's context deadline (`pkg/rewrite` sets `aiTimeout` on the detached bg ctx),
   not a client option. Verify the exact option/param identifiers against the pinned
   version at implementation time and pin it in `go.mod`.
3. **Duration parsing location → config load (step 1).** `time.Duration` has no
   `TextUnmarshaler`, so go-toml can't decode `"30s"` directly. Parse once in
   `config.Init` into exported `…Dur` fields and fail fast on a bad value; consumers
   read typed Durations and never re-parse. (Alternative — parse at each use site —
   rejected: duplicated parsing, late failure.)
4. **CDATA / escaping / XML-declaration fidelity.** The byte-identity round-trip
   (step 3) and the real-RSSHub "only the title changed" diff (step 9) are the
   oracles. Tune `etree.ReadSettings`/`WriteSettings` to preserve CDATA, keep the
   declaration, and avoid reindenting. Open sub-question: when a source title was
   itself CDATA, should the rewritten title be re-wrapped in CDATA or escaped? Either
   is valid XML and the title node is the one field we _intend_ to change, so the
   integration assertion tolerates it — default to whatever etree's `SetText`
   produces and only revisit if a reader complains.
5. **Body to AI: raw HTML or stripped?** This release passes the
   `<content:encoded>`/`<description>` text as-is (may contain HTML). Stripping tags
   for a cleaner prompt is a possible later refinement; not done now (YAGNI).
6. **Testing in-flight singleflight ↔ timeout handoff.** Use `aiclient.FakeClient`
   with a `block chan` the test releases on cue: (a) **dedup** — fire two concurrent
   `RewriteFeed`s for the same key, fake blocks until both arrive, assert `Complete`
   ran once; (b) **timeout→background** — tiny `waitTimeout`, large `aiTimeout`, fake
   blocks past the wait, assert the call returns the source title with `TimedOut`,
   then release and poll the fake cache until the background write lands, and assert
   the next call is a cache hit with no new AI call. Run the package with `-race`.
7. **Gateway `request.done` for passthrough status.** The proxy writes directly, so
   read `c.Response().Status` after `Passthrough` for the event's `status` field.
   Confirm Echo v5 populates `Response().Status` after a reverse-proxy write (plan-1
   already unwraps for `.Status`); if not, capture via a status-recording wrapper.
8. **Concurrency of AI calls within one feed.** Misses are launched concurrently
   (each its own `DoChan`) and share one `waitTimeout` budget, so wall-clock wait is
   ~max, not sum. The `rate.Limiter` (rpm) bounds the AI call rate across all of them.
   Confirm this matches the intended read of §7.3 ("请求侧带 wait_timeout 的 select")
   — the plan treats the wait as per-request (shared deadline over concurrent items),
   not per-item-sequential.

```
```
