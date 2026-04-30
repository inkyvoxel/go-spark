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

## Adding a Feature

A "feature" is a `Component` defined in `DefaultManifest()` in `internal/generator/components.go`.

To add a new feature:

1. **Define the component.** Add a `Component` entry to the slice in `DefaultManifest()`. The required fields are:
   - `ID` — a kebab-case constant (also add a `Feature*` constant at the top of the file).
   - `Name` — the short human-readable name shown in the interactive prompt.
   - `Description` — one sentence shown under the name in the interactive prompt.
   - `DependsOn` — list of component IDs this feature requires (almost always includes `FeatureCore`).
   - `Files`, `Templates`, `Migrations`, `Docs`, `Tests` — source paths relative to the embedded FS.

2. **Mark hidden if appropriate.** Set `Hidden: true` only if the component should never be shown in the interactive prompt (e.g. it is always pulled in as a dependency of another component). User-facing features should leave `Hidden` at its zero value (`false`).

3. **Add source files.** Place the files the component needs in the right directories (`internal/`, `templates/`, `migrations/`, `docs/app/`, etc.) so they are picked up by the embedded FS.

4. **Update `generatedFeaturesFile()`.** If the feature needs a corresponding boolean flag in the generated `internal/features/features.go`, add it to `generatedFeaturesFile()` and `resolvedFeatureSet()` in `generator.go`.

5. **Update integration tests.** Add or extend test cases in `internal/generator/integration_test.go` to assert the new component's files are present/absent based on selection. The prompt tests in `prompt_test.go` will cover the new component automatically via `countSelectableComponents`.

That's it. `promptFeatures()` discovers selectable components from the manifest at runtime, so no additional prompt registration is needed.

## Editing Documentation

* Edit `docs/app/*` for content intended for generated applications.
* Edit `docs/maintainer/*` only for generator/template contributor guidance.
