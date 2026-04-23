# Go Spark

A small SQLite-first starter template for server-rendered Go web
applications.

Go Spark keeps the default shape intentionally simple:

* Go with `net/http`
* server-rendered HTML with `html/template`
* SQLite with `database/sql`
* SQL-first data access with `sqlc`
* SQL migrations with `goose`
* structured logging with `log/slog`

It includes a runnable app, basic auth flows, transactional email, and a
small background jobs worker.

The template is designed for new projects that want a solid SQLite
foundation, minimal infrastructure, and an easy path to running as a single
binary. It does not currently promise plug-and-play support for multiple
database engines. If a future fork outgrows SQLite, treat that as an explicit
refactor rather than a built-in template feature.

## Quick Start

```sh
make init
cp .env.example .env
make setup
make start
```

Open `http://localhost:8080`.

The normal first-run path uses the SQLite database at `./data/app.db`.
If you are using this repo as a template, run `make init` first to rename the module, app branding, default database path, and other starter defaults.

The `init` command prompts for:

* project name
* Go module path
* app display name
* default email sender name and address
* default SQLite database path
* whether email verification should be enabled by default

If you want a non-interactive setup, pass flags such as:

```sh
go run ./cmd/app init \
  -project-name "Acme Starter" \
  -module-path github.com/acme/acme-starter \
  -app-name "Acme Portal" \
  -database-path ./var/acme.db
```

## Process Commands

The CLI uses explicit subcommands:

```sh
./go-spark all
./go-spark serve
./go-spark worker
./go-spark migrate status
```

`APP_PROCESS` still exists as an environment-level override, but the preferred
entrypoints are the explicit CLI commands.

## What’s Included

* email/password authentication
* server-side sessions
* CSRF protection
* account email verification
* password reset
* email outbox delivery
* periodic SQLite-backed cleanup jobs

Email delivery defaults to `EMAIL_PROVIDER=log` for safe local development.

## Commands

```sh
make init
make start
make start-web
make start-worker
make setup
make migrate-up
make test
make check
```

Use `make check` before opening a pull request.

## Read Next

Start here if you are new to the project:

* [docs/development.md](docs/development.md) for setup and day-to-day workflow
* [docs/architecture.md](docs/architecture.md) for the codebase shape and conventions
* [docs/jobs.md](docs/jobs.md) for background work patterns
* [docs/email.md](docs/email.md) for auth email flows and outbox design

Reference docs:

* [CONTRIBUTING.md](CONTRIBUTING.md)
* [SECURITY.md](SECURITY.md)
* [docs/todo.md](docs/todo.md)

## Project Layout

```text
/cmd/app            application entrypoint
/internal/config    environment config
/internal/database  SQLite-backed domain stores
/internal/db        SQL queries and generated sqlc package
/internal/email     email messages, senders, and outbox processor
/internal/jobs      background jobs runner and jobs
/internal/platform  engine-specific platform code such as SQLite setup
/internal/paths     canonical public URL paths
/internal/server    HTTP routes and handlers
/internal/services  business logic
/migrations         goose SQL migrations
/templates          server-rendered HTML templates
/static             CSS and static assets
/docs               onboarding and design notes
```

## Template Checklist

If you use this as a template for a new project:

* rename the module in `go.mod`
* copy `.env.example` to `.env`
* initialize the local SQLite database with `make setup`
* replace or remove example routes, templates, and branding
* keep new public paths in `internal/paths`
* assume SQLite is the intended foundation unless you are deliberately
  refactoring the persistence layer
* review production settings before deployment
