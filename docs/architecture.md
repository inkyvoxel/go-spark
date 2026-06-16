# Architecture

This document explains the shape of the codebase at a high level.

For day-to-day commands, see [development.md](development.md).  
For background work, see [jobs.md](jobs.md).  
For auth email flows, see [email.md](email.md).

## Principles

Go Spark prefers:

* explicit code over framework magic
* standard library defaults where practical
* SQL-first data access
* SQLite-first persistence for new projects
* server-rendered UI by default
* small, focused packages with clear boundaries

## Package Boundaries

```text
/cmd/app            wires the application together
/internal/app       application bootstrap and runtime assembly
/internal/config    reads environment config
/internal/database  SQLite-backed domain stores
/internal/db        SQL queries and generated sqlc code
/internal/email     email messages, senders, and outbox processor
/internal/jobs      jobs runner and periodic background jobs
/internal/platform  engine-specific platform code such as SQLite setup
/internal/paths     canonical public URL paths
/internal/server    HTTP handlers, middleware, templates
/internal/services  business logic
```

Rules of thumb:

* handlers own HTTP concerns
* services own business logic
* stores own persistence and SQLite-specific translation today
* engine setup belongs in engine-focused packages under `internal/platform`
* templates render data, not business rules
* `internal/server` must not import `internal/db/generated`; handlers should work with service-owned types and leave generated persistence models inside stores

Go Spark keeps service/store seams because they protect business logic from
HTTP and persistence concerns. Those seams are not a promise that the starter
currently supports interchangeable database backends.

## Request Flow

Most features follow this path:

1. `internal/server` receives and validates the request
2. `internal/services` applies business rules
3. `internal/database` persists through SQLite-targeted `sqlc` queries
4. the handler renders HTML or redirects

This keeps HTTP concerns, business rules, and SQLite persistence behavior
separate.

## Rendering Conventions

The app is server-rendered by default.

Important conventions:

* public paths live in `internal/paths`
* mux patterns are assembled in server routing, not duplicated as string literals
* templates use `.Routes` instead of hard-coded URLs
* template keys and fragments are centralized in the server package

## Authentication Model

The starter uses:

* email and password login
* two-factor authentication (TOTP) with backup codes — optional per user, enforced at login when enabled
* passkeys (WebAuthn) — optional per user, for passwordless phishing-resistant sign-in
* server-side sessions stored in SQLite
* HTTP-only session cookies
* email verification
* password reset by email

Login defaults to a browser-session cookie. Users can explicitly choose "Remember me on this device" to receive a persistent cookie that lasts until the server-side session expires. Session cookies remain HTTP-only, SameSite=Lax, and Secure when configured or served over TLS.

TOTP follows a three-step flow: the user initiates setup (secret generated and stored as pending), scans the QR code in their authenticator app, then confirms with a valid code — at which point 8 one-time backup codes are generated and shown once. At login, users with 2FA enabled are issued a short-lived signed pending cookie and redirected to a challenge page before a full session is created. The pending cookie carries the explicit remember-me choice so the final session cookie matches what the user selected at the password step.

Passkeys are built on the [`go-webauthn/webauthn`](https://github.com/go-webauthn/webauthn) library — the one place the starter accepts a third-party dependency, because WebAuthn's CBOR/COSE/attestation handling is not something to hand-roll. The relying party is configured from `APP_BASE_URL` (the RP ID defaults to its host, the allowed origin is the URL itself). Each user gets a random 64-byte WebAuthn handle (folded into the base users migration) so the authenticator stores no PII, and credentials live in `webauthn_credentials`. Registration is offered after email confirmation and from the account passkey page; it requires a verified, authenticated session. Because passkeys require **user verification** (biometric, PIN, or security-key gesture), a passkey assertion is treated as full multi-factor authentication: a passkey login skips both the password and the TOTP challenge. Email/password + TOTP remain as a fallback, so removing the last passkey is safe. Login is discoverable (usernameless): the browser surfaces passkeys via conditional-UI autofill and an explicit button, and the server resolves the user from the credential's user handle. Like the TOTP pending cookie, the WebAuthn begin→finish ceremony state is carried in a short-lived, HMAC-signed, path-scoped cookie rather than a server-side store, so no extra cleanup job is needed. The register/login endpoints are JSON over `fetch`, protected by the existing `X-CSRF-Token` header and Origin checks.

It intentionally does not use JWTs or a large auth framework for the default server-rendered flow.

## Data Layer

The project is SQL-first:

* the starter's SQLite schema migrations live in `migrations`
* queries go in `internal/db/queries`
* `sqlc` generates typed query code in `internal/db/generated`

SQLite is not just the default implementation; it is the intended foundation
for this starter. That keeps setup small, local development easy, and the
deployment story friendly to single-node and single-binary projects.

The starter does not currently aim to provide plug-and-play support for
multiple SQL engines. If a future fork needs something else, treat that as an
explicit refactor of the persistence layer.

If later phases split connection setup away from stores, the preferred
direction is:

* keep SQLite engine setup and tuning in an explicit SQLite-focused package
* keep domain stores separate from engine setup
* keep service/store seams because they support domain boundaries, not because
  they imply broad engine portability
* keep tuning defaults small and documented instead of introducing a large
  connection abstraction

Current SQLite tuning defaults in `internal/platform/sqlite` are:

* `PRAGMA foreign_keys = ON` to keep relational constraints enforced
* `PRAGMA journal_mode = WAL` for concurrent reads and non-blocking writes
* `PRAGMA busy_timeout = 5000` to tolerate short write contention
* `MaxOpenConns = 1` per process to match the single-writer SQLite model

WAL mode allows concurrent reads from multiple processes without blocking writes. In the Docker deployment, `app` and `worker` each open the same SQLite file from separate containers via a shared volume. WAL handles this correctly; `busy_timeout` handles any transient write contention.

## Application Bootstrap

`cmd/app` stays intentionally thin:

* load environment and parse process mode
* assemble signal handling and shutdown behavior
* delegate runtime wiring to `internal/app`

`internal/app` owns application assembly:

* SQLite connection setup
* service and store wiring
* email sender and background job composition
* HTTP server construction

## Background Work

Background work uses two patterns:

* periodic housekeeping jobs in `internal/jobs`
* durable domain-specific processors backed by explicit tables, like `email_outbox`

The app intentionally does not have a generic persisted jobs framework. See [jobs.md](jobs.md) for the decision and extension guidance.
