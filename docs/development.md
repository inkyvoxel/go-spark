# Development

This is the best place to start when onboarding to the codebase.

For system design and package boundaries, see [architecture.md](architecture.md).  
For background work patterns, see [jobs.md](jobs.md).  
For auth email flows, see [email.md](email.md).
For production deployment guidance, see [production.md](production.md).

## Requirements

* Go 1.26 or newer

## First Run

```sh
go run ./cmd/go-spark new ../my-app \
  -project-name "My App" \
  -module-path github.com/me/my-app \
  -yes
cd ../my-app
cp .env.example .env
make migrate-up
make start
```

This starter assumes SQLite as the normal development and first-run path. The
default database file is `./data/app.db`.

`go-spark new` is the intended first step for a new app. It writes a plain Go project with the module path, app branding, default email sender, default SQLite path, and selected feature set already applied.

Feature selection is component-based. See [generated-features.md](generated-features.md) for the current mapping between selected features and generated docs, templates, migrations, and runtime behavior.

The app loads `.env` when present. Existing shell environment variables still win.
Use `LOG_FORMAT=text` locally by default; switch to `LOG_FORMAT=json` when you want machine-parseable logs during development.

## Common Commands

```sh
make start
make start-web
make start-worker
make build-generator
make build-prod
make migrate-status
make migrate-up
make migrate-down
make test
make check
make sqlc
```

Notes:

* `make migrate-up` creates the local SQLite path and applies the schema migrations.
* `make start` starts the HTTP server and background jobs worker together.
* `make start-web` starts only the HTTP server.
* `make start-worker` starts only the background jobs worker.
* `make build-prod` builds a release-style binary for deployment targets.
* `make migrate-up`, `make migrate-down`, and `make migrate-status` run through the app CLI so migrations share the same command surface as production.
* `make check` runs formatting, module tidy, sqlc generation, vulncheck, and tests.
* The app CLI uses explicit runtime commands: `all`, `serve`, `worker`, and `migrate`.
* The generator CLI uses `go-spark new <path>` for one-time project creation.
* Generated feature flags live in `internal/features/features.go` in the created app.

## Tooling

`sqlc`, `goose`, and `govulncheck` are pinned as Go tools in `go.mod`.
This intentionally adds a larger set of indirect dependencies to `go.mod`, because a single module keeps template setup and upgrades straightforward. Do not split tools into a separate module unless that tradeoff becomes painful for real template users.

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
* Unmatched `GET` and `HEAD` requests render `templates/404.html`.

If you add a new route, update `internal/paths` first and use those constants from handlers, templates, emails, and tests.

### Database changes

The template is SQLite-first today.

* Migrations live in `migrations`.
* Schema migrations are split by feature ownership so generator components can eventually copy only the migrations they need.
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
* `EMAIL_FROM`
* optionally `SMTP_USERNAME` and `SMTP_PASSWORD`
