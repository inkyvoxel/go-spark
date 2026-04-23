# TODO

This file is now a short active backlog for Go Spark. The larger starter
cleanup and initialization roadmap has mostly landed, so only genuinely
unfinished work stays here.

## Active Backlog

### 1. Consider a small transaction helper

The auth stores already handle multi-step transactions explicitly. A tiny
helper could still be worth adding if it reduces repeated `BeginTx` /
`Commit` / `Rollback` ceremony without hiding SQL behavior.

Guardrails:

* keep it small and optional
* use it only where it improves readability
* avoid introducing a heavy repository or unit-of-work abstraction

### 2. Decide whether to remove legacy CLI compatibility paths

The CLI now prefers explicit commands such as `all`, `serve`, `worker`,
`migrate`, and `init`. The old `start` compatibility path still exists to
avoid a sharp break.

Follow-up:

* decide whether `start` should stay as a long-term alias
* if it is removed, update docs, tests, and error messaging together
* keep `make` targets aligned with whichever public CLI shape wins

## Lower Priority

These are still reasonable future improvements, but they are intentionally
deferred:

* embed templates and static assets into the binary for simpler deployment
* introduce database-backed rate limiting only if the in-memory limiter stops
  being sufficient for the starter's deployment model
* split auth into a reusable module only if a real second bounded context
  appears
* add richer observability hooks only after there is a concrete operational
  need
