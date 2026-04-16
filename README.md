# Go Spark

A small starter template for server-rendered Go web applications.

The default shape is intentionally simple: Go, `net/http`, `html/template`, PicoCSS, HTMX where it helps, SQLite, SQL migrations, and a small set of conventions that are easy to change later.

It includes a runnable app, SQLite setup, migrations, generated SQL code, CSRF protection, and email/password session authentication with basic transactional email support.

Frontend assets are vendored under `static/vendor` so the starter works without runtime CDN dependencies.

Auth business logic is kept behind a small storage interface. The default storage adapter uses SQLite and `sqlc`, but database-specific driver behavior stays out of services and HTTP handlers.

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

See [docs/architecture.md](docs/architecture.md) for the longer rationale and authentication approach.

## Quick Start

```sh
cp .env.example .env
make migrate-up
make run
```

The app listens on `:8080` by default, stores its SQLite database at `./data/app.db`, and requires registration passwords to be at least 12 characters unless `AUTH_PASSWORD_MIN_LENGTH` is set.

`make run` loads `.env` when the file exists. Environment variables already set in your shell take precedence over values in `.env`.

Email delivery defaults to `EMAIL_PROVIDER=log` for safe local development. Set `EMAIL_PROVIDER=smtp` with `SMTP_*` values to send real mail.

Built-in email functionality includes:

* Account confirmation emails on registration.
* Confirmation links at `/confirm-email`.
* Resend confirmation from the account page for signed-in, unverified users.
* Durable email delivery intent via a database outbox worker.

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

See [docs/development.md](docs/development.md) for tool and workflow details. See [docs/email.md](docs/email.md) for the staged email and account-confirmation plan.

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening pull requests, and [CHANGELOG.md](CHANGELOG.md) for notable changes.

## Project Layout

```text
/cmd/app            application entrypoint
/internal/config    environment config
/internal/database  SQLite connection setup
/internal/db        SQL queries and generated sqlc package
/internal/server    HTTP routes and handlers
/internal/services  business logic
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
* Keep `AUTH_PASSWORD_MIN_LENGTH` at 12 or higher unless you have a clear compatibility reason.
* Add request timeouts and deployment-specific logging/metrics as the app grows.

## Replacing SQLite Later

SQLite is a good default for a simple starter because it keeps local development and deployment small. If a future project needs multiple app instances, heavier write concurrency, or managed database operations, Postgres is the natural next step.

The intended migration path is:

1. Add a Postgres driver.
2. Add or switch the database opener and config.
3. Port migrations and SQL queries.
4. Update `sqlc.yaml` to use the Postgres engine and regenerate generated code.
5. Implement storage adapters that satisfy the service-owned interfaces, such as the auth store.
6. Translate Postgres driver errors, such as unique violations, inside those adapters.
7. Run adapter, service, route, and migration tests against the new database.

Services own business rules, while storage adapters own SQL and database-driver details. Keeping that boundary small should make a database switch possible without rewriting HTTP handlers or auth business logic.

## Removing Example Code

As the starter grows, example code should stay easy to identify and delete. For a new project, start by replacing the home template, removing sample routes that do not fit, and adding the first real model, migration, query, service, and handler as a thin vertical slice.
