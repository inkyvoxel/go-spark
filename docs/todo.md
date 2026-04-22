# TODO

This document is a curated improvement backlog for Go Spark as a starter
template. The goal is not to add every feature a production app might need.
The goal is to make the template easier to fork, safer to extend, and more
opinionated in the places where opinion reduces accidental complexity.

## Direction

The current project is already in a good place:

* package boundaries are clear
* the request -> service -> store flow is disciplined
* the background work model is intentionally small
* SQLite is already the real operational center of the starter

The next round of improvements should reinforce that shape rather than dilute
it. In practice that means:

* optimize for "fork and ship" over optional enterprise flexibility
* keep abstractions only when they support extension without hiding important
  behavior
* lean into SQLite as the first-class database
* make first-run setup feel deliberate and polished

## Recommended Order

Implement these in roughly this order so later work builds on earlier
decisions instead of fighting them:

1. Define the template opinion: SQLite-first, single-binary-friendly, easy to
   fork
2. Simplify bootstrap and schema setup around that opinion
3. Tighten persistence boundaries without pretending all databases are equal
4. Extract app composition into a clearer bootstrap shape
5. Add one-time project initialization for forked repos
6. Refresh docs after the code shape settles

## Phase 1: Lock In The Template Opinion

### 1. Make SQLite the explicit default architecture

Today the codebase says SQLite is the default, but a few docs still read like
the project is holding the door open for a generic future database story.

Improve:

* rewrite architecture and README language to say the starter is designed for
  SQLite-first projects
* describe Postgres or other databases as a deliberate fork/refactor path, not
  a built-in promise
* rename any "generic database" wording that actually means "SQLite-backed
  application state"

Why first:

* this decision affects migrations, setup UX, package naming, and how much
  adapter abstraction is worth keeping

### 2. Decide on the adapter strategy: capability seams, not fake portability

A full database adapter pattern is probably too expensive for this template.
The current code already depends on SQLite behavior in meaningful ways:

* `sqlc` is generated for SQLite
* `database.Open` enables SQLite pragmas
* stores translate SQLite-specific constraint errors
* the outbox and claim logic are written against SQLite semantics

Recommended direction:

* keep service-to-store interfaces where they protect business logic from HTTP
  and persistence details
* do not add a top-level "support any database" abstraction layer yet
* if future optionality matters, isolate it at a smaller seam:
  `internal/platform/sqlite`, migration tooling, and store implementations
* treat "swap database" as a structured future refactor, not a current product
  claim

Follow-up structure to consider:

* keep `internal/services` interfaces
* move SQLite-specific opening/tuning code out of `internal/database` into a
  more explicit package such as `internal/platform/sqlite` or
  `internal/persistence/sqlite`
* keep domain-oriented stores, but make their SQLite nature obvious in docs

Why this complements the rest:

* it preserves clean business boundaries without forcing the starter to carry a
  premature abstraction tax

## Phase 2: Simplify Schema And Bootstrap

### 3. Collapse migrations into a single baseline migration

For a starter template used for new projects, the current 5-step migration
history mostly reflects template evolution rather than user value.

Recommended change:

* replace the existing migration chain with a single baseline schema migration
  that creates the current schema from scratch
* keep the schema readable and grouped by concern:
  users/sessions, verification and reset tokens, email outbox, indexes
* regenerate `sqlc` after the baseline is in place

Benefits:

* easier to understand for new adopters
* easier to review as the canonical starter schema
* fewer confusing "historical" steps in a template that expects fresh installs

Guardrails:

* do this before advertising versioned upgrade paths
* keep column names and table semantics stable while collapsing, so tests and
  stores need minimal rework

### 4. Add a schema bootstrap path for greenfield projects

Starter users should not need to think about migration history on day one.
They should be able to create a fresh database with one obvious command.

Improve:

* add a `make setup` or `make bootstrap` command that creates local state and
  applies the baseline schema
* consider a future `go run ./cmd/app init-db` or similar subcommand for
  environments that do not use `make`
* keep `goose` for ongoing project migrations after initialization

Nice outcome:

* new users get a clean "create app, initialize DB, run app" story

## Phase 3: Strengthen Persistence Boundaries

### 5. Separate domain stores from engine setup more clearly

`internal/database` currently holds both:

* SQLite connection setup
* domain stores such as auth, cleanup, and email outbox

That works, but the package is doing two jobs.

Refactor:

* move connection/opening/pragmas into an engine-focused package
* keep domain stores in a store-focused package, or split by concern if that
  improves navigability
* document which layer owns:
  connection lifecycle, transactions, `sqlc` queries, and domain mapping

Suggested shapes:

* `internal/platform/sqlite` for connection setup and pragmas
* `internal/stores` for domain stores
* or `internal/persistence/sqlite` plus subpackages if you want the engine
  choice front-and-center

Why this matters:

* it makes the codebase easier to fork
* it preserves future optionality without pretending portability already exists

### 6. Make SQLite tuning a first-class, documented part of startup

Right now startup sets:

* `foreign_keys = ON`
* `busy_timeout = 5000`
* `max open conns = 1`

Those are sensible, but they deserve a more deliberate home.

Improve:

* centralize SQLite pragmas and connection tuning in one small type or config
* document why each setting exists
* consider whether WAL mode should be enabled by default for this starter
* consider configurable busy timeout and journal mode only if the defaults stay
  very small and understandable

Important note:

* if WAL is adopted, document the tradeoffs clearly for local development,
  backups, and multi-process access

### 7. Introduce a tiny transaction helper pattern

The stores already open explicit transactions for multi-step auth flows. That
is good. A small helper could make the pattern more consistent.

Improve:

* add one small transaction helper around `BeginTx` / `Commit` / `Rollback`
* use it only where it reduces repeated ceremony
* avoid a heavy repository or unit-of-work abstraction

Reason:

* keeps transactional flows easier to read without obscuring SQL

## Phase 4: Clean Up Application Composition

### 8. Extract composition from `cmd/app/main.go`

`cmd/app/main.go` is still manageable, but it is already the place where
config, database, services, email, jobs, and HTTP are all composed.

Refactor:

* move wiring into a bootstrap package such as `internal/app` or
  `internal/bootstrap`
* keep `main.go` focused on process entry, signal handling, and exit behavior
* expose a small `Build` or `NewApp` surface that returns the web server, job
  runner, and closers

Benefits:

* easier to extend as the starter grows
* easier to test composition decisions
* better separation between app runtime and app assembly

### 9. Move from process modes toward explicit subcommands

The `all`, `web`, and `worker` argument convention is fine, but starter
projects usually age better when CLI entrypoints are discoverable.

Future direction:

* consider `serve`, `worker`, `migrate`, and `init` subcommands
* keep `all` as a convenience mode if desired
* make one-time setup commands part of the same CLI story

This pairs well with the bootstrap and repo-initialization goals.

## Phase 5: Make Forking Feel Excellent

### 10. Build a one-time project initialization command

This is one of the highest-value improvements for a starter template.

Goal:

* a forked repo should be able to run one command and answer a few questions

Questions to support:

* project name
* Go module path
* app display name
* default email sender name/address
* default database path
* whether email verification should start enabled
* whether starter auth pages/examples should be kept

The command should update at least:

* `go.mod`
* README/project naming
* `.env.example`
* starter branding strings
* any obvious package/module references

Implementation ideas:

* `go run ./cmd/app init`
* or a dedicated `cmd/init` tool if you want to keep runtime and setup separate

Design rule:

* keep it idempotent or clearly one-time
* prefer a small declarative manifest of replacement targets over scattered
  string replacement logic

### 11. Add a "starter cleanup" mode

Many template users want auth, email, and jobs as examples, but not
necessarily every example artifact.

Consider:

* an optional init prompt to remove demo branding and starter-specific docs
* a documented list of safe-to-delete starter surfaces
* maybe a flag that trims example content while preserving the core platform

This should come after the main init command exists.

## Phase 6: Documentation And Maintenance Pass

### 12. Rewrite docs around the new first-run path

Once the code changes land, docs should reflect the intended user journey:

1. fork template
2. run init command
3. bootstrap database
4. run web app
5. start customizing features

Update:

* README quick start
* `docs/development.md`
* `docs/architecture.md`
* migration guidance
* any wording that still implies broad database support out of the box

### 13. Add an architecture decision record for the database stance

The SQLite-vs-adapter question is important enough to record explicitly.

Add a short ADR covering:

* why the starter is SQLite-first
* why service/store seams stay
* why a full multi-database adapter layer is intentionally deferred
* what future changes would justify revisiting that decision

This will reduce future drift and second-guessing.

## Lower Priority Ideas

These are reasonable, but they should wait until the main direction above is
finished:

* introduce database-backed rate limiting if in-memory limits become too
  limiting for deployed single-node apps
* embed templates/static assets into the binary for a cleaner deployment story
* split auth into a reusable module only if a real second bounded context
  appears
* add richer observability hooks only after the bootstrap and architecture
  cleanup land

## Summary Recommendation

If choosing only a few next moves, the best sequence is:

1. collapse to one baseline migration
2. make SQLite the explicit first-class database story
3. separate SQLite engine setup from domain stores
4. extract application bootstrap from `main.go`
5. build a one-time `init` command for forked repos

That sequence keeps the starter simple, internally consistent, and much closer
to the "fork, answer questions, start building" experience.
