# Go Spark

A small SQLite-first starter and project generator for server-rendered Go web
applications.

Go Spark keeps the default shape intentionally simple:

* Go with `net/http`
* server-rendered HTML with `html/template`
* SQLite with `database/sql`
* SQL-first data access with `sqlc`
* SQL migrations with `goose`
* structured logging with `log/slog`

It includes a runnable app, basic auth flows, transactional email, a small
background jobs worker, and a `go-spark new` generator for creating fresh projects.

The template is designed for new projects that want a solid SQLite
foundation, minimal infrastructure, and an easy path to running as a single
binary. It does not currently promise plug-and-play support for multiple
database engines. If a future fork outgrows SQLite, treat that as an explicit
refactor rather than a built-in template feature.

## Generate A Project

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

Open `http://localhost:8080`.

The generator is one-time scaffolding. Generated projects are plain Go apps and do not depend on the generator at runtime.

The `new` command prompts for:

* project name
* Go module path
* default SQLite database path
* default email sender
* feature selection

If you want a non-interactive setup, pass flags such as:

```sh
go run ./cmd/go-spark new ../acme-starter \
  -project-name "Acme Starter" \
  -module-path github.com/acme/acme-starter \
  -database-path ./var/acme.db \
  -email-from "Acme <team@acme.test>" \
  -features all \
  -yes
```

Feature dependency resolution and component-owned source bundles are implemented now. Some partial feature sets still need the follow-up bootstrap refactor before they compile as standalone runtime apps.

## Process Commands

The CLI uses explicit subcommands:

```sh
./my-app all
./my-app serve
./my-app worker
./my-app migrate status
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
make start
make start-web
make start-worker
make build-generator
make build-prod
make check
make test
make fmt
make tidy
make sqlc
make vulncheck
make migrate-up
make migrate-down
make migrate-status
make tools
```

Use `make check` before opening a pull request.

## Read Next

Start here if you are new to the project:

* [docs/development.md](docs/development.md) for setup and day-to-day workflow
* [docs/production.md](docs/production.md) for production build and deployment guidance
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
/internal/app       application bootstrap and runtime assembly
/internal/config    environment config
/internal/database  SQLite-backed domain stores
/internal/db        SQL queries and generated sqlc package
/internal/email     email messages, senders, and outbox processor
/internal/jobs      background jobs runner and jobs
/internal/platform  engine-specific platform code such as SQLite setup
/internal/paths     canonical public URL paths
/internal/generator project generation workflow
/internal/server    HTTP routes and handlers
/internal/services  business logic
/migrations         goose SQL migrations
/templates          server-rendered HTML templates
/static             CSS and static assets
/docs               onboarding and design notes
```

## Template Checklist

If you use this as a template for a new project:

* run `go-spark new` to set project name, Go module path, default sender, and default database path across starter files
* copy `.env.example` to `.env`
* initialize the local SQLite database with `make migrate-up`
* replace or remove example routes, templates, and branding
* keep new public paths in `internal/paths`
* assume SQLite is the intended foundation unless you are deliberately
  refactoring the persistence layer
* review production settings before deployment
