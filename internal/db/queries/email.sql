-- name: EnqueueEmail :one
INSERT INTO email_outbox (
    sender,
    recipient,
    subject,
    text_body,
    html_body,
    available_at
) VALUES (
    ?,
    ?,
    ?,
    ?,
    ?,
    ?
)
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, sent_at, created_at;

-- name: ClaimPendingEmails :many
UPDATE email_outbox
SET status = 'sending'
WHERE id IN (
    SELECT id
    FROM email_outbox AS pending_email
    WHERE pending_email.status = 'pending'
      AND pending_email.available_at <= ?
    ORDER BY pending_email.available_at, pending_email.id
    LIMIT ?
)
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, sent_at, created_at;

-- name: MarkEmailSent :one
UPDATE email_outbox
SET status = 'sent',
    sent_at = ?
WHERE id = ?
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, sent_at, created_at;

-- name: MarkEmailFailed :one
UPDATE email_outbox
SET status = 'pending',
    attempts = attempts + 1,
    last_error = ?,
    available_at = ?
WHERE id = ?
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, sent_at, created_at;
