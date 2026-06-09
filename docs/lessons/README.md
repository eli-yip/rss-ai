# Lessons

Reflections captured while executing a plan. One file per plan, named to match
the plan: `YYYY-MM-DD-<topic>-lessons.md`.

## Workflow

1. **During execution — append only.** When something surprises you, breaks, or
   teaches you a non-obvious fact, append a short entry. Do **not** rewrite or
   reorganize earlier entries while working; appending is cheap and keeps token
   use low.
2. **After the plan is done — consolidate.** Do one pass: dedupe, group related
   entries, drop noise, and rewrite into a clean organized form.

## Append-entry format

While executing, keep each entry terse:

```markdown
## <step or date> — <one-line title>

- What happened / what I expected.
- Root cause or insight.
- What to do differently / the rule to remember.
```

## Consolidated format (after completion)

```markdown
# <Topic> — Lessons

## <Theme, e.g. Concurrency / XML / Config>

- Distilled lesson, stated as a reusable rule.
- ...
```
