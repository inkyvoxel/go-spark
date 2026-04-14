# Development

This document covers local setup and developer workflow for the starter template.

## Requirements

* Go 1.26 or newer.

## Project Tools

`sqlc` and `goose` are pinned as Go tool dependencies in `go.mod`.

You do not need separate global installs for this project. The Makefile runs them through `go tool`, which uses the versions recorded by the module:

```sh
go tool sqlc version
go tool goose --version
```

To update a tool version later:

```sh
go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
go get -tool github.com/pressly/goose/v3/cmd/goose@v3.27.0
go mod tidy
```

Use a newer version in place of the examples above when intentionally upgrading.

Global installs can still be useful for ad-hoc terminal use, but this project should not rely on them:

```sh
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
```

When using global installs, make sure your Go binary directory is on `PATH`. For a default Go setup, installed tools are usually available under:

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
make migrate-status
make tools
```

## Frontend Assets

PicoCSS and HTMX are vendored in:

```text
static/vendor
```

This keeps local development independent from CDN availability and makes runtime assets visible in the repository. When intentionally upgrading a vendored asset, replace the file in `static/vendor`, verify the app in a browser, and commit the asset change with the code that depends on it.

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
make migrate-up DB_PATH=/tmp/go-spark.db
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

GitHub Actions runs on pushes to `main` and on pull requests. The workflow checks formatting, verifies `go mod tidy` leaves `go.mod` and `go.sum` clean, checks generated SQLC code, runs migrations against a temporary SQLite database, and runs the Go test suite.
