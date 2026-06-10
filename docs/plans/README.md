# Plans

Implementation plans. One file per plan, named `YYYY-MM-DD-<topic>-plan.md`.

A plan turns a spec (`docs/spec/`) into ordered, verifiable steps. Each step
should be small enough to implement and test on its own.

## Format

```markdown
# <Topic> Implementation Plan

**Spec:** docs/spec/<file>.md
**Lessons:** docs/lessons/<matching-file>.md
**Status:** not started | in progress | done

## Overview

One paragraph: what this plan delivers and the rough shape of the work.

## Steps

### 1. <Step name>

- **Goal:** what this step produces.
- **Changes:** files/packages touched.
- **Tests:** how it is verified (the test to write first).
- **Done when:** the observable, checkable condition.

### 2. <Step name>

...

## Open questions

Anything unresolved that may change the plan.
```

## Conventions

- Steps are ordered; later steps may depend on earlier ones.
- Prefer test-first: each step names the test that proves it.
- Mark steps `[x]` as they land; keep `Status` current.
- Record reflections in the matching `docs/lessons/` file while executing.
