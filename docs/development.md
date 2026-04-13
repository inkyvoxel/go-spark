# Development

This document covers local setup and developer workflow for the starter template.

## Requirements

* Go 1.26 or newer.
* `sqlc` for generating type-safe Go code from SQL.
* `goose` for running SQL migrations.

## Install Tools

Install `sqlc`:

```sh
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

Install `goose`:

```sh
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Make sure your Go binary directory is on `PATH`:

```sh
go env GOPATH
```

For a default Go setup, installed tools are usually available under:

```text
$(go env GOPATH)/bin
```

## Common Commands

```sh
make run
make test
make fmt
make tidy
make sqlc
make migrate-up
make migrate-down
```

## Database Workflow

The default database path is:

```text
./data/app.db
```

Run migrations:

```sh
make migrate-up
```

Roll back one migration:

```sh
make migrate-down
```

Override the database path for a command:

```sh
make migrate-up DB_PATH=/tmp/go-starter.db
```

## SQLC Workflow

SQL queries live in:

```text
internal/db/queries
```

Generated Go code is configured to go in:

```text
internal/db/generated
```

After changing SQL queries or migrations, regenerate code:

```sh
make sqlc
```

## CI

GitHub Actions runs on pushes to `main` and on pull requests. The workflow checks formatting, verifies `go mod tidy` leaves `go.mod` and `go.sum` clean, and runs the Go test suite.
