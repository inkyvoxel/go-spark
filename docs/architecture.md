# Architecture

This document captures the longer design notes for Go Spark.

## Overview

This project is a server-rendered web application starter built with a focus on simplicity, maintainability, and performance. It uses a Go-first architecture with minimal dependencies, avoiding unnecessary frontend complexity and heavy abstractions.

The guiding philosophy is:

> Prefer simple, explicit, and well-understood tools over complex frameworks.

This repository is intentionally structured to be:

* Easy for humans to understand.
* Stable over time with minimal churn.

## Tech Stack

### Backend

* Go as the primary language.
* `net/http` for the HTTP server.
* `html/template` for server-side rendering.

### Frontend

* PicoCSS for default styling and semantic HTML components.
* HTMX for progressive enhancement.
* Minimal JavaScript only when necessary.
* Project-specific CSS only when the defaults need overrides.

### Database

* SQLite as the embedded database.
* `database/sql` as the standard Go database interface.
* `modernc.org/sqlite` as the pure-Go SQLite driver.

### Data Access

* SQL-first approach.
* `sqlc` for type-safe Go code generated from SQL queries.

### Migrations

* `goose` for SQL-based migrations.

### Logging

* `log/slog` for structured logging.

## Architecture Principles

### Server-Driven UI

The application uses server-side rendering as the default:

* HTML is rendered on the server using `html/template`.
* HTMX is used for partial updates and interactivity.
* SPA-style complexity is avoided unless clearly needed.

This keeps state on the server, centralizes logic, and keeps the frontend predictable.

### SQL-First Data Layer

This starter intentionally avoids ORMs.

Instead:

* SQL queries live in `.sql` files.
* `sqlc` generates strongly typed Go code.
* Queries are explicit, readable, and easy to optimize.
* Storage adapters wrap generated queries and translate driver-specific errors into service-level errors.

The goal is to keep performance, debugging, and data access behavior visible.

Business logic should not import database drivers. For example, auth registration treats duplicate email as a domain error, while the SQLite-backed auth store is responsible for recognizing SQLite's unique constraint error and returning that domain error.

### Minimal Dependencies

Prefer:

* Standard library where possible.
* Small, focused libraries where necessary.

Avoid:

* Large frameworks.
* Hidden magic.
* Code generation beyond cases where it clearly pays for itself, such as `sqlc`.
* Deep dependency trees.

### Clear Separation of Concerns

The intended codebase structure is:

```text
/cmd/app            application entrypoint
/internal
  /config           environment config
  /database         database connection setup
  /email            email rendering, sending, and outbox delivery
  /paths            canonical public URL path constants
  /server           HTTP routes and handlers
  /services         business logic
  /db
    /queries        SQL files for sqlc
    /generated      sqlc-generated code
/templates          HTML templates, with account/auth pages under /templates/account
/migrations         goose migration files
/static             CSS and assets
```

Guidelines:

* Handlers own HTTP request and response concerns.
* Services own business logic.
* Services define the small storage interfaces they need.
* The DB layer owns persistence, generated queries, and database-driver error translation.
* Shared public URL paths live in `internal/paths` so server code, email links, tests, and templates do not drift apart.
* Templates render data and avoid business rules.

### Route and Template Constants

Public URL paths are centralized in `internal/paths`.

Use those constants when:

* Registering routes.
* Redirecting from handlers or middleware.
* Generating email links.
* Building route-aware template data.
* Asserting route behavior in tests.

Server mux patterns should be composed at registration time instead of duplicated as separate constants:

```go
dynamic.Handle(route(http.MethodGet, paths.Account), s.requireVerifiedAuth(http.HandlerFunc(s.account)))
dynamic.Handle(route(http.MethodGet, paths.ChangePassword), s.requireVerifiedAuth(http.HandlerFunc(s.changePasswordForm)))
```

Templates cannot import Go packages directly, so handlers pass a route catalog into `templateData` as `.Routes`. Template links and form actions should use those helpers:

```html
<a href="{{ .Routes.Login }}">Sign in</a>
<form method="post" action="{{ .Routes.ForgotPassword }}" hx-post="{{ .Routes.ForgotPassword }}">
```

Template keys and fragment names are centralized in the server package. Use those constants in render calls and tests instead of inline strings such as `"login.html"` or `"login_form_section"`.

### Canonical Routes

Core routes:

| Method | Path | Purpose | Auth policy |
| --- | --- | --- | --- |
| `GET` | `/` | Home page | Optional session |
| `GET` | `/healthz` | Health check | Public |
| `GET` | `/static/*` | Static assets | Public |

Auth and account routes:

| Method | Path | Purpose | Auth policy |
| --- | --- | --- | --- |
| `GET` | `/login` | Sign-in form | Anonymous only |
| `POST` | `/login` | Sign in | Anonymous only, rate limited |
| `GET` | `/register` | Registration form | Anonymous only |
| `POST` | `/register` | Create account | Anonymous only, rate limited |
| `POST` | `/logout` | Sign out | Authenticated |
| `GET` | `/account` | Account page | Verified authenticated |
| `GET` | `/account/change-password` | Change password form | Verified authenticated |
| `POST` | `/account/change-password` | Change password | Verified authenticated, rate limited |
| `GET` | `/account/change-email` | Change email request form | Verified authenticated |
| `POST` | `/account/change-email` | Request email change verification | Verified authenticated, rate limited |
| `GET` | `/account/confirm-email-change` | Consume email change token | Public |
| `GET` | `/account/verify-email` | Unverified-account interstitial | Authenticated (redirects to `/account` when verification is optional) |
| `POST` | `/account/verify-email/resend` | Resend confirmation for signed-in user | Authenticated, rate limited (redirects to `/account` when verification is optional) |
| `GET` | `/account/confirm-email` | Consume email confirmation token | Public (redirects to `/login` or `/account` when verification is optional) |
| `GET` | `/account/forgot-password` | Password reset request form | Anonymous only |
| `POST` | `/account/forgot-password` | Request password reset email | Anonymous only, rate limited |
| `GET` | `/account/reset-password` | Reset password form | Anonymous only |
| `POST` | `/account/reset-password` | Reset password | Anonymous only |
| `GET` | `/account/resend-verification` | Public resend confirmation form | Anonymous only (redirects to `/login` or `/account` when verification is optional) |
| `POST` | `/account/resend-verification` | Public resend confirmation request | Anonymous only, rate limited (redirects to `/login` or `/account` when verification is optional) |

### Thin Templates

Templates should:

* Render data.
* Use simple conditionals and loops.
* Avoid complex logic.

Real logic belongs in Go code.

### HTMX Usage

HTMX is used for:

* Partial page updates.
* Forms and interactions.
* Reducing full page reloads where it improves the user experience.

Guidelines:

* Return HTML fragments from handlers where appropriate.
* Keep endpoints small and focused.
* Prefer progressive enhancement.

### Database Strategy

SQLite is used because it has zero service configuration, works well for local development, and is fast for many small-to-medium workloads.

Important notes:

* SQLite is suitable for low to moderate concurrency.
* If scaling to multiple instances, consider migrating to Postgres.
* Keep schema simple and well-indexed.
* Treat database backups as part of production readiness.

Database-specific behavior should stay inside database adapters. If a project moves to Postgres, keep the service interfaces stable, port the migrations and SQL queries, regenerate `sqlc` code for Postgres, and add Postgres-backed adapters that translate Postgres errors such as unique violations into the same service errors.

### Migrations

Migrations live in `/migrations` and use `goose` SQL files:

```sql
-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL
);

-- +goose Down
DROP TABLE users;
```

Always write reversible migrations when practical.

## Authentication Strategy

Authentication uses a simple, server-side session model implemented with Go's standard library and a few focused dependencies.

The goal is to keep authentication:

* Easy to understand.
* Secure by default.
* Compatible with server-rendered HTML.
* Maintainable without a large auth framework.

### Overview

Authentication uses:

* Email and password login.
* Account email verification via confirmation links.
* Password reset via email.
* Server-side sessions stored in SQLite.
* HTTP-only cookies for session IDs.
* Minimal external dependencies.

This starter intentionally avoids:

* JWT-based auth for regular server-rendered pages.
* Large auth frameworks.
* Client-side auth state.

### Users Table

```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    email_verified_at TIMESTAMP
);
```

### Sessions Table

```sql
CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
```

### Email Verification Tokens Table

```sql
CREATE TABLE email_verification_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
```

Only token hashes are stored. The raw token is generated once, sent to the user, and treated as a secret.

### Password Reset Tokens Table

```sql
CREATE TABLE password_reset_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
```

Only token hashes are stored. The raw reset token is sent to the user once and must be treated as a bearer credential.

### Email Outbox Table

```sql
CREATE TABLE email_outbox (
    id INTEGER PRIMARY KEY,
    sender TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    text_body TEXT NOT NULL,
    html_body TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    available_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

The outbox stores email delivery intent durably. A worker can claim pending rows, send them through the configured email sender, and mark them sent or retryable.

### Login Flow

1. User submits email and password.
2. Application looks up the user by email.
3. Application compares the password using Argon2id.
4. If valid, the application generates a secure random session token, stores it in the `sessions` table, and sets a cookie with the session token.

### Session Handling

The cookie contains only the session token.

On each request:

1. Middleware reads the session cookie.
2. Middleware looks up the session in the database.
3. Middleware loads the associated user.
4. Middleware attaches the user to the request context.

Handlers should not handle auth logic directly.

### Route Auth Policies

Routes should express their auth policy at registration time:

```go
dynamic.Handle(route(http.MethodGet, paths.Account), s.requireVerifiedAuth(http.HandlerFunc(s.account)))
dynamic.Handle(route(http.MethodGet, paths.Login), s.requireAnonymous(http.HandlerFunc(s.loginForm)))
```

Use:

* `loadSession` for dynamic routes that should know who the current user is when a valid session cookie is present.
* `requireAuth` for protected pages and actions.
* `requireVerifiedAuth` for pages and actions that require both a signed-in user and a verified email address.
* `requireAnonymous` for sign-in and registration pages that should redirect signed-in users back to their account.

For browser page requests, anonymous `GET` requests to protected pages should redirect to `/login` with a safe `next` path. Unsafe requests such as `POST` should return `401 Unauthorized` when the user is not signed in.

Account-specific routes should live under `/account` unless there is a clear product reason not to. This keeps auth/account URLs from colliding with future application features.

Redirect destinations must be validated before use. Only local paths such as `/account` or `/dashboard?tab=home` should be accepted. Absolute URLs like `https://example.com` and protocol-relative URLs like `//example.com` must be rejected to avoid open redirect vulnerabilities.

### Logout

Logout deletes the session from the database and clears the cookie.

### Password Security

* Passwords are hashed using Argon2id in PHC format.
* When configured, an application-level pepper is applied as an HMAC-SHA256 pre-hash before Argon2id.
* Plaintext passwords are never stored.
* Defaults follow OWASP's recommended Argon2id baseline (`m=19456`, `t=2`, `p=1`).

### Cookie Configuration

Session cookies must be configured securely:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "session",
    Value:    token,
    Path:     "/",
    HttpOnly: true,
    Secure:   true,
    SameSite: http.SameSiteLaxMode,
})
```

For local development, `Secure` may need environment-aware handling if testing over plain HTTP.

Set `APP_COOKIE_SECURE=true` in production when HTTPS is terminated before the Go process, such as behind a reverse proxy or load balancer. Direct HTTPS requests also receive secure cookies automatically because the server can see `r.TLS`.

### CSRF Protection

All state-changing requests should include CSRF protection.

Recommended approach:

* Generate a CSRF token cookie.
* Include the token in forms.
* Validate submitted tokens against the cookie on unsafe requests.

### Token Generation

Use cryptographically secure random values with at least 32 bytes of entropy:

```go
b := make([]byte, 32)
_, err := rand.Read(b)
token := hex.EncodeToString(b)
```

### Session Expiry

* Store `expires_at` in the database.
* Enforce expiration on each request.
* Optionally add session rotation.

### Optional Enhancements

These can be added later if a project needs them:

* Remember-me sessions.
* OAuth login with providers such as Google or GitHub.
* Production-grade shared rate limiting for multi-instance deployments.

### Auth Non-Goals

* Do not store passwords in plaintext.
* Do not invent custom hashing or crypto.
* Do not store auth tokens in `localStorage`.
* Do not expose session tokens to JavaScript.
* Do not rely on client-side auth logic.

## Development Guidelines

### Code Style

* Prefer clarity over cleverness.
* Use small functions.
* Use explicit naming.
* Avoid deep abstraction layers.

### Adding Features

1. Add a migration if schema changes are needed.
2. Add or update SQL queries.
3. Generate code via `sqlc`.
4. Add or update a focused storage interface and database adapter if service logic needs persistence.
5. Add a handler.
6. Add a template or partial.
7. Add focused tests for the behavior.

### Testing

Focus tests on service logic, database interactions, and route behavior. Avoid over-testing template markup unless the rendered behavior is important.

## When This Architecture Works Best

* CRUD apps.
* SaaS dashboards.
* Internal tools.
* Admin panels.
* Content-driven apps.

## When To Reconsider

You may need a different architecture if the product requires:

* Highly interactive client-side app behavior.
* Real-time collaboration.
* Heavy frontend state management.
* Offline-first functionality.

## Summary

This starter favors Go, SQL, HTML, server-first design, minimalism, and clarity. The goal is a codebase that scales in complexity slowly and remains understandable.
