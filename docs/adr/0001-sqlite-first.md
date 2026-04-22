# ADR 0001: SQLite-First Starter Architecture

## Status

Accepted

## Context

Go Spark is a starter template for new server-rendered Go web apps. The
current implementation already relies on SQLite-specific behavior in important
places:

* `sqlc` is generated for SQLite
* startup applies SQLite pragmas and connection tuning
* stores translate SQLite-specific constraint behavior
* the current outbox and claim logic is written around SQLite semantics

The project also benefits from staying small, easy to fork, and easy to run
locally with minimal infrastructure.

## Decision

Go Spark is explicitly a SQLite-first starter.

We will:

* document SQLite as the intended foundation for new projects
* keep service/store seams in `internal/services` because they protect domain
  logic boundaries
* avoid adding a top-level multi-database adapter abstraction at this stage
* treat support for another database engine as a deliberate future refactor,
  not a built-in template promise

For future package direction, SQLite engine setup and tuning should move
toward an explicit SQLite-focused package such as `internal/platform/sqlite`
or `internal/persistence/sqlite`. Domain stores should remain separate from
engine setup.

## Consequences

This keeps the starter honest about what it supports today and avoids a
premature abstraction layer that would add complexity without real user value.

It also means future refactors have a clear path:

* keep the business-layer seams
* separate engine setup from domain stores
* revisit broader database portability only when a real project requirement
  justifies it

## Revisit When

Revisit this decision only if one of these becomes true:

* the starter must actively support more than one SQL engine
* the persistence layer is split into explicit engine-specific packages
* deployment targets consistently outgrow SQLite's operating model
