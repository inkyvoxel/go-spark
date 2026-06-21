-- name: CreateEmailChangeToken :one
INSERT INTO email_change_tokens (
    user_id,
    new_email,
    token_hash,
    expires_at
) VALUES (
    ?,
    ?,
    ?,
    ?
)
RETURNING id, user_id, new_email, token_hash, expires_at, consumed_at, created_at;

-- name: ConsumeEmailChangeToken :one
UPDATE email_change_tokens
SET consumed_at = ?
WHERE token_hash = ?
  AND consumed_at IS NULL
  AND expires_at > ?
RETURNING id, user_id, new_email, token_hash, expires_at, consumed_at, created_at;

-- name: PruneEmailChangeTokens :execrows
DELETE FROM email_change_tokens
WHERE expires_at <= sqlc.arg(expired_before)
   OR (consumed_at IS NOT NULL AND consumed_at <= sqlc.arg(consumed_before));

-- name: UpdateUserEmail :one
UPDATE users
SET email = ?,
    email_verified_at = ?
WHERE id = ?
RETURNING id, email, password_hash, created_at, email_verified_at;
