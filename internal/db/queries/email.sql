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
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, claimed_at, claim_expires_at, claim_token, sent_at, created_at;

-- name: GetEmailOutboxRow :one
SELECT id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, claimed_at, claim_expires_at, claim_token, sent_at, created_at
FROM email_outbox
WHERE id = ?
LIMIT 1;

-- name: ClaimPendingEmails :many
UPDATE email_outbox
SET status = 'sending',
    claimed_at = sqlc.arg(now),
    claim_expires_at = sqlc.arg(claim_expires_at),
    claim_token = sqlc.arg(claim_token)
WHERE id IN (
    SELECT id
    FROM email_outbox AS outbox
    WHERE (
        outbox.status = 'pending'
        AND outbox.available_at <= sqlc.arg(now)
    ) OR (
        outbox.status = 'sending'
        AND outbox.claim_expires_at IS NOT NULL
        AND outbox.claim_expires_at <= sqlc.arg(now)
    )
    ORDER BY outbox.available_at, outbox.id
    LIMIT sqlc.arg(limit)
)
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, claimed_at, claim_expires_at, claim_token, sent_at, created_at;

-- name: MarkEmailSent :one
UPDATE email_outbox
SET status = 'sent',
    sent_at = sqlc.arg(sent_at),
    claimed_at = NULL,
    claim_expires_at = NULL,
    claim_token = ''
WHERE id = sqlc.arg(id)
  AND claim_token = sqlc.arg(claim_token)
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, claimed_at, claim_expires_at, claim_token, sent_at, created_at;

-- name: MarkEmailFailed :one
UPDATE email_outbox
SET status = 'pending',
    attempts = attempts + 1,
    last_error = sqlc.arg(last_error),
    available_at = sqlc.arg(available_at),
    claimed_at = NULL,
    claim_expires_at = NULL,
    claim_token = ''
WHERE id = sqlc.arg(id)
  AND claim_token = sqlc.arg(claim_token)
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, claimed_at, claim_expires_at, claim_token, sent_at, created_at;

-- name: MarkEmailFailedPermanently :one
UPDATE email_outbox
SET status = 'failed',
    attempts = attempts + 1,
    last_error = sqlc.arg(last_error),
    available_at = sqlc.arg(available_at),
    claimed_at = NULL,
    claim_expires_at = NULL,
    claim_token = ''
WHERE id = sqlc.arg(id)
  AND claim_token = sqlc.arg(claim_token)
RETURNING id, sender, recipient, subject, text_body, html_body, status, attempts, last_error, available_at, claimed_at, claim_expires_at, claim_token, sent_at, created_at;

-- name: PruneSentEmailOutboxRows :execrows
DELETE FROM email_outbox
WHERE status = 'sent'
  AND sent_at IS NOT NULL
  AND sent_at <= ?;

-- name: PruneFailedEmailOutboxRows :execrows
DELETE FROM email_outbox
WHERE status = 'failed'
  AND available_at <= ?;
