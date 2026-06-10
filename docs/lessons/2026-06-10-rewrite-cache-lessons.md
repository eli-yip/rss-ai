# Title Rewrite + Cache — Lessons

Reflections from filling the handled seam: surgical etree title rewrite (spec
§5.1), the title cache (§5.3/§6), and the concurrency trio + timeout fallback
(§7). Append terse entries during execution; consolidate into themed sections
after the plan is done.

<!-- Append entries below as you execute (format: docs/lessons/README.md). -->

## Step 1 — config durations

- `time.Duration` has no `TextUnmarshaler`, so go-toml can't decode `"30s"`. Keep
  the TOML string field, add a `toml:"-"` Duration sibling, parse once in `Init`
  (fail fast). Defaults (`"30s"`/`"10s"`) parse cleanly.

## Step 2 — store update-or-insert

- GORM's `First` logs `record not found` at error level when the row is absent —
  noisy. Use `Limit(1).Find` + `RowsAffected == 0` to branch insert-vs-update
  without the spurious log line.
- `Updates(map[string]any{...})` auto-bumps `updated_at`; `Create` auto-fills both
  timestamps. No need to set them by hand.
- No local Postgres in the env, so spin a throwaway `postgres:16-alpine` on a
  high port and point `RSS_AI_TEST_DSN` at it to actually exercise the DB methods.

## Step 3 — etree fidelity

- etree v1.6.0 round-trips byte-for-byte with `ReadSettings{Permissive:true,
  PreserveCData:true}` and default write settings: XML declaration, self-closing
  `<enclosure .../>`, CDATA, and namespaced `<content:encoded>` all survive
  verbatim. No WriteSettings tuning needed.
- `SelectElement("content:encoded")` matches the namespace prefix directly (tag
  may include `prefix:`), so no manual child iteration for namespaced bodies.
- **Test surgical edits against the no-op round-trip baseline, not the raw input.**
  `want := strings.Replace(baseline, oldTitle, newTitle, 1)` asserts "only the
  title changed" independent of any formatting quirk; a separate identity test
  pins baseline == input for hand-written fixtures.
- Preserve title CDATA-vs-text wrapping on rewrite (`SetCData` if the original
  title's CharData `IsCData()`, else `SetText`) to minimize the diff.
