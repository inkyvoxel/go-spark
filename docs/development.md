# Development

This is the best place to start when onboarding to the codebase.

For system design and package boundaries, see [architecture.md](architecture.md).  
For background work patterns, see [jobs.md](jobs.md).  
For auth email flows, see [email.md](email.md).

## Requirements

* Go 1.26 or newer

## First Run

```sh
cp .env.example .env
make setup
make run
```

This starter assumes SQLite as the normal development and first-run path. The
default database file is `./data/app.db`.

The app loads `.env` when present. Existing shell environment variables still win.

## Common Commands

```sh
make run
make run-web
make run-worker
make setup
make migrate-up
make migrate-down
make test
make check
make sqlc
```

Notes:

* `make setup` creates the local SQLite path and applies the baseline schema.
* `make run` starts the HTTP server and background jobs worker together.
* `make run-web` starts only the HTTP server.
* `make run-worker` starts only the background jobs worker.
* `make check` runs formatting, module tidy, sqlc generation, vulncheck, and tests.

## Tooling

`sqlc`, `goose`, and `govulncheck` are pinned as Go tools in `go.mod`.

Useful commands:

```sh
go tool sqlc version
go tool goose --version
go tool govulncheck -h
```

## Daily Workflow

Typical change flow:

1. make the code change
2. run `make sqlc` if you changed SQL
3. run `make test`
4. run `make check` before opening a PR

## Important Conventions

### Routes and templates

* Public URL paths live in `internal/paths`.
* Templates receive route helpers through `.Routes`.
* Template keys and fragment names live in `internal/server/template_constants.go`.

If you add a new route, update `internal/paths` first and use those constants from handlers, templates, emails, and tests.

### Database changes

The template is SQLite-first today.

* Migrations live in `migrations`.
* The template currently ships with one baseline schema migration for fresh projects.
* SQL queries live in `internal/db/queries`.
* Generated code lives in `internal/db/generated`.
* SQLite connection setup now lives under `internal/platform/sqlite`.
* SQLite-backed domain stores live in `internal/database`.
* The default SQLite tuning is:
  `foreign_keys = ON`, `busy_timeout = 5000`, and `MaxOpenConns = 1`.

If you change schema or queries, regenerate with:

```sh
make sqlc
```

### Background work

The `worker` process hosts all background jobs. Today that includes:

* email outbox delivery
* SQLite-backed data cleanup

When adding new background behavior, follow [jobs.md](jobs.md) instead of inventing a new pattern.

## Local Email

Keep `EMAIL_PROVIDER=log` during local development.

To send real email, switch to `EMAIL_PROVIDER=smtp` and set:

* `SMTP_HOST`
* `SMTP_PORT`
* `SMTP_FROM`
* optionally `SMTP_USERNAME` and `SMTP_PASSWORD`
