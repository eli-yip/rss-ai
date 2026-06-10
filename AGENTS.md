# AGENTS.md

Guidance for AI agents working in this repository. `CLAUDE.md` is a symlink to this file.

## Project

`rss-ai` is a gateway placed in front of a single RSSHub instance. RSSHub item
titles are sometimes hard to read; for **registered** path prefixes the gateway
rewrites titles with an LLM, caches the result, and returns a surgically-modified
feed. Unregistered paths are reverse-proxied through unchanged and uncached.

**Stack:** Go, Echo/v5, GORM + PostgreSQL, `beevik/etree` (XML), `x/sync/singleflight`,
`x/time/rate`. No Redis. No `gorilla/feeds`.

The authoritative design is the spec — read it before implementing.

## Documentation Layout

| Path               | Purpose                                              |
| ------------------ | ---------------------------------------------------- |
| `docs/spec/`       | Design specs (source of truth for _what_ and _why_). |
| `docs/plans/`      | Implementation plans (the _how_, step by step).      |
| `docs/lessons/`    | Reflections captured during plan execution.          |
| `docs/PROGRESS.md` | Current overall progress. Keep it up to date.        |
| `goal.md`          | The original one-paragraph idea. Historical.         |

### Working rules

- **Read `docs/PROGRESS.md` first** at the start of any work session to learn
  where things stand. Update it as milestones land.
- **Specs** live in `docs/spec/`, named `YYYY-MM-DD-<topic>-design.md`.
- **Plans** live in `docs/plans/`, named `YYYY-MM-DD-<topic>-plan.md`. Format in
  `docs/plans/README.md`.
- **Lessons** live in `docs/lessons/`, one file per plan, named to match the plan
  (`YYYY-MM-DD-<topic>-lessons.md`). Format in `docs/lessons/README.md`.

### Lessons capture (important)

During plan execution, **append** reflections to the matching lessons file as you
go — append-only, terse, no rewriting earlier entries (this saves tokens while
working). **After the plan is complete**, do a single consolidation pass: dedupe,
group, and rewrite the lessons file into a clean, organized form.

## Commits

Use **Conventional Commits** (`<type>(<scope>): <subject>`), e.g.
`feat(gateway): add longest-prefix handler resolution`,
`fix(cache): re-rewrite when upstream title changes`,
`docs(spec): clarify json passthrough`. Common types: `feat`, `fix`, `docs`,
`refactor`, `test`, `chore`, `build`, `ci`.

## Language

- **Repository files** (docs, code, comments, commit messages) are written in
  **English**.
- **Claude Code conversations** with the user are conducted in **Chinese**.
