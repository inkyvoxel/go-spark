# Development

This document covers local setup and developer workflow for the starter template.

## Requirements

* Go 1.26 or newer.

## Project Tools

`sqlc`, `goose`, and `govulncheck` are pinned as Go tool dependencies in `go.mod`.

You do not need separate global installs for this project. The Makefile runs them through `go tool`, which uses the versions recorded by the module:

```sh
go tool sqlc version
go tool goose --version
go tool govulncheck -h
```

To update a tool version later:

```sh
go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
go get -tool github.com/pressly/goose/v3/cmd/goose@v3.27.0
go get -tool golang.org/x/vuln/cmd/govulncheck@v1.2.0
go mod tidy
```

Use a newer version in place of the examples above when intentionally upgrading.

Global installs can still be useful for ad-hoc terminal use, but this project should not rely on them:

```sh
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

When using global installs, make sure your Go binary directory is on `PATH`. For a default Go setup, installed tools are usually available under:

```text
$(go env GOPATH)/bin
```

## Common Commands

```sh
make run
make check
make run-all
make run-web
make run-worker
make fmt
make tidy
make sqlc
make vulncheck
make test
make migrate-up
make migrate-down
make migrate-status
make tools
```

`make run` starts the app with `go run ./cmd/app`. On startup, the app loads `.env` when the file exists. Existing shell environment variables take precedence over `.env` values.

`make check` is the default pre-PR verification command. It runs `make fmt`, `make tidy`, `make sqlc`, `make vulncheck`, and `make test` in order.

The app binary supports process modes:

* `make run` starts the default `all` mode, which runs the HTTP server and email worker together.
* `make run-all` starts the same all-in-one mode explicitly.
* `make run-web` starts only the HTTP server.
* `make run-worker` starts only the email outbox worker.

For deployed environments, set `APP_PROCESS=web` and `APP_PROCESS=worker` in separate process manager entries, or pass `web` or `worker` as the first binary argument.

This starter includes basic transactional email out of the box for account confirmation, resend-verification, and password reset flows.

For email, keep `EMAIL_PROVIDER=log` during local development. To send real mail, set `EMAIL_PROVIDER=smtp` and provide `SMTP_HOST`, `SMTP_PORT`, and `SMTP_FROM` (plus `SMTP_USERNAME` and `SMTP_PASSWORD` when your server requires authentication).

## Routes and Templates

Public URL paths live in `internal/paths`. When adding or changing routes, update the canonical path constants first, then use those constants from handlers, middleware, emails, and tests.

Server route registration should compose mux patterns from HTTP methods and path constants:

```go
dynamic.Handle(route(http.MethodGet, paths.Account), s.requireVerifiedAuth(http.HandlerFunc(s.account)))
```

Avoid creating extra constants that only mirror `paths.*` values. The small `route(method, path)` helper is the intended place where `net/http` method/path patterns are assembled.

HTML templates receive route helpers through `.Routes`, which is populated from `paths.TemplateRoutes`. Template links and form actions should use those helpers instead of inline literals:

```html
<a href="{{ .Routes.Login }}">Sign in</a>
<form method="post" action="{{ .Routes.ResetPassword }}">
```

Template keys and fragment names live in `internal/server/template_constants.go`. Use those constants in render calls and tests to avoid drift between handlers and template files.

## Frontend Assets

PicoCSS is vendored in:

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
