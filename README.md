# Go Starter

A small starter template for server-rendered Go web applications.

The default shape is intentionally simple: Go, `net/http`, `html/template`, HTMX where it helps, SQLite, SQL migrations, and a small set of conventions that are easy to change later.

## Current Status

This template is in early development. The first runnable scaffold is in place, and the outstanding work is tracked in [docs/roadmap.md](docs/roadmap.md).

Implemented so far:

* Go module and runnable app entrypoint.
* Environment-based config.
* SQLite database opener.
* Static file serving.
* Home route and `/healthz`.
* Base template and stylesheet.
* Initial migration file.
* `sqlc` configuration and example query.
* MIT license placeholder.

Still pending:

* Generated `sqlc` code.
* Local `goose` migration run.
* Authentication implementation.
* Final module path and license copyright details.
* GitHub-ready release polish.

## Tech Stack

* Go
* `net/http`
* `html/template`
* HTMX
* SQLite via `modernc.org/sqlite`
* `database/sql`
* `sqlc`
* `goose`
* `log/slog`

See [docs/architecture.md](docs/architecture.md) for the longer rationale and planned authentication approach.

## Quick Start

```sh
cp .env.example .env
make run
```

The app listens on `:8080` by default and stores its SQLite database at `./data/app.db`.

Open:

```text
http://localhost:8080
http://localhost:8080/healthz
```

## Commands

```sh
make run
make test
make fmt
make tidy
make sqlc
make migrate-up
make migrate-down
```

`make sqlc`, `make migrate-up`, and `make migrate-down` require `sqlc` and `goose` to be installed locally.

See [docs/development.md](docs/development.md) for local tool installation and workflow details.

## Project Layout

```text
/cmd/app            application entrypoint
/internal/config    environment config
/internal/database  SQLite connection setup
/internal/db        SQL queries and generated sqlc package
/internal/server    HTTP routes and handlers
/migrations         goose migrations
/templates          server-rendered HTML
/static             CSS and static assets
/docs               roadmap and architecture notes
.github/workflows   CI configuration
```

## After Cloning

If you use this as a template for a new project:

- [ ] Rename the module in `go.mod`.
- [ ] Update the app name in README, templates, and config where needed.
- [ ] Copy `.env.example` to `.env`.
- [ ] Review `DATABASE_PATH`.
- [ ] Install `sqlc` and `goose`.
- [ ] Run migrations.
- [ ] Replace or remove example routes and templates.
- [ ] Update the copyright holder in `LICENSE`.
- [ ] Review production settings before deployment.

## Production Notes

Before deploying an app based on this template:

* Serve over HTTPS.
* Use secure, HTTP-only cookies for session tokens.
* Keep secrets out of Git and load them from environment or your deployment platform.
* Back up the SQLite database file.
* Run migrations as part of deploy or a controlled release step.
* Set file permissions so the app can read and write the database path, but does not expose it publicly.
* Add request timeouts and deployment-specific logging/metrics as the app grows.

## Replacing SQLite Later

SQLite is a good default for a simple starter because it keeps local development and deployment small. If a future project needs multiple app instances, heavier write concurrency, or managed database operations, Postgres is the natural next step.

The intended migration path is:

1. Add a Postgres driver.
2. Update database config.
3. Update `sqlc.yaml` to use the Postgres engine.
4. Port migrations and SQL queries.
5. Run the test suite against the new database.

Keeping database access behind explicit SQL and small service boundaries should make that move easier.

## Removing Example Code

As the starter grows, example code should stay easy to identify and delete. For a new project, start by replacing the home template, removing sample routes that do not fit, and adding the first real model, migration, query, service, and handler as a thin vertical slice.

## Roadmap

See [docs/roadmap.md](docs/roadmap.md).
