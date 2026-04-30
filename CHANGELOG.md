# Changelog

All notable changes to this project will be documented in this file.

This project follows a simple, human-written changelog format.

## Unreleased

### Added

* Initial server-rendered Go application scaffold.
* SQLite connection setup using `database/sql` and `modernc.org/sqlite`.
* Goose migration for users and sessions.
* SQLC configuration and generated starter query package.
* Project-pinned `sqlc` and `goose` tool dependencies.
* Basic home page and CSS.
* Focused tests for config, database opening, and routes.
* Auth service foundation with Argon2id password hashing and database-backed sessions.
* SQLC auth queries for users and sessions.
* Session middleware and current-user request context helpers.
* CSRF token cookie, request validation middleware, and tests.
* Register, login, logout, and authenticated account routes.
* Account verification, resend verification, password reset, and account credential update flows.
* Session management routes for revoking current or other active sessions.
* Durable email delivery via a SQLite-backed outbox processor and worker process.
* Periodic SQLite cleanup jobs for expired sessions, tokens, and outbox records.
* CLI subcommands for `all`, `serve`, `worker`, and `migrate`.
* Project-pinned `govulncheck` tooling and `make check` integration.
* Custom 404 page template for unmatched `GET`/`HEAD` routes.
* GitHub Actions test workflow.
* `make init` script for one-time project setup (module path and project name).
* Rate limiting with configurable policies and trusted proxy support.
