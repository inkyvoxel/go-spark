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
/internal/auth      user accounts, sessions, TOTP, and passkeys
/internal/config    reads environment config
/internal/database  background stores (email outbox, cleanup)
/internal/db        SQL queries and generated sqlc code
/internal/email     email messages, senders, and outbox processor
/internal/jobs      jobs runner and periodic background jobs
/internal/platform  engine-specific platform code such as SQLite setup
/internal/paths     canonical public URL paths
/internal/project   example feature (a user-owned projects list)
/internal/ratelimit rate-limit policy vocabulary (names, defaults) shared by config and server
/internal/secret    purpose-scoped key derivation from the root secret
/internal/server    HTTP handlers, middleware, templates
```

Rules of thumb:

* handlers own HTTP concerns
* a feature package (`internal/auth`, `internal/project`) owns its business rules *and* its SQL, calling the generated `sqlc` queries directly — no separate store layer
* feature packages return their own domain types, never generated `db` rows
* engine setup belongs in engine-focused packages under `internal/platform`
* templates render data, not business rules
* `internal/server` must not import `internal/db/generated`; handlers work with the domain types feature packages expose (enforced by a test in `internal/server`)

Earlier versions of the starter put a separate store layer behind an interface
in front of each service. That seam was removed: with a single committed
database engine (SQLite) the interface had one implementation and earned only
ceremony, so each feature now talks to `sqlc` directly and is tested against a
real in-memory database. The boundary that still matters — keeping SQL out of
handlers and templates — is preserved by returning domain types.

## Request Flow

Most features follow this path:

1. `internal/server` receives and validates the request
2. a feature package (`internal/auth`, `internal/project`) applies business
   rules and persists through SQLite-targeted `sqlc` queries
3. the handler renders HTML or redirects

This keeps HTTP concerns separate from business-and-persistence logic, while
the latter two live together in one cohesive package per feature.

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

TOTP follows a three-step flow: the user initiates setup (secret generated and stored as pending), scans the QR code in their authenticator app, then confirms with a valid code — at which point 8 one-time backup codes are generated and shown once. At login, users with 2FA enabled are issued a short-lived signed pending cookie and redirected to a challenge page before a full session is created. The pending cookie carries the explicit remember-me choice so the final session cookie matches what the user selected at the password step. The HMAC-SHA1 code generation and verification are delegated to the maintained [`pquerna/otp`](https://github.com/pquerna/otp) library rather than hand-rolled; `internal/totp` adds only the otpauth:// URI and counter-returning verification, and the replay-protection counter is claimed atomically in `internal/auth`.

Passkeys are built on the [`go-webauthn/webauthn`](https://github.com/go-webauthn/webauthn) library — a deliberate third-party dependency (alongside `pquerna/otp` for TOTP), because WebAuthn's CBOR/COSE/attestation handling is not something to hand-roll. The relying party is configured from `APP_BASE_URL` (the RP ID defaults to its host, the allowed origin is the URL itself). Each user gets a random 64-byte WebAuthn handle (folded into the base users migration) so the authenticator stores no PII, and credentials live in `webauthn_credentials`. Registration is offered after email confirmation and from the account passkey page; it requires a verified, authenticated session. Because passkeys require **user verification** (biometric, PIN, or security-key gesture), a passkey assertion is treated as full multi-factor authentication: a passkey login skips both the password and the TOTP challenge. Email/password + TOTP remain as a fallback, so removing the last passkey is safe. Login is discoverable (usernameless): the browser surfaces passkeys via conditional-UI autofill and an explicit button, and the server resolves the user from the credential's user handle. Like the TOTP pending cookie, the WebAuthn begin→finish ceremony state is carried in a short-lived, HMAC-signed, path-scoped cookie rather than a server-side store, so no extra cleanup job is needed. The register/login endpoints are JSON over `fetch`; like every state-changing request they are covered by the cross-origin checks described below (a same-origin `fetch` sends `Sec-Fetch-Site: same-origin` automatically, so no custom header or token is needed).

It intentionally does not use JWTs or a large auth framework for the default server-rendered flow.

## CSRF Protection

State-changing requests are protected by the standard library's [`http.CrossOriginProtection`](https://pkg.go.dev/net/http#CrossOriginProtection) (Go 1.25+), paired with `SameSite=Lax` cookies. This is the modern defense for cookie-authenticated apps and needs no per-form token.

`CrossOriginProtection` rejects cross-origin unsafe requests using the `Sec-Fetch-Site` header (sent by all browsers since 2023), falling back to comparing the `Origin` header against the request host. The app's own origin (`APP_BASE_URL`) is registered as trusted so requests still pass behind a Host-rewriting reverse proxy. Requests that send neither `Sec-Fetch-Site` nor `Origin` — non-browser clients such as `curl` or server-to-server calls — are allowed, because there is no cross-origin signal to act on and such a request cannot be driven from a victim's browser. The implementation lives in `internal/server/csrf.go`; the middleware runs the check on every unsafe method (`POST`/`PUT`/`PATCH`/`DELETE`) and lets safe methods through untouched.

This template deliberately relies on origin verification alone rather than also issuing a signed CSRF token. For an app targeting modern browsers the token adds plumbing — a cookie, a hidden form field, a signing key — without meaningfully raising the bar over `CrossOriginProtection` + `SameSite=Lax`. If you must support pre-2023 browsers, or sit behind a proxy that strips both `Sec-Fetch-Site` and `Origin`, add a signed double-submit token as a second layer, following the [OWASP CSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html).

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

If a future fork does need engine portability, the preferred direction is:

* keep SQLite engine setup and tuning in an explicit SQLite-focused package
* introduce a persistence interface only at the point where a second engine
  actually exists, not speculatively
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
