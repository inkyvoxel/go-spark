# Jobs

This document explains how background work is modeled in Go Spark.

For the overall codebase shape, see [architecture.md](architecture.md).  
For local commands and workflow, see [development.md](development.md).  
For auth email delivery, see [email.md](email.md).

## The Rule

Go Spark uses two background-work patterns:

* use a periodic job for recurring housekeeping
* use a dedicated durable table plus processor for delayed or retryable domain work

This is intentional. The starter does not use a generic persisted jobs framework.

## What Runs Today

The `worker` process hosts all background jobs. Today that means:

* email outbox delivery
* database cleanup

`APP_PROCESS=all` runs the web server and the worker together.  
`APP_PROCESS=worker` runs only the background jobs worker.

## Periodic Jobs

Periodic jobs live in `internal/jobs`.

Use a periodic job when the work is recurring and safe to recompute from current state, for example:

* pruning expired data
* refreshing derived data
* maintenance tasks

Current example:

* database cleanup removes expired sessions, old tokens, and old outbox rows

## Durable Processors

Use a durable table-backed processor when the work must survive restarts and be retried safely, for example:

* sending email
* future webhook delivery
* future export generation

Current example:

* `email_outbox` stores delivery intent
* the email processor claims rows, sends them, and marks them sent or retryable

## How to Choose

Choose a periodic job when:

* the task is housekeeping
* the task runs on a schedule
* there is no per-item delivery contract to preserve

Choose a durable processor when:

* each unit of work matters individually
* work must survive restarts
* retries, claiming, or visibility matter

## Adding a New Periodic Job

Keep the pattern small:

1. add a focused job type in `internal/jobs`
2. keep persistence in `internal/database` if the job needs DB access
3. register it from `cmd/app`
4. add tests for scheduling behavior and job-specific behavior

The job should be easy to understand on its own. Avoid building an abstraction that tries to represent every future background task.

## Adding a New Durable Processor

Use the outbox pattern as the model:

1. add a domain table for pending work
2. add explicit SQL for enqueueing, claiming, and marking outcomes
3. add a focused processor package or type
4. run that processor from the worker through `internal/jobs`

That keeps durability concerns explicit in the schema instead of hiding them behind a generic queue layer.
