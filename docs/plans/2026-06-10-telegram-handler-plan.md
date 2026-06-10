# Telegram Handler Implementation Plan

**Spec:** docs/spec/2026-06-09-rss-ai-gateway-design.md (§4 three-layer resolution, §8 AI input)
**Lessons:** docs/lessons/2026-06-10-telegram-handler-lessons.md
**Status:** done

## Overview

plan-2 left the specialized-handler seam ready but unused: every enabled prefix
falls back to `generalHandler`, which feeds the model `Title: … / Content: …`.
That seam (`handler.Handler` = `Prompt()` + `ComposePrompt(feed.Item)`, plus
`handler.Register` from `init()`, looked up by the config-matched prefix key) is
the only extension point — this plan fills it for the **first** time with a
specialized **Telegram** handler.

Why Telegram needs one: RSSHub's telegram channel route (`/telegram/channel/:user`,
built from the `t.me/s/` web preview) has **no real title** — RSSHub
auto-derives `<title>` by truncating the message text, while `<description>`
holds the full message **HTML** (links, `<br>`, emoji, "Forwarded from …"
lines, media captions). Feeding the truncated pseudo-title as `Title:` misleads
the model, and the raw HTML wastes tokens and confuses it. The Telegram handler
(a) composes a **body-centric** prompt that treats the body as the source of
truth and de-emphasizes the synthetic title, (b) **strips HTML to plain text**
and bounds length before sending, and (c) ships a **Telegram-specific system
prompt** asking for a short, readable title. Output is still only the rewritten
title; the rewrite/cache/concurrency machinery from plan-2 is reused unchanged.

This plan establishes the **pattern** every future specialized handler follows:
a self-contained sub-package, `init()` registration, a blank import in `cmd`,
and a matching config entry under the **same** prefix key.

## Design decisions

- **Prefix = `/telegram/channel`** (decided). The same-key rule (spec §4,
  `TestResolveSameKeyRule`) requires the registry key to equal the prefix
  **config enables** — a handler registered at a key config doesn't enable is
  dormant. Channel is the only telegram route whose titles are synthetic;
  `/telegram/stickerpack`, `/telegram/blog`, etc. are fine on the general
  handler. So register **and** enable `/telegram/channel`. (Alternative:
  register+enable `/telegram` to cover all telegram routes with channel logic —
  simpler config, but applies message-title logic to non-message routes. See
  Open questions.)
- **New sub-package `pkg/handler/telegram`**, imported for side effect
  (`init()` → `handler.Register`) via a blank import in `cmd/rss-ai`. This keeps
  specialized handlers out of the core `handler` package and mirrors the
  database/sql driver pattern the exported `Register` already implies. No import
  cycle: `telegram` imports `handler` + `feed`; neither imports `telegram`.
- **Pure + unit-testable.** The handler depends only on `feed.Item` (ID, Title,
  Body) — no I/O, no AI, no config. HTML→text is a small internal helper with
  its own table tests.

## Steps

### 1. HTML-to-text helper for Telegram bodies — [x]

- **Goal:** `htmlToText(string) string` that turns a Telegram `<description>`
  HTML fragment into clean plain text the model can read.
- **Changes:** `pkg/handler/telegram/htmltext.go` (unexported helper in the
  telegram package). Use `golang.org/x/net/html` to tokenize (already an
  indirect dep via resty/echo; promote to direct if needed). Drop tags, decode
  entities, turn `<br>`/block boundaries into single newlines, collapse runs of
  whitespace, trim.
- **Tests:** `htmltext_test.go` — tags stripped (`<a href>text</a>` → `text`);
  entities decoded (`&amp;` → `&`); `<br>` and `</p>` become newlines; multiple
  blank lines/spaces collapse; empty/plain input passes through; a realistic
  multi-line Telegram message fixture renders readably.
- **Done when:** helper passes the table tests; no external network or AI.

### 2. Telegram handler type (`Prompt` + `ComposePrompt`) — [x]

- **Goal:** a `Handler` whose `Prompt()` is Telegram-specific and whose
  `ComposePrompt(feed.Item)` is body-centric.
- **Changes:** `pkg/handler/telegram/telegram.go`:
  - `const maxBodyChars` (e.g. 4000) — bound long-form posts before they hit the
    model; truncate on a rune boundary with an ellipsis marker.
  - `telegramPrompt` system prompt: "These are Telegram channel messages. The
    title is auto-generated and may be truncated or missing — derive a concise,
    informative title (roughly ≤ 80 chars) from the message content, in the
    message's own language. Strip forwarding/emoji/promo noise; output the title
    only, no quotes or markdown."
  - `Prompt()` returns `telegramPrompt`.
  - `ComposePrompt(item)`: run `htmlToText(item.Body)`, truncate to
    `maxBodyChars`; emit a **message-first** layout, e.g.
    `Telegram message:\n<text>` and append the synthetic title only as a weak
    hint (`\n\n(auto-generated title hint: <title>)`) — or omit it entirely when
    the title is just a prefix of the body. With empty body, fall back to the
    title alone (mirror generalHandler's empty-body behavior).
- **Tests:** `telegram_test.go` — `Prompt()` non-empty and Telegram-specific
  (mentions Telegram); `ComposePrompt` contains the plain-text body and **not**
  raw HTML tags; long body is truncated to ≤ maxBodyChars; empty body → title
  only; body that the title is a prefix of → title hint suppressed.
- **Done when:** handler satisfies `handler.Handler` (compile-time
  `var _ handler.Handler = …`) and all unit tests pass.

### 3. Self-registration + cmd wiring — [x]

- **Goal:** `/telegram/channel` resolves to the Telegram handler at runtime.
- **Changes:**
  - `pkg/handler/telegram/telegram.go`: `func init() { handler.Register("/telegram/channel", New()) }`.
  - `cmd/rss-ai/main.go`: blank import
    `_ "github.com/eli-yip/rss-ai/pkg/handler/telegram"` so `init()` runs before
    `handler.NewResolver` is called.
- **Tests:** `pkg/handler/telegram/register_test.go` — build a
  `handler.NewResolver(map[string]config.HandlerConfig{"/telegram/channel": {Enabled:true}})`,
  resolve `/telegram/channel/durov/123`, assert `Matched && Specialized` and
  that `EffectivePrompt()` is the Telegram prompt (and that a config `prompt`
  still overrides it, per spec §4). Registering in this package's own test
  binary exercises the real `init()` path without polluting core `handler`
  tests.
- **Done when:** resolver returns the specialized Telegram handler for the
  channel prefix; `go build ./...` and `go test ./...` green.

### 4. Config + end-to-end resolution — [x]

- **Goal:** the shipped example enables Telegram and documents the optional
  prompt override.
- **Changes:** `config.example.toml` — add
  ```toml
  [handlers."/telegram/channel"]
  enabled = true
  # No prompt → uses the specialized Telegram handler's built-in prompt.
  # Set prompt here to override it without touching code (spec §4).
  ```
- **Tests:** covered by Step 3's resolver test (config-driven). Optionally extend
  the existing gateway integration test (gated on `RSS_AI_TEST_UPSTREAM`) with a
  `/telegram/channel/<public-channel>` case asserting the handled feed is the
  upstream feed with only `<item>/<title>` changed — same invariant plan-2
  proved for the general handler. Keep it env-gated so the default suite stays
  green.
- **Done when:** with the example config, a request under `/telegram/channel`
  takes the handled branch via the specialized handler.

### 5. Docs — [x]

- **Goal:** record the milestone and the now-exercised seam.
- **Changes:** `docs/PROGRESS.md` — flip "Production specialized handlers" toward
  done, note Telegram as the first specialized handler and the
  sub-package + blank-import pattern; create
  `docs/lessons/2026-06-10-telegram-handler-lessons.md` (append-only during
  execution, consolidate at the end per `docs/lessons/README.md`).
- **Done when:** PROGRESS reflects plan-3; lessons file exists.

## Open questions

- ~~**Prefix scope:** `/telegram/channel` vs `/telegram`.~~ **Decided:
  `/telegram/channel`** — precise; the general handler covers other telegram
  routes. This is both the `Register` key and the config key (they must match).
- **Title hint inclusion:** always append the synthetic title as a hint, or drop
  it when it's a prefix of the body? Cheap to test both; default to suppress-on-
  prefix (Step 2) and revisit if titles regress.
- **`maxBodyChars` value:** 4000 is a starting point; tune against real channel
  posts and the model's context/cost once observed.
