# To-do

Working list toward a v1 feature-complete release. Tackle top to bottom.
Each item notes the why and the main files to touch.

## Email/jobs

Context: email sending already uses the transactional outbox pattern
(enqueue inside the business txn, separate worker polls + sends with
retries/lease). This is the correct design. Do NOT move to per-request
goroutines or per-email processes; that loses durability, atomicity, and
retries. The list below is refinement, not a rewrite.

- [ ] Tidy `jobs.Runner.Run`: replace manual `wg.Add(1)`/`go func(job Job)`/
      `defer wg.Done()` with `wg.Go(func() { ... })` (Go 1.25+)
- [ ] Drop the explicit `func(job Job)` loop-var capture in the runner
      (per-iteration loop vars since Go 1.22)
- [ ] Review `emailJobInterval`: shorten to a few seconds so auth mail
      (verify/reset) sends promptly. SQLite + index on
      `(status, available_at)` makes frequent polling cheap. Skip building a
      cross-process notify.
- [ ] Future jobs decision: recurring work -> interval Runner; one-shot
      durable work (PDF, webhook, upload) -> generalise the OUTBOX, not the
      ticker runner. (River is the Go reference but it's Postgres-only.)

## Write the CHANGELOG before tagging v1

`CHANGELOG.md` is still all "Unreleased / Initial scaffold". Fill in a real v1
entry before shipping a forkable release.

- Files: `CHANGELOG.md`.
