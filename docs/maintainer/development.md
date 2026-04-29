# Maintainer Development

This guide is for working on the go-spark generator/template repository itself.

## Core Workflow

```sh
make start
make start-web
make start-worker
make build-generator
make check
```

## Validate Generator Output

```sh
go test ./internal/generator/...
go test ./...
```

The generator output contract is:

* generated app docs come from `docs/app/*`
* generated output excludes generator implementation (`cmd/go-spark`, `internal/generator`)
* generated output excludes maintainer-only docs/files (`CONTRIBUTING.md`, `CHANGELOG.md`, `docs/maintainer/*`)

## Editing Documentation

* Edit `docs/app/*` for content intended for generated applications.
* Edit `docs/maintainer/*` only for generator/template contributor guidance.
