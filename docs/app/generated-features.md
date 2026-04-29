# Generated Features

This document describes which runtime features are enabled in this app and what
files each feature typically includes.

## How To Read This

* `core` is the foundational component.
* Higher-level features may require foundational components.
* Some shared support code can remain present even when a higher-level runtime
  feature is off.

## Component Matrix

| Selectable feature | Auto-included dependencies | Adds runtime behavior | Adds docs | Adds templates | Adds migrations |
| --- | --- | --- | --- | --- | --- |
| `core` | none | app bootstrap, config, logging, shared services, generated feature flags, SQLite connection setup, sqlc config, HTTP server/middleware/routes, and CSRF protection | `docs/architecture.md`, `docs/development.md`, `docs/production.md` | `templates/404.html`, `templates/breadcrumb.html`, `templates/home.html`, `templates/layout.html` | none |
| `email-outbox` | `core` | durable email outbox store, processor wiring, SMTP/log senders | `docs/email.md` | email message templates under `internal/email/templates` | `migrations/00005_email_outbox_schema.sql` |
| `worker` | `core` | background jobs runner and worker process support | `docs/jobs.md` | none | none |
| `auth` | `core`, `email-outbox` | registration, login, sessions, account pages, change password, forgot-password/reset-password, resend-verification/verify-email, and change-email/confirm-email-change flows | none | `templates/account/account.html`, `templates/account/change_email.html`, `templates/account/change_password.html`, `templates/account/confirm_email.html`, `templates/account/confirm_email_change.html`, `templates/account/forgot_password.html`, `templates/account/login.html`, `templates/account/register.html`, `templates/account/resend_verification.html`, `templates/account/reset_password.html`, `templates/account/verify_email.html` | `migrations/00001_auth_schema.sql`, `migrations/00002_email_verification_schema.sql`, `migrations/00003_password_reset_schema.sql`, `migrations/00004_email_change_schema.sql` |
| `cleanup` | `auth`, `email-outbox`, `worker` | periodic cleanup jobs for sessions, tokens, and outbox rows | none | none | none |

## Practical Notes

* The smallest supported web app today is `core`.
* Generated apps keep the normal runtime CLI: `all`, `serve`, `worker`, and `migrate`.
