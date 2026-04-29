-- name: CreatePasswordResetToken :one
INSERT INTO password_reset_tokens (
    user_id,
    token_hash,
    expires_at
) VALUES (
    ?,
    ?,
    ?
)
RETURNING id, user_id, token_hash, expires_at, consumed_at, created_at;

-- name: GetValidPasswordResetTokenByHash :one
SELECT id, user_id, token_hash, expires_at, consumed_at, created_at
FROM password_reset_tokens
WHERE token_hash = ?
  AND consumed_at IS NULL
  AND expires_at > ?
LIMIT 1;

-- name: ConsumePasswordResetToken :one
UPDATE password_reset_tokens
SET consumed_at = ?
WHERE token_hash = ?
  AND consumed_at IS NULL
  AND expires_at > ?
RETURNING id, user_id, token_hash, expires_at, consumed_at, created_at;

-- name: PrunePasswordResetTokens :execrows
DELETE FROM password_reset_tokens
WHERE expires_at <= sqlc.arg(expired_before)
   OR (consumed_at IS NOT NULL AND consumed_at <= sqlc.arg(consumed_before));
