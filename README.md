# Go Spark

A Go web app starter template. Fork it, run `make init`, and you're off.

## What's Included

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
* email change
* email outbox delivery
* background worker and periodic cleanup jobs
* rate limiting

## Getting Started

1. Fork or clone this repository
2. Run `make init` — prompts for your project name and Go module path, rewrites relevant files, then removes itself
3. Copy the example env file and start the app:

```sh
cp .env.example .env
make migrate-up
make start
```

## Common Commands

```sh
make start          # run HTTP server + background worker
make start-web      # run HTTP server only
make start-worker   # run background worker only
make build-prod     # build a release binary
make migrate-up     # apply migrations
make migrate-status # check migration status
make test           # run tests
make check          # fmt + tidy + sqlc + vulncheck + test
```

## Read Next

* [docs/development.md](docs/development.md)
* [docs/architecture.md](docs/architecture.md)
* [docs/production.md](docs/production.md)
* [CONTRIBUTING.md](CONTRIBUTING.md)
