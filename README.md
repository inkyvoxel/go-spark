# Go Spark

A small starter template for server-rendered Go web applications.

The default shape is intentionally simple: Go, `net/http`, `html/template`, PicoCSS, HTMX where it helps, SQLite, SQL migrations, and a small set of conventions that are easy to change later.

It includes a runnable app, SQLite setup, migrations, generated SQL code, CSRF protection, and basic email/password session authentication.

Frontend assets are vendored under `static/vendor` so the starter works without runtime CDN dependencies.

## Tech Stack

* Go
* `net/http`
* `html/template`
* PicoCSS
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
make migrate-up
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
make migrate-status
make tools
```

`sqlc` and `goose` are pinned as Go tool dependencies in `go.mod`, so the Makefile runs them through `go tool`. You do not need separate global installs.

See [docs/development.md](docs/development.md) for tool and workflow details.

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening pull requests, and [CHANGELOG.md](CHANGELOG.md) for notable changes.

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
/docs               architecture and development notes
.github/workflows   CI configuration
.github             issue and pull request templates
```

## After Cloning

If you use this as a template for a new project:

- [ ] Rename the module in `go.mod`.
- [ ] Update the app name in README, templates, and config where needed.
- [ ] Copy `.env.example` to `.env`.
- [ ] Review `DATABASE_PATH`.
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
* Set `APP_COOKIE_SECURE=true` when the app is served over HTTPS by a reverse proxy or load balancer.
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
