# Roadmap

This document tracks the starter-template work so future sessions can pick up the thread quickly.

## Phase 1: Starter Foundation

- [x] Add Go module.
- [x] Add project ignore rules.
- [x] Add MIT license placeholder.
- [x] Add environment example.
- [x] Add Makefile workflow commands.
- [x] Add minimal server entrypoint.
- [x] Add config loading from environment.
- [x] Add SQLite database opener.
- [x] Add home route and health check.
- [x] Add base template and stylesheet.
- [x] Add initial migration file.
- [x] Add sqlc configuration and example query.
- [ ] Generate sqlc code after `sqlc` is installed.
- [ ] Run migrations after `goose` is installed.
- [ ] Decide final module path before publishing.
- [x] Initialize Git repository.
- [ ] Update `LICENSE` with the correct copyright holder.

## Phase 2: Template Documentation

- [x] Rewrite `README.md` for template users.
- [x] Move detailed architecture notes into `docs/architecture.md`.
- [x] Add "After Cloning" checklist.
- [x] Add production notes for cookies, HTTPS, backups, migrations, and secrets.
- [x] Add a short section on replacing SQLite with Postgres later.
- [x] Add guidance for removing or replacing example code.

## Phase 3: Authentication Slice

- [ ] Add user registration route.
- [ ] Add login route.
- [ ] Add logout route.
- [ ] Add bcrypt password hashing.
- [ ] Add session creation and deletion.
- [ ] Add session middleware.
- [ ] Add request context helper for current user.
- [ ] Add CSRF token generation and validation.
- [ ] Add authenticated example page.
- [ ] Add tests for auth services and session handling.

## Phase 4: Developer Experience

- [x] Add focused tests for config, database opening, and route behavior.
- [ ] Add GitHub Actions for formatting and tests.
- [ ] Add `CONTRIBUTING.md`.
- [ ] Add `CHANGELOG.md`.
- [ ] Add issue and pull request templates.
- [ ] Document local installation for `sqlc` and `goose`.

## Phase 5: Release Preparation

- [ ] Confirm repository name.
- [ ] Confirm package/module path.
- [ ] Run full test suite from a clean clone.
- [ ] Check generated files are committed or documented.
- [ ] Review README from a first-time user perspective.
- [ ] Publish under MIT license on GitHub.
