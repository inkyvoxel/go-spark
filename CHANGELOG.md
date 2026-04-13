# Changelog

All notable changes to this project will be documented in this file.

This project follows a simple, human-written changelog format. Versioning will become more formal before the starter is published as a public template.

## Unreleased

### Added

* Initial server-rendered Go application scaffold.
* SQLite connection setup using `database/sql` and `modernc.org/sqlite`.
* Goose migration for users and sessions.
* SQLC configuration and generated starter query package.
* Project-pinned `sqlc` and `goose` tool dependencies.
* Basic home page, static CSS, and `/healthz`.
* Focused tests for config, database opening, and routes.
* Auth service foundation with bcrypt password hashing and database-backed sessions.
* SQLC auth queries for users and sessions.
* Session middleware and current-user request context helpers.
* GitHub Actions test workflow.
* Template README, architecture notes, development guide, and roadmap.
