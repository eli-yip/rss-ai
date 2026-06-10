# Telegram Handler — Lessons

First use of the plan-2 specialized-handler seam. Notes worth keeping.

## The seam / registration

- `handler.Register` is exported precisely so specialized handlers live in their
  own sub-package and self-register from `init()` (database/sql driver pattern).
  The core `handler` package never imports them → no cycle; `cmd/rss-ai` blank
  imports the sub-package to make `init()` run before `NewResolver`.
- Same-key rule bites here: the `Register` key, the config `[handlers."…"]` key,
  and the prefix you expect to match must be **identical**. We register, enable,
  and match all on `/telegram/channel`. A handler registered at a key config
  doesn't enable is silently dormant.
- Testing registration is easy from the package's own test binary — its `init()`
  runs, so `handler.NewResolver(... Prefix ...)` + `Resolve` exercises the real
  path without touching core `handler` tests (no global-registry pollution).

## Why Telegram needs a specialized handler

- RSSHub's `/telegram/channel` has **no real title**: `<title>` is the message
  text truncated, `<description>` is the full message HTML. Feeding the synthetic
  title as `Title:` misleads the model; raw HTML wastes tokens.
- Fix is body-first composition: flatten HTML → text, bound length, demote the
  title to a hint, and drop the hint when it's just a prefix of the body
  (prefix compare is case/space-insensitive to absorb flattening differences).

## HTML → text

- `golang.org/x/net/html`'s tokenizer is enough — no parse tree needed.
  `Tokenizer.Text()` returns **entity-decoded** bytes, so `&amp;` handling is
  free; copy them immediately (the slice is reused on the next `Next()`).
- Turn a small set of block tags (`br,p,div,li,tr,blockquote,h1-6`) into `\n`,
  then normalize line-by-line with `strings.Fields` (collapses internal
  whitespace, trims, and dropping empties removes blank-line runs in one pass).
- It was an **indirect** dep already; first direct use needs `go mod tidy` to
  promote it (a diagnostic flags this).

## Misc

- Rune-safe truncation: iterate `for i := range s` (byte index at rune starts),
  count runes, cut at `s[:i]` when the count hits the cap — never split a rune.
