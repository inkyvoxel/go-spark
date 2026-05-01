-- name: CreateEmailVerificationToken :one
INSERT INTO email_verification_tokens (
    user_id,
    token_hash,
    expires_at
) VALUES (
    ?,
    ?,
    ?
)
RETURNING id, user_id, token_hash, expires_at, consumed_at, created_at;

-- name: ConsumeEmailVerificationToken :one
UPDATE email_verification_tokens
SET consumed_at = ?
WHERE token_hash = ?
  AND consumed_at IS NULL
  AND expires_at > ?
RETURNING id, user_id, token_hash, expires_at, consumed_at, created_at;

-- name: PruneEmailVerificationTokens :execrows
DELETE FROM email_verification_tokens
WHERE expires_at <= sqlc.arg(expired_before)
   OR (consumed_at IS NOT NULL AND consumed_at <= sqlc.arg(consumed_before));

-- name: DeleteEmailVerificationTokensByUserID :exec
DELETE FROM email_verification_tokens
WHERE user_id = ?;

-- name: MarkUserEmailVerified :one
UPDATE users
SET email_verified_at = ?
WHERE id = ?
RETURNING id, email, password_hash, created_at, email_verified_at;
