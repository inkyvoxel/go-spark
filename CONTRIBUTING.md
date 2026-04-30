# Contributing

Thanks for taking an interest in Go Spark.

This project is intended to stay small, explicit, and easy to adapt. Contributions should preserve that shape.

## Development Setup

```sh
cp .env.example .env
make tools
make check
```

The project pins development tools in `go.mod`, so `sqlc` and `goose` are run through `go tool` via the Makefile.

## Before Opening a Pull Request

Run:

```sh
make check
```

If you change migrations, also run:

```sh
make migrate-up
make migrate-status
```

## Guidelines

* Prefer standard library APIs where they fit.
* Keep handlers focused on HTTP request and response concerns.
* Put business logic in services rather than templates or handlers.
* Keep SQL explicit and readable.
* Add focused tests for behavior that can regress.
* Avoid broad refactors unless they directly support the change.

## Generated Code

SQLC output in `internal/db/generated` is generated from migrations and query files. After changing SQL, run:

```sh
make sqlc
```

Commit generated changes when they are expected.

## Security

Do not open public issues with secrets, private keys, tokens, production database files, or sensitive user data.

See [SECURITY.md](SECURITY.md) for vulnerability reporting guidance.
