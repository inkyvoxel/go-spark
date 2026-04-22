# Email

This document covers the auth-related email features in the starter.

For the background worker pattern behind delivery, see [jobs.md](jobs.md).  
For general package boundaries, see [architecture.md](architecture.md).

## What Exists Today

The starter includes:

* account confirmation emails
* resend verification flows
* password reset emails
* SMTP and log senders
* a database-backed outbox processor for durable delivery

## Design

Email is split into a few clear responsibilities:

* `internal/services` decides when email should be sent
* `internal/email` builds messages and sends them
* `internal/database` stores tokens and outbox rows
* `internal/jobs` runs the outbox processor in the worker process

The request path never calls SMTP directly. It creates the domain record, enqueues an outbox row, and returns. Delivery happens later in the worker.

## Why the Outbox Exists

The outbox gives the starter a durable default:

* requests do not block on provider calls
* delivery survives restarts
* retries are explicit and testable
* local development stays simple because the queue is just a database table

This is the preferred pattern for durable, delayed, retryable work in this project.

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

## Local Development

Use `EMAIL_PROVIDER=log` by default.

This makes delivery visible in logs without sending real email. Switch to SMTP only when you want to test real provider behavior.

## When to Extend

Reach for `internal/email` changes when you need:

* a new auth-related email type
* a new sender/provider
* changes to message rendering

Reach for [jobs.md](jobs.md) when the change is really about scheduling, retries, or adding a new class of background work.
