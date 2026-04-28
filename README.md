# Go Spark

Go Spark is a Go web app generator and starter-template.

Generated apps include:

* server-rendered HTML with `html/template`
* SQLite with `database/sql`
* SQL-first data access with `sqlc`
* SQL migrations with `goose`
* structured logging with `log/slog`
* email/password authentication
* server-side sessions
* CSRF protection
* email verification
* password reset
* email outbox delivery
* background worker and periodic cleanup jobs

This repository is for maintainers of the generator/template itself. Developers
creating a new app should use `go-spark new <path>` to scaffold a dedicated app
repository that does not include generator implementation code.

## Maintainer Workflow

```sh
make start
make start-web
make start-worker
make build-generator
make check
```

Generate a sample app locally:

```sh
go run ./cmd/go-spark new ../my-app \
  -project-name "My App" \
  -module-path github.com/me/my-app \
  -yes
```

## Repository Surfaces

* `cmd/go-spark` and `internal/generator`: generation workflow and component manifest
* runtime app template source: `cmd/app`, `internal/*` runtime packages, `templates`, `static`, `migrations`
* `docs/maintainer`: maintainer-oriented project guidance
* `docs/app`: documentation source copied into generated apps

## Generation Contract

`go-spark new` produces a standalone application repository:

* excludes generator implementation (`cmd/go-spark`, `internal/generator`)
* excludes maintainer-only docs/files (`CONTRIBUTING.md`, `CHANGELOG.md`, `docs/todo.md`)
* writes app-focused docs and README for the generated project

## Read Next

* [docs/development.md](docs/development.md)
* [docs/architecture.md](docs/architecture.md)
* [docs/generated-features.md](docs/generated-features.md)
* [docs/maintainer/README.md](docs/maintainer/README.md)
* [CONTRIBUTING.md](CONTRIBUTING.md)
