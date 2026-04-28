# Generated Features

This document describes what `go-spark new` includes for each selectable feature.

The generator resolves dependencies between components, copies the selected source bundles, and writes `internal/features/features.go` so runtime wiring only enables the chosen behaviors.

## How To Read This

* `core`, `sqlite`, and `web` are foundational components. Selecting a higher-level feature automatically pulls them in when required.
* Some shared internal support code remains in generated projects even when a higher-level feature is off. That keeps the generated app on a stable compile-time surface while feature flags, routes, templates, docs, and migrations stay aligned with the selected runtime behavior.
* `docs/todo.md` in generated apps is intentionally replaced with `No open TODOs.`

## Component Matrix

| Selectable feature | Auto-included dependencies | Adds runtime behavior | Adds docs | Adds templates | Adds migrations |
| --- | --- | --- | --- | --- | --- |
| `core` | none | app bootstrap, config, logging, shared services, generated feature flags | `docs/architecture.md`, `docs/development.md`, `docs/production.md` | none | none |
| `sqlite` | `core` | SQLite connection setup and sqlc config | none | none | none |
| `web` | `sqlite` | HTTP server, health pages, request middleware, base route handling | none | `templates/404.html`, `templates/breadcrumb.html`, `templates/home.html`, `templates/layout.html` | none |
| `csrf` | `web` | CSRF protection for form flows | none | none | none |
| `email-outbox` | `sqlite` | durable email outbox store, processor wiring, SMTP/log senders | `docs/email.md` | email message templates under `internal/email/templates` | `migrations/00005_email_outbox_schema.sql` |
| `worker` | `core` | background jobs runner and worker process support | `docs/jobs.md` | none | none |
| `auth` | `sqlite`, `web`, `csrf`, `email-outbox` | registration, login, sessions, account pages, change password | none | `templates/account/account.html`, `templates/account/change_password.html`, `templates/account/login.html`, `templates/account/register.html` | `migrations/00001_auth_schema.sql`, `migrations/00003_password_reset_schema.sql`, `migrations/00004_email_change_schema.sql` |
| `password-reset` | `auth`, `email-outbox` | forgot-password and reset-password routes | none | `templates/account/forgot_password.html`, `templates/account/reset_password.html` | `migrations/00003_password_reset_schema.sql` |
| `email-verification` | `auth`, `email-outbox`, `worker` | resend-verification and verify-email flows | none | `templates/account/confirm_email.html`, `templates/account/resend_verification.html`, `templates/account/verify_email.html` | `migrations/00002_email_verification_schema.sql` |
| `email-change` | `auth`, `email-outbox` | change-email and confirm-email-change flows | none | `templates/account/change_email.html`, `templates/account/confirm_email_change.html` | `migrations/00004_email_change_schema.sql` |
| `cleanup` | `auth`, `password-reset`, `email-verification`, `email-change`, `email-outbox`, `worker` | periodic cleanup jobs for sessions, tokens, and outbox rows | none | none | none |

## Smoke-Tested Generated Outputs

The generator test suite now covers these generated-project combinations end to end:

* minimal web
* auth without verification
* auth plus password reset
* full feature output

Those tests generate a fresh app, run migrations, build the generated module, and verify key HTTP routes for the selected feature set.

## Practical Notes

* The smallest supported web app today is `web`, which also pulls in `sqlite` and `core`.
* `email-verification`, `password-reset`, and `email-change` are runtime-facing auth subfeatures. Their route and template surfaces are pruned independently even though some shared auth support code remains in the generated module.
* Generated apps keep the normal runtime CLI: `all`, `serve`, `worker`, and `migrate`.