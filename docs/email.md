# Email

This document covers the auth-related email features in the starter.

For the background worker pattern behind delivery, see [jobs.md](jobs.md).  
For general package boundaries, see [architecture.md](architecture.md).

## What Exists Today

The starter includes:

* account confirmation emails
* resend verification flows
* password reset emails
* email change verification emails and old-address notices
* SMTP and log senders
* a SQLite-backed outbox processor for durable delivery

The SMTP sender speaks STARTTLS (enabled by `SMTP_TLS=true`, the standard
setup on port 587) with `PLAIN` authentication. Implicit TLS on port 465 is
not supported; see [production.md](production.md) for provider guidance.

## Design

Email is split into a few clear responsibilities:

* `internal/services` decides when email should be sent
* `internal/email` builds messages and sends them
* `internal/database` stores SQLite-backed tokens and outbox rows
* `internal/platform/sqlite` owns SQLite connection setup
* `internal/jobs` runs the outbox processor in the worker process

The request path never calls SMTP directly. It creates the domain record, enqueues an outbox row, and returns. Delivery happens later in the worker.

## Why the Outbox Exists

The outbox gives the starter a durable default:

* requests do not block on provider calls
* delivery survives restarts
* retries are explicit and testable
* local development stays simple because the queue is just a SQLite table

This is the preferred pattern for durable, delayed, retryable work in this project.

The web and worker run as separate processes, so the worker polls the outbox on
an interval (`emailJobInterval`, currently 5 seconds) rather than being notified
the instant a row is enqueued. That means up to one interval of latency between a
request enqueuing mail and the worker sending it, which is fine for auth mail. If
you need it snappier, shorten the interval — SQLite in WAL mode with the
`(status, available_at)` index makes frequent polling cheap. A cross-process
notify is not worth building for this.

## Auth Email Flows

### Account confirmation

Registration:

1. creates the user
2. creates a verification token
3. enqueues a confirmation email

Confirmation:

1. accepts the raw token
2. looks up the hashed token
3. marks it consumed
4. marks the user verified

### Password reset

Reset request:

1. accepts an email address
2. creates a reset token for a matching user
3. enqueues a reset email

Reset confirmation:

1. accepts the raw token
2. validates the hashed token
3. updates the password
4. consumes the token
5. revokes existing sessions

### Email change

Change request:

1. accepts the current password and new email address
2. creates an email-change token only when the new address is valid, different, and not already registered to another account (a taken address returns neutral success without creating a token or sending mail)
3. enqueues a verification email to the new address

Change confirmation:

1. accepts the raw token
2. validates the hashed token
3. updates the user's email address and marks the new address verified
4. consumes the token
5. revokes all of the user's sessions
6. optionally enqueues a notice to the old address, controlled by `AUTH_EMAIL_CHANGE_NOTICE_ENABLED`

## Local Development

Use `EMAIL_PROVIDER=log` by default.

This makes delivery visible in logs without sending real email. Switch to SMTP only when you want to test real provider behavior.

## When to Extend

Reach for `internal/email` changes when you need:

* a new auth-related email type
* a new sender/provider
* changes to message rendering

Reach for [jobs.md](jobs.md) when the change is really about scheduling, retries, or adding a new class of background work.
