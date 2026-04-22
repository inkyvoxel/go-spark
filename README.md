# Go Spark

A small starter template for server-rendered Go web applications.

The default shape is intentionally simple: Go, `net/http`, `html/template`, PicoCSS, HTMX where it helps, SQLite, SQL migrations, and a small set of conventions that are easy to change later.

It includes a runnable app, SQLite setup, migrations, generated SQL code, CSRF protection, and email/password session authentication with basic transactional email support.

Frontend assets are vendored under `static/vendor` so the starter works without runtime CDN dependencies.

Auth business logic is kept behind a small storage interface. The default storage adapter uses SQLite and `sqlc`, but database-specific driver behavior stays out of services and HTTP handlers.

## Tech Stack

* Go
* `net/http`
* `html/template`
* PicoCSS
* HTMX
* SQLite via `modernc.org/sqlite`
* `database/sql`
* `sqlc`
* `goose`
* `log/slog`

See [docs/architecture.md](docs/architecture.md) for the longer rationale and authentication approach.

## Quick Start

```sh
cp .env.example .env
make migrate-up
make run
```

Open:

```text
http://localhost:8080
http://localhost:8080/healthz
```

The app listens on `:8080` by default, stores its SQLite database at `./data/app.db`.

## Environment variables

`make run` loads `.env` when the file exists. Environment variables already set in your shell take precedence over values in `.env`.

`APP_PROCESS` selects which long-running process the binary should run:

* `all` starts the HTTP server and email worker together. This is the default and is best for local development.
* `web` starts only the HTTP server.
* `worker` starts only the email outbox worker.

You can also pass the process mode as the first CLI argument. The CLI argument wins over `APP_PROCESS`:

```sh
./go-spark web
./go-spark worker
./go-spark all
```

`AUTH_PASSWORD_PEPPER` is optional. When set, the app uses it as an application-level secret in password hashing by applying an HMAC-SHA256 pre-hash before Argon2id. When blank, no pepper is applied.
`CSRF_SIGNING_KEY` is used to sign CSRF tokens. It is required in production. In non-production, when blank, the app generates an ephemeral in-memory key at startup.

`AUTH_EMAIL_VERIFICATION_REQUIRED` controls whether account email verification is enforced. It defaults to `true`.
`AUTH_EMAIL_CHANGE_NOTICE_ENABLED` controls whether the app sends an old-email notification when an account email address changes. It defaults to `true`.

## CSRF Protection

State-changing requests use signed CSRF tokens that are bound to the current session context:

* Token format is versioned and HMAC-signed.
* Payload includes expiry and a session binding (`sha256(session cookie)` for authenticated requests, `anon` otherwise).
* Unsafe methods require:
  * submitted token (form field or `X-CSRF-Token` header),
  * exact match between submitted token and CSRF cookie,
  * valid signature,
  * unexpired token,
  * session binding match.

Token rotation behavior:

* CSRF token rotates on successful login/register.
* CSRF cookie is cleared on logout and other session-invalidating transitions.
* Legacy unsigned tokens are not accepted (hard cutover).

## Emails

Built-in email functionality includes:

* Account confirmation emails on registration.
* Confirmation links at `/account/confirm-email`.
* Resend confirmation from the account page for signed-in, unverified users.
* Password reset emails with reset links at `/account/reset-password`.
* Durable email delivery intent via a database outbox worker.

When `AUTH_EMAIL_VERIFICATION_REQUIRED=false`:

* New users are marked verified at registration time.
* Verification emails are not enqueued.
* Verification routes remain mounted for compatibility but redirect to normal login/account flows.

Email delivery defaults to `EMAIL_PROVIDER=log` for safe local development. Set `EMAIL_PROVIDER=smtp` with `SMTP_*` values to send real mail.

## Auth Rate Limiting

Sensitive auth POST endpoints are protected by an in-memory fixed-window limiter:

* `POST /login` keyed by `IP + normalized email`
* `POST /register` keyed by `IP + normalized email`
* `POST /account/forgot-password` keyed by `IP + normalized email`
* `POST /account/reset-password` keyed by `IP + reset-cookie token hash prefix`
* `POST /account/resend-verification` keyed by `IP + normalized email`
* `POST /account/verify-email/resend` keyed by `IP + userID`
* `POST /account/change-password` keyed by `IP + userID`
* `POST /account/change-email` keyed by `IP + userID`
* `POST /account/sessions/revoke` keyed by `IP + userID`
* `POST /account/sessions/revoke-others` keyed by `IP + userID`

Default policies:

* Login: `5/min`
* Register: `3/10min`
* Forgot password: `3/15min`
* Reset password: `5/15min`
* Public resend verification: `3/15min`
* Account resend verification: `5/15min`
* Change password: `5/15min`
* Change email: `5/15min`
* Revoke session: `20/15min`
* Revoke other sessions: `10/15min`

Behavior:

* Denied requests return HTTP `429 Too Many Requests`.
* `Retry-After` is set in seconds.
* Limiter uses `RemoteAddr` IP parsing in v1 (forwarded headers are not trusted).
* If email is missing or malformed, keying falls back to IP-only for safety.
* Data is in-memory per app instance (not shared across replicas).

Optional env overrides are available for each policy:

* `RATE_LIMIT_<POLICY>_MAX_REQUESTS` (positive integer)
* `RATE_LIMIT_<POLICY>_WINDOW` (Go duration, for example `1m`, `10m`, `15m`)

Policy names:

* `LOGIN`
* `REGISTER`
* `FORGOT_PASSWORD`
* `RESET_PASSWORD`
* `PUBLIC_RESEND_VERIFICATION`
* `ACCOUNT_RESEND_VERIFICATION`
* `CHANGE_PASSWORD`
* `CHANGE_EMAIL`
* `REVOKE_SESSION`
* `REVOKE_OTHER_SESSIONS`

Example:

```sh
RATE_LIMIT_LOGIN_MAX_REQUESTS=10
RATE_LIMIT_LOGIN_WINDOW=1m
RATE_LIMIT_FORGOT_PASSWORD_MAX_REQUESTS=5
RATE_LIMIT_FORGOT_PASSWORD_WINDOW=30m
```

## Commands

```sh
make run
make run-all
make run-web
make run-worker
make test
make fmt
make tidy
make sqlc
make vulncheck
make migrate-up
make migrate-down
make migrate-status
make tools
```

`sqlc`, `goose`, and `govulncheck` are pinned as Go tool dependencies in `go.mod`, so the Makefile runs them through `go tool`. You do not need separate global installs.

See [docs/development.md](docs/development.md) for tool and workflow details. See [docs/email.md](docs/email.md) for the staged email and account-confirmation plan.

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening pull requests, and [CHANGELOG.md](CHANGELOG.md) for notable changes.

## Project Layout

```text
/cmd/app            application entrypoint
/internal/config    environment config
/internal/database  SQLite connection setup
/internal/db        SQL queries and generated sqlc package
/internal/email     email rendering, sending, and outbox delivery
/internal/paths     canonical public URL path constants
/internal/server    HTTP routes and handlers
/internal/services  business logic
/migrations         goose migrations
/templates          server-rendered HTML, with auth/account pages in /templates/account
/static             CSS and static assets
/docs               architecture and development notes
.github/workflows   CI configuration
.github             issue and pull request templates
```

## After Cloning

If you use this as a template for a new project:

- [ ] Rename the module in `go.mod`.
- [ ] Update the app name in README, templates, and config where needed.
- [ ] Copy `.env.example` to `.env`.
- [ ] Review `DATABASE_PATH`.
- [ ] Run migrations.
- [ ] Replace or remove example routes and templates.
- [ ] Add new public paths to `internal/paths` instead of scattering route strings through handlers, emails, templates, or tests.
- [ ] Update the copyright holder in `LICENSE`.
- [ ] Review production settings before deployment.

## Production Notes

Before deploying an app based on this template:

* Serve over HTTPS.
* Use secure, HTTP-only cookies for session tokens.
* Keep secrets out of Git and load them from environment or your deployment platform.
* Back up the SQLite database file.
* Run migrations as part of deploy or a controlled release step.
* Use `APP_PROCESS=web` for the HTTP process and `APP_PROCESS=worker` for the email worker when you want to run them separately.
* Set file permissions so the app can read and write the database path, but does not expose it publicly.
* Set `APP_COOKIE_SECURE=true` when the app is served over HTTPS by a reverse proxy or load balancer.
* Keep `AUTH_PASSWORD_MIN_LENGTH` at 12 or higher unless you have a clear compatibility reason.
* Set `AUTH_PASSWORD_PEPPER` in production for defense in depth.
* Plan pepper rotation carefully: changing `AUTH_PASSWORD_PEPPER` invalidates existing password verification until users reset passwords.
* Set `CSRF_SIGNING_KEY` to a strong secret in production.
* Add request timeouts and deployment-specific logging/metrics as the app grows.

A simple deployment can run the same binary as two services, for example one service with `APP_PROCESS=web` behind Caddy, nginx, or another reverse proxy, and one service with `APP_PROCESS=worker` for email delivery.

## Replacing SQLite

SQLite is a good default for a simple starter because it keeps local development and deployment small. If a future project needs multiple app instances, heavier write concurrency, or managed database operations, Postgres is the natural next step.

The intended migration path is:

1. Add a Postgres driver.
2. Add or switch the database opener and config.
3. Port migrations and SQL queries.
4. Update `sqlc.yaml` to use the Postgres engine and regenerate generated code.
5. Implement storage adapters that satisfy the service-owned interfaces, such as the auth store.
6. Translate Postgres driver errors, such as unique violations, inside those adapters.
7. Run adapter, service, route, and migration tests against the new database.

Services own business rules, while storage adapters own SQL and database-driver details. Keeping that boundary small should make a database switch possible without rewriting HTTP handlers or auth business logic.

## Removing Example Code

As the starter grows, example code should stay easy to identify and delete. For a new project, start by replacing the home template, removing sample routes that do not fit, and adding the first real model, migration, query, service, and handler as a thin vertical slice.
