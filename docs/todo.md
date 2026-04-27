# TODO

## Go Spark Generator Refactor

- [x] Add a separate `cmd/go-spark` generator CLI with `go-spark new <path>`.
- [x] Add `internal/generator` with `Component`, `Manifest`, `ProjectOptions`, and dependency resolution.
- [x] Embed the current full starter source as the v1 generated output.
- [x] Keep generated apps on the normal runtime CLI: `all`, `serve`, `worker`, and `migrate`.
- [x] Remove `make init` from the generated Makefile command surface.
- [x] Add generator unit tests for dependency resolution, validation, and generated output.
- [x] Split the baseline migration into component-owned migration files.
- [x] Split SQL query files by component and verify `sqlc generate` for generated feature sets.
- [x] Move from full-template copy to per-component source bundles.
- [ ] Refactor bootstrap so selected features register routes, stores, services, jobs, config, and migrations through explicit wiring.
- [ ] Add generated-project smoke tests for minimal web, web+SQLite, auth without verification, auth+password reset, and full feature output.
- [ ] Update the docs once per-component pruning is implemented so feature choices exactly match generated files.
