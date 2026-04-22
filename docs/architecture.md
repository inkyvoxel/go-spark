# Architecture

This document explains the shape of the codebase at a high level.

For day-to-day commands, see [development.md](development.md).  
For background work, see [jobs.md](jobs.md).  
For auth email flows, see [email.md](email.md).
For the database stance behind this starter, see
[adr/0001-sqlite-first.md](adr/0001-sqlite-first.md).

## Principles

Go Spark prefers:

* explicit code over framework magic
* standard library defaults where practical
* SQL-first data access
* SQLite-first persistence for new projects
* server-rendered UI by default
* small, focused packages with clear boundaries

## Package Boundaries

```text
/cmd/app            wires the application together
/internal/config    reads environment config
/internal/database  SQLite-backed domain stores
/internal/db        SQL queries and generated sqlc code
/internal/email     email messages, senders, and outbox processor
/internal/jobs      jobs runner and periodic background jobs
/internal/platform  engine-specific platform code such as SQLite setup
/internal/paths     canonical public URL paths
/internal/server    HTTP handlers, middleware, templates
/internal/services  business logic
```

Rules of thumb:

* handlers own HTTP concerns
* services own business logic
* stores own persistence and SQLite-specific translation today
* engine setup belongs in engine-focused packages under `internal/platform`
* templates render data, not business rules

Go Spark keeps service/store seams because they protect business logic from
HTTP and persistence concerns. Those seams are not a promise that the starter
currently supports interchangeable database backends.

## Request Flow

Most features follow this path:

1. `internal/server` receives and validates the request
2. `internal/services` applies business rules
3. `internal/database` persists through SQLite-targeted `sqlc` queries
4. the handler renders HTML or redirects

This keeps HTTP concerns, business rules, and SQLite persistence behavior
separate.

## Rendering Conventions

The app is server-rendered by default.

Important conventions:

* public paths live in `internal/paths`
* mux patterns are assembled in server routing, not duplicated as string literals
* templates use `.Routes` instead of hard-coded URLs
* template keys and fragments are centralized in the server package

## Authentication Model

The starter uses:

* email and password login
* server-side sessions stored in SQLite
* HTTP-only session cookies
* email verification
* password reset by email

It intentionally does not use JWTs or a large auth framework for the default server-rendered flow.

## Data Layer

The project is SQL-first:

* the starter's baseline SQLite schema lives in `migrations`
* queries go in `internal/db/queries`
* `sqlc` generates typed query code in `internal/db/generated`

SQLite is not just the default implementation; it is the intended foundation
for this starter. That keeps setup small, local development easy, and the
deployment story friendly to single-node and single-binary projects.

The starter does not currently aim to provide plug-and-play support for
multiple SQL engines. If a future fork needs something else, treat that as an
explicit refactor of the persistence layer.

If later phases split connection setup away from stores, the preferred
direction is:

* keep SQLite engine setup and tuning in an explicit SQLite-focused package
* keep domain stores separate from engine setup
* keep service/store seams because they support domain boundaries, not because
  they imply broad engine portability
* keep tuning defaults small and documented instead of introducing a large
  connection abstraction

Current SQLite tuning defaults in `internal/platform/sqlite` are:

* `PRAGMA foreign_keys = ON` to keep relational constraints enforced
* `PRAGMA busy_timeout = 5000` to tolerate short write contention
* `MaxOpenConns = 1` to match the starter's single-writer SQLite model

WAL mode is intentionally not enabled by default yet. If the starter adopts it
later, that should come with clear documentation about local development,
backups, and multi-process tradeoffs.

## Background Work

Background work uses two patterns:

* periodic housekeeping jobs in `internal/jobs`
* durable domain-specific processors backed by explicit tables, like `email_outbox`

The app intentionally does not have a generic persisted jobs framework. See [jobs.md](jobs.md) for the decision and extension guidance.
