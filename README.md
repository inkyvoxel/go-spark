# Go Starter

This repository is being built into a starter template for small server-rendered Go web applications.

Current status:

* The first runnable scaffold is in place.
* The longer architecture notes are still captured below.
* Outstanding template work is tracked in [`docs/roadmap.md`](docs/roadmap.md).

## Quick Start

```sh
cp .env.example .env
make run
```

By default, the app listens on `:8080` and stores its SQLite database at `./data/app.db`.

Before publishing this as a public template, update the module path in `go.mod`, fill in the copyright holder in `LICENSE`, and review the roadmap.

## Overview

This project is a **server-rendered web application** built with a focus on simplicity, maintainability, and performance. It uses a **Go-first architecture** with minimal dependencies, avoiding unnecessary frontend complexity and heavy abstractions.

The guiding philosophy is:

> Prefer simple, explicit, and well-understood tools over complex frameworks.

This repository is intentionally structured to be:

* Easy for humans to understand
* Easy for AI tools (e.g., Codex) to navigate and modify
* Stable over time with minimal churn

---

## Tech Stack

### Backend

* **Go (Golang)** — primary language
* **net/http** — standard HTTP server
* **html/template** — server-side rendering

### Frontend

* **HTMX** — progressive enhancement for interactivity
* Minimal JavaScript (only when necessary)
* CSS via simple custom styles

### Database

* **SQLite** — embedded database
* **database/sql** — standard Go DB interface
* **modernc.org/sqlite** — pure Go SQLite driver (no CGO)

### Data Access

* **SQL-first approach**
* **sqlc** — generates type-safe Go code from SQL queries

### Migrations

* **goose** — SQL-based migration tool

### Logging

* **log/slog** — structured logging (standard library)

---

## Architecture Principles

### 1. Server-Driven UI

The application uses **server-side rendering (SSR)** as the default:

* HTML is rendered on the server using `html/template`
* HTMX is used for partial updates and interactivity
* Avoid SPA-style complexity unless absolutely necessary

This keeps:

* state on the server
* logic centralized
* frontend simple and predictable

---

### 2. SQL-First Data Layer

We intentionally avoid ORMs.

Instead:

* SQL queries live in `.sql` files
* `sqlc` generates strongly typed Go code
* Queries are explicit, readable, and easy to optimize

Benefits:

* No hidden abstractions
* Full control over performance
* Easier debugging

---

### 3. Minimal Dependencies

We prefer:

* Standard library where possible
* Small, focused libraries where necessary

Avoid:

* Large frameworks
* Magic/code generation beyond sqlc
* Deep dependency trees

---

### 4. Clear Separation of Concerns

Structure the codebase roughly as:

```
/cmd/app            # application entrypoint
/internal
  /handlers         # HTTP handlers (request/response layer)
  /services         # business logic
  /db
    /queries        # SQL files (for sqlc)
    /generated      # sqlc-generated code
/templates          # HTML templates
/migrations         # goose migration files
/static             # CSS, assets
```

Guidelines:

* Handlers: HTTP concerns only
* Services: business logic
* DB layer: persistence only
* Templates: rendering only (no business logic)

---

### 5. Thin Templates

Templates should:

* Only render data
* Avoid complex logic
* Use simple conditionals/loops only

All real logic belongs in Go code.

---

### 6. HTMX Usage

HTMX is used for:

* Partial page updates
* Forms and interactions
* Reducing full page reloads

Guidelines:

* Return HTML fragments from handlers
* Keep endpoints small and focused
* Prefer progressive enhancement

---

### 7. Database Strategy

SQLite is used because:

* Zero configuration
* Single file
* Fast for most workloads

Important notes:

* Suitable for low to moderate concurrency
* If scaling to multiple instances, consider migrating to Postgres
* Keep schema simple and well-indexed

---

### 8. Migrations

We use **goose** for migrations.

* Migrations live in `/migrations`
* Use SQL files with `-- +goose Up` / `-- +goose Down`
* Always write reversible migrations when possible

Example:

```sql
-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL
);

-- +goose Down
DROP TABLE users;
```

---

### 9. Future AI Integration

* All AI calls are **server-side only**
* Wrap OpenAI calls behind a small internal interface
* Never call external APIs directly from handlers

Guidelines:

* Keep prompts versioned or structured
* Log inputs/outputs (with redaction where needed)
* Prefer structured outputs when possible

---

## Authentication Strategy

This project uses a **simple, server-side session authentication model** implemented with Go’s standard library and a few small, focused dependencies.

The goal is to keep authentication:

* easy to understand
* secure by default
* compatible with server-rendered HTML
* maintainable by both humans and AI tools

---

### Overview

Authentication is implemented using:

* **Email + password login**
* **Server-side sessions stored in SQLite**
* **HTTP-only cookies for session IDs**
* **Minimal external dependencies**

We intentionally avoid:

* JWT-based auth (unnecessary for this architecture)
* large auth frameworks
* client-side auth state

---

### Core Components

#### 1. Users Table

Stores account information:

```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL
);
```

---

#### 2. Sessions Table

Stores active sessions:

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

---

### Authentication Flow

#### Login

1. User submits email + password
2. Lookup user by email
3. Compare password using `bcrypt`
4. If valid:

   * generate a secure random session token
   * store it in `sessions` table
   * set cookie with session token

---

#### Session Handling

* Cookie contains only the **session token**
* On each request:

  * middleware reads cookie
  * looks up session in DB
  * loads associated user
  * attaches user to request context

---

#### Logout

* Delete session from database
* Clear cookie

---

### Password Security

* Passwords are hashed using **bcrypt**
* Never store plaintext passwords
* Use appropriate cost factor (default is fine to start)

---

### Cookie Configuration

Session cookies must be configured securely:

* `HttpOnly: true` → prevents JavaScript access
* `Secure: true` → HTTPS only (required in production)
* `SameSite: Lax` (or `Strict` if possible)
* Set reasonable expiration

Example:

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

---

### Middleware

All authenticated routes should use middleware that:

1. Reads session cookie
2. Validates session in DB
3. Loads user
4. Adds user to request context

Handlers should **not** handle auth logic directly.

---

### CSRF Protection

All state-changing requests (POST, PUT, DELETE) must include CSRF protection.

Recommended approach:

* generate CSRF token per session
* include token in forms
* validate token on submission

---

### Token Generation

* Use cryptographically secure random values
* Minimum 32 bytes entropy
* Encode with base64 or hex

Example:

```go
b := make([]byte, 32)
_, err := rand.Read(b)
token := hex.EncodeToString(b)
```

---

### Session Expiry

* Store `expires_at` in DB
* Enforce expiration on each request
* Optionally implement session rotation

---

### Optional Enhancements

These can be added later if needed:

* password reset via email
* email verification
* “remember me” sessions
* OAuth login (Google, GitHub)
* rate limiting on login attempts

---

### Design Rationale

This approach is chosen because:

* it aligns with **server-rendered architecture**
* avoids unnecessary complexity (no JWTs)
* keeps **all auth state on the server**
* easy to reason about and debug
* works well with Go’s standard library
* easy for AI tools (e.g., Codex) to modify safely

---

### What Not To Do

* Do not store passwords in plaintext
* Do not invent your own hashing or crypto
* Do not store auth tokens in localStorage
* Do not expose session tokens to JavaScript
* Do not rely on client-side auth logic

---

### Summary

Authentication in this project is:

* **simple**
* **secure**
* **server-driven**
* **explicitly implemented**

This keeps the system predictable, maintainable, and aligned with the rest of the stack.

## Development Guidelines

### Code Style

* Prefer clarity over cleverness
* Small functions
* Explicit naming
* Avoid deep abstraction layers

### Adding Features

1. Add SQL query (if needed)
2. Generate code via `sqlc`
3. Add service logic
4. Add handler
5. Add template or partial

### Testing

* Focus on:

  * service logic
  * database interactions
* Avoid over-testing templates

---

## Why This Stack?

This stack is optimized for:

* Fast iteration
* Low operational complexity
* Long-term maintainability
* Small team (or solo developer) efficiency
* AI-assisted development (Codex-friendly)

It intentionally avoids:

* frontend-heavy architectures
* complex build pipelines
* unnecessary abstractions

---

## When This Architecture Works Best

* CRUD apps
* SaaS dashboards
* internal tools
* admin panels
* AI-backed workflows
* content-driven apps

---

## When to Reconsider

You may need a different architecture if you require:

* highly interactive client-side apps
* real-time collaboration (e.g., Google Docs-style)
* heavy frontend state management
* offline-first functionality

---

## Summary

This project favors:

* **Go + SQL + HTML**
* **Server-first design**
* **Minimalism and clarity**

The goal is a codebase that:

* scales in complexity slowly
* remains understandable
* works well with both humans and AI tools

---
