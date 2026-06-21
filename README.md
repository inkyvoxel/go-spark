# Go Spark

[![CI](https://github.com/inkyvoxel/go-spark/actions/workflows/ci.yml/badge.svg)](https://github.com/inkyvoxel/go-spark/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/inkyvoxel/go-spark)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A Go + SQLite web app starter template. Use it as a template, run `make init`, and you're off.

## What's Included

* production-ready server-rendered web app foundation
* standard library HTTP server and `html/template` rendering
* classless [Pico CSS](https://picocss.com) for sensible default styling — vendored, no build step, and easy to swap out
* SQLite persistence with `database/sql`, WAL mode, and practical connection defaults
* SQL-first workflow with `sqlc` generated queries and `goose` migrations
* complete email/password auth: registration, login, logout, email verification, password reset, and email change
* two-factor authentication with TOTP setup, challenge flow, and backup codes
* passwordless passkey (WebAuthn) sign-in with discoverable credentials and conditional-UI autofill, plus a passkey management page
* secure browser sessions with HTTP-only cookies and CSRF protection
* rate limiting for auth and account-sensitive endpoints
* transactional email templates with SMTP and local log senders
* durable email outbox with retrying background delivery
* background worker process with periodic cleanup jobs
* structured logging with `log/slog`
* health and readiness endpoints for deployment checks
* production Dockerfile and Docker Compose setup
* separate `migrate`, `app`, and `worker` services for clean deploys and failure isolation
* Caddy reverse proxy with automatic HTTPS
* documented single-server deployment, backups, SMTP deliverability, and secret rotation

## Getting Started

1. Click **Use this template** on GitHub to create your own repository with a clean history (or clone this one)
2. Run `make init` — prompts for your project name and Go module path, rewrites relevant files, generates a `.env` with random secrets, then removes itself
3. Start the app:

```sh
make migrate-up
make start
```

> If you skip `make init`, run `cp .env.example .env` and set the required secrets yourself (`SECRET_KEY_BASE`, `AUTH_TOTP_KEY`, `AUTH_PASSWORD_PEPPER` — generate each with `openssl rand -hex 32`).

## Adding Your First Feature

Once the app is running, the next step is usually adding your own product
feature: a database table, a page, a form, an email, or a worker job.

Start with [docs/extending.md](docs/extending.md). It walks through the common
extension patterns in this starter, including SQL migrations, `sqlc` queries,
stores, services, authenticated and public routes, HTML templates, email
templates, and background jobs.

## Common Commands

```sh
make start          # run HTTP server + background worker
make start-web      # run HTTP server only
make start-worker   # run background worker only
make build-prod     # build a release binary
make migrate-up     # apply migrations
make migrate-status # check migration status
make test           # run tests
make check          # fmt + tidy + sqlc + vulncheck + test
```

## Deploying

Production deployment uses Docker Compose and Caddy (automatic TLS). Copy `.env.example` to `.env`, fill in your domain and secrets, then:

```sh
docker compose up -d
```

See [docs/production.md](docs/production.md) for the full guide.

## Read Next

* [docs/development.md](docs/development.md)
* [docs/architecture.md](docs/architecture.md)
* [docs/extending.md](docs/extending.md)
* [docs/components.md](docs/components.md)
* [docs/production.md](docs/production.md)
* [CONTRIBUTING.md](CONTRIBUTING.md)
