# Title Rewrite + Cache — Lessons

Filling the handled seam: surgical etree title rewrite (spec §5.1), the title
cache (§5.3/§6), and the concurrency trio + timeout fallback (§7), plus the
AI-client and upstream-fetch plumbing.

## Config

- `time.Duration` has no `TextUnmarshaler`, so go-toml can't decode `"30s"`
  straight into a Duration field. Keep the TOML string, add a `toml:"-"` Duration
  sibling, and parse once in `config.Init` (fail fast on a bad value). Consumers
  read the typed field and never re-parse.

## Storage / GORM

- For an app-level update-or-insert, use `Limit(1).Find` + `RowsAffected == 0` to
  branch, not `First`: `First` logs `record not found` at error level when the row
  is absent, which is noise on the common insert path.
- `Updates(map[string]any{...})` auto-bumps `updated_at`; `Create` auto-fills both
  timestamps — no need to set them by hand.
- The rewriter calls `SaveTitle` concurrently (one goroutine per item). The real
  `*store.Store` (GORM/pgx) is safe, but **in-memory `Cache` test doubles need a
  mutex** — a bare map hits `fatal error: concurrent map writes` on any multi-item
  feed. Single-item fixtures hide this; the real RSSHub feed exposed it.

## XML fidelity (beevik/etree v1.6.0)

- `ReadSettings{Permissive:true, PreserveCData:true}` + default write settings
  round-trips byte-for-byte: XML declaration, self-closing `<enclosure .../>`,
  CDATA, and namespaced `<content:encoded>` all survive verbatim. No WriteSettings
  tuning needed.
- `SelectElement("content:encoded")` matches the namespace prefix directly (the tag
  arg may include `prefix:`), so no manual child iteration for namespaced bodies.
- Preserve a title's CDATA-vs-text wrapping on rewrite: `SetCData` if the original
  title's `CharData.IsCData()`, else `SetText`. Minimizes the diff.

## AI client (any-llm-go v0.9.0)

- `openai.New(anyllm.WithBaseURL, anyllm.WithAPIKey)` → `*openai.Provider`, which
  embeds `*CompatibleProvider` carrying `Completion(ctx, CompletionParams)
  (*ChatCompletion, error)`. There is no `WithModel` — model is a
  `CompletionParams` field.
- `Message.Content` is typed `any`, not `string`: pass a string in, and
  **type-assert** `Choices[0].Message.Content.(string)` on the way out.
- `openai.New` has `RequireAPIKey: true`, so a construction test must pass a
  non-empty key; construction itself makes no network call.
- Keep transport thin: a local `completer` interface (the one `Completion` method)
  makes the provider swappable; the public seam is the `Client` interface. The
  per-call timeout is the **caller's context deadline** (set in pkg/rewrite), not a
  client option.
- `go mod tidy` pulls the real `github.com/openai/openai-go` transitively — needed
  before the package compiles.

## Concurrency (pkg/rewrite)

- `singleflight.DoChan` (not `Do`): it runs the closure in its own goroutine and
  returns a buffered channel, so the AI call + cache write survive a request that
  stops reading. Build the detached context **inside** the closure via
  `mlog.CopyTraceID(reqCtx, context.Background())` + `WithTimeout(aiTimeout)`, so
  request cancellation never reaches the AI call (the §7.3 timeout-vs-cancel rule)
  while the trace_id carries through.
- A second concurrent request for the same key joins the in-flight call; only the
  first caller's closure runs, so its `AICalls` increments and the second reports
  0 — natural per-request stats.
- One shared `deadline` across all per-item futures makes the wait per-request
  (wall-clock ≈ max of the concurrent calls), not per-item-sequential.
- Cache-read failure degrades to "rewrite everything" (log WARN), not serving raw
  titles. Trade-off: a DB outage means unbounded AI calls until it recovers.
- Skipped `rewrite.background_done` (§11.4): the closure can't tell whether the
  request waited or timed out, so a precise event is awkward; `ai.rewrite` (ok) at
  DEBUG covers it.

## Gateway / HTTP (Echo v5)

- `c.Response()` returns a bare `http.ResponseWriter` (no `.Status`). Read the
  proxied status with `echo.UnwrapResponse(c.Response())` → `(*echo.Response,
  error)` (the server's logging middleware uses the same call). `c.Blob(status,
  contentType, body)` writes the rewritten feed.
- plan-2 is RSS-2.0-only, so `?format=atom` joins `?format=json` and unmatched
  paths in the passthrough set; only default-format enabled prefixes are handled.

## Testing & infra

- **Test surgical edits against the no-op round-trip baseline, not the raw input.**
  `want := strings.Replace(baseline, oldTitle, newTitle, 1)` asserts "only the
  title changed" independent of formatting quirks; a separate identity test pins
  baseline == input for hand-written fixtures.
- **The real-RSSHub fidelity assertion needs no AI-output guessing:** make the fake
  AI return a _constant_, then compute expected = `feed.Parse(directBytes)` →
  `SetTitle(all, constant)` → `Bytes()`. The gateway does exactly that over the
  same cached upstream bytes, so `expected == through` proves only titles moved —
  byte-for-byte, including fields etree might reformat. (resty fetch and a direct
  fetch hit the same RSSHub cache window, so the gateway's input == the direct
  bytes — same property plan-1 relied on.)
- **Deterministic concurrency tests need an arrival signal.** `FakeClient.Block` (a
  channel released by `close()`) + `require.Eventually(ai.Calls()==1)` pins "the
  first call is in flight" before starting the second goroutine, else the dedup
  race is flaky. Timeout test: tiny `waitTimeout`, large `aiTimeout`, fake blocks
  past the wait → assert fallback + `TimedOut`, then `close(Block)` and
  `Eventually(cache.len()==1)` to prove the background write landed, then assert the
  next call is a clean hit. Run the package with `-race`.
- No local Postgres in the env: spin a throwaway `postgres:16-alpine` on a high
  port and point `RSS_AI_TEST_DSN` at it to actually exercise the store methods
  (otherwise they just skip).
- `dprint`'s emphasis normalization (`*x*` → `_x_`) is enforced by `just lint`; run
  the bare `dprint fmt` (config globs), not `dprint fmt docs/`, to catch it.
