# Email Strategy

This document outlines a staged email architecture for Go Spark.

The first email use case is account confirmation. The design should keep request handlers fast, avoid provider lock-in, and stay small enough for a starter template.

## Goals

* Send account confirmation emails without blocking web requests on provider calls.
* Keep email delivery intent durable across process restarts.
* Keep business logic independent from email providers.
* Use simple Go interfaces and explicit SQL instead of a background job framework.
* Preserve SQLite as the default database.

## Non-Goals

* Do not add Redis, RabbitMQ, or another queue service by default.
* Do not build a generic background job framework.
* Do not build a marketing email system.
* Do not introduce an HTML email design system yet.
* Do not make HTTP handlers know about SMTP or provider APIs.

## Recommended Shape

Use three separate concerns:

* Account/auth services own account verification rules.
* An email package owns message types and provider sending.
* The database layer owns persistence for verification tokens and email outbox rows.

The request path should create the user, create a verification token, and enqueue an email row in the database. A small worker should send queued emails outside the request path.

This is close to a queue, but the queue is just a database table. That is a good starting point for this template because it is durable, understandable, and does not require extra infrastructure.

## Package Boundaries

Suggested package boundaries:

```text
/cmd/app              wires config, database, services, sender, and worker
/internal/email       message types, sender interface, sender adapters, worker
/internal/services    auth/account verification business logic
/internal/database    SQLite-backed stores and driver-specific behavior
/internal/db          SQL queries and sqlc-generated code
/migrations           schema changes
```

The email package should own a small provider-facing interface:

```go
type Sender interface {
	Send(ctx context.Context, message Message) error
}

type Message struct {
	From     string
	To       string
	Subject  string
	TextBody string
	HTMLBody string
}
```

Provider adapters can later implement `Sender` for SMTP, Postmark, Resend, SES, Mailgun, or another service. A development `LogSender` should be the default so local development never sends real email by accident.

## Data Model

Add account verification state to users:

```sql
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMP;
```

Add verification tokens:

```sql
CREATE TABLE email_verification_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);
```

Store only the token hash. The raw token should be generated once, included in the email URL, and never persisted.

Add an email outbox:

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

Useful indexes:

```sql
CREATE INDEX email_verification_tokens_user_id_idx ON email_verification_tokens(user_id);
CREATE INDEX email_verification_tokens_token_hash_idx ON email_verification_tokens(token_hash);
CREATE INDEX email_outbox_pending_idx ON email_outbox(status, available_at);
```

## Account Confirmation Flow

Registration should:

1. Validate and create the user.
2. Generate a random verification token.
3. Store a hash of the token with an expiry, for example 24 hours.
4. Build a confirmation URL using the configured application base URL.
5. Enqueue an email in `email_outbox`.
6. Return the normal registration response without waiting for provider delivery.

Confirmation should:

1. Accept the raw token from the confirmation URL.
2. Hash the token and look up an unexpired, unconsumed token row.
3. Mark the token consumed.
4. Set `users.email_verified_at`.
5. Render a clear success or invalid-token page.

The app can allow sign-in before verification at first. If a project needs stricter behavior, add route or service checks later.

## Delivery Model

Start with an in-process worker:

* Runs from `cmd/app` alongside the HTTP server.
* Polls pending outbox rows at a small interval.
* Claims a small batch.
* Sends each message through the configured `email.Sender`.
* Marks successful rows as sent.
* Marks failed rows with `last_error`, increments `attempts`, and schedules retry using `available_at`.
* Marks rows as permanently failed after a small maximum attempt count.
* Stops cleanly when the app context is canceled.

This worker is intentionally modest. It is enough for a starter app and keeps all local development in one process.

If a project later needs multiple app instances or higher email throughput, the outbox table still helps: the worker can move to a separate process, or the outbox can be replaced with an external queue.

## Configuration

Add only the config needed for the current slice.

Initial config:

```text
APP_BASE_URL=http://localhost:8080
EMAIL_FROM=Go Spark <hello@example.com>
EMAIL_PROVIDER=log
EMAIL_LOG_BODY=false
```

SMTP config can be added in the provider slice:

```text
SMTP_HOST=
SMTP_PORT=587
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM=
SMTP_TLS=true
```

Invalid provider config should fail at startup. Missing real provider config should not matter when `EMAIL_PROVIDER=log`.

## Implementation Slices

### Slice 1: Strategy Document

Add this document and make no runtime changes.

### Slice 2: Email Foundation

Add `internal/email` with:

* `Message`
* `Sender`
* `LogSender`
* message construction helpers for account confirmation

Add config for:

* application base URL
* from address
* email provider

Tests should cover message construction and log sender behavior without sending real email.

### Slice 3: Verification Tokens

Add:

* `users.email_verified_at`
* `email_verification_tokens`
* SQLC queries for creating, consuming, and looking up tokens
* service methods for creating and verifying tokens

Token rules:

* Generate at least 32 random bytes.
* Encode the raw token for URLs.
* Store only a hash.
* Expire tokens after 24 hours by default.

### Slice 4: Email Outbox

Add:

* `email_outbox`
* SQLC queries to enqueue, claim, mark sent, and mark failed
* a database-backed outbox store

Registration should enqueue an account confirmation email after the user and token are created.

### Slice 5: Worker

Add a small in-process worker that:

* polls pending outbox rows
* sends through `email.Sender`
* records success and failure
* retries with a simple delay
* stops retrying after a small maximum attempt count
* exits on context cancellation

Start the worker from `cmd/app`.

### Slice 6: SMTP Adapter

Add SMTP as the first real provider because it is standard and does not force a vendor choice.

Keep provider-specific config, authentication, TLS behavior, and errors inside the adapter. The rest of the app should continue to depend only on `email.Sender`.

## Testing Strategy

Use focused tests at each boundary:

* Service tests use fake stores and fake senders.
* Store tests run against SQLite.
* Worker tests use a fake sender and controlled retry timing where practical.
* Route tests assert user-facing behavior and redirects, not provider calls.
* Provider adapter tests should avoid real network calls by default.

For local development, the log sender makes email delivery attempts visible in application logs. By default it does not log message bodies or raw confirmation tokens. Set `EMAIL_LOG_BODY=true` locally when you want clickable confirmation links in the logs.

## Error Handling

Email delivery should not make registration fail after the user has been created. If outbox enqueueing fails, return an internal server error before telling the user registration succeeded.

Provider send failures should be stored on the outbox row and retried by the worker. Logs should include enough context to debug delivery issues without logging raw verification tokens.

Confirmation token errors should be user-safe. Expired, missing, malformed, or already-used tokens can all render the same invalid confirmation response.

## Security Notes

* Store verification token hashes, not raw tokens.
* Use short-lived verification tokens, such as 24 hours.
* Do not log raw tokens.
* Do not expose whether an email address exists in resend flows.
* Use HTTPS in production so confirmation URLs are protected in transit.
* Keep provider credentials out of Git and load them from environment variables or deployment secrets.

## When To Revisit

Reconsider the in-process worker if:

* the app runs multiple instances,
* email volume grows significantly,
* retries need stronger scheduling guarantees,
* delivery needs operational dashboards,
* or the app already depends on an external queue.

Until then, a database outbox plus a small worker is the simplest durable approach for this starter.
