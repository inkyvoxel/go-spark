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
* Cross-origin request protection (`Sec-Fetch-Site`/`Origin`) middleware and tests.
* Register, login, logout, and authenticated account routes.
* Account verification, resend verification, password reset, and account credential update flows.
* Session management routes for revoking current or other active sessions.
* Durable email delivery via a SQLite-backed outbox processor and worker process.
* Periodic SQLite cleanup jobs for expired sessions, tokens, and outbox records.
* CLI subcommands for `all`, `serve`, `worker`, and `migrate`.
* Project-pinned `govulncheck` tooling and `make check` integration.
* Custom 404 page template for unmatched `GET`/`HEAD` routes.
* GitHub Actions CI workflow: gofmt, vet, build, tests, govulncheck, sqlc drift check, Docker image build, and an init-script smoke test that renames the project and rebuilds it.
* Dependabot configuration with monthly, grouped updates for Go modules (root and tools), Docker base images, and GitHub Actions.
* A short, informal code of conduct, with a note that template users should adapt it to their own community.
* README badges (CI, Go version, license) and "Use this template" instructions.
* `make init` script for one-time project setup (module path and project name).
* Rate limiting with configurable policies and trusted proxy support.
* Two-factor authentication (TOTP) with QR code setup, code confirmation, backup codes (8 per user), disable flow, and mid-login challenge. Code generation and verification use the `github.com/pquerna/otp` library.
* TOTP replay protection: each accepted code's time-step counter is recorded and codes are rejected once used, so a captured code cannot be replayed within its validity window.
* Passkey (WebAuthn) support via `github.com/go-webauthn/webauthn`: passwordless, phishing-resistant sign-in that complements email/password and TOTP. A user-verified passkey login fully authenticates and skips the TOTP step, while password + TOTP remain as a fallback.
* Discoverable (usernameless) passkey login with browser conditional-UI autofill plus an explicit "Sign in with a passkey" button.
* Account passkey management page to register, rename, and remove passkeys, and an optional "set up a passkey" prompt after email confirmation.
* Per-user random 64-byte WebAuthn handle folded into the base users migration, and a `webauthn_credentials` table storing each credential's public key, sign counter, and backup-eligibility/state flags.
* WebAuthn ceremony state is carried in short-lived, HMAC-signed, path-scoped cookies (no server-side session store needed), mirroring the TOTP pending-login cookie.
* Configurable passkey relying party: `AUTH_PASSKEY_RP_ID` (defaults to the host of `APP_BASE_URL`) and `AUTH_PASSKEY_RP_DISPLAY_NAME`, plus rate-limit overrides for passkey register/login/manage.

### Changed

* Upgraded dependencies (`modernc.org/sqlite`, `golang.org/x/crypto`, `goose`, `sqlc`) and the Docker runtime image to Alpine 3.24.
* Replaced the `godotenv` dependency with a small standard-library `.env` parser covering the syntax this starter uses.
* SQLite pragmas (`foreign_keys`, `busy_timeout`, WAL) now apply through the connection string so they reach every pooled connection, and `synchronous=NORMAL` is set to pair with WAL.
* SMTP multipart messages use a random MIME boundary per message instead of a fixed string.
* `AUTH_PASSWORD_PEPPER` is trimmed of surrounding whitespace like `SECRET_KEY_BASE`, so a trailing newline from a secrets manager cannot silently change the effective pepper.

### Fixed

* The `all` process mode now exits with an error instead of hanging when the HTTP server fails to start (for example, when the port is already in use).

### Security

* Rate limiting no longer trusts the spoofable `X-Real-IP` header and reads `X-Forwarded-For` from the rightmost untrusted hop instead of the client-controlled leftmost entry, closing a per-IP rate-limit bypass behind trusted proxies.
* Login takes the same time whether or not the email is registered, by verifying against a dummy Argon2id hash on the unknown-email path.
* TOTP codes are compared in constant time.
