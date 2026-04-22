-- name: CreateUser :one
INSERT INTO users (
    email,
    password_hash
) VALUES (
    ?,
    ?
)
RETURNING id, email, password_hash, created_at, email_verified_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at, email_verified_at
FROM users
WHERE email = ?
LIMIT 1;

-- name: GetUserByID :one
SELECT id, email, password_hash, created_at, email_verified_at
FROM users
WHERE id = ?
LIMIT 1;

-- name: GetUserBySessionTokenHash :one
SELECT users.id, users.email, users.password_hash, users.created_at, users.email_verified_at
FROM users
JOIN sessions ON sessions.user_id = users.id
WHERE sessions.token_hash = ?
  AND sessions.expires_at > CURRENT_TIMESTAMP
LIMIT 1;

-- name: CreateSession :one
INSERT INTO sessions (
    user_id,
    token_hash,
    expires_at
) VALUES (
    ?,
    ?,
    ?
)
RETURNING id, user_id, token_hash, expires_at, created_at;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions
WHERE token_hash = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at <= ?;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions
WHERE user_id = ?;

-- name: ListActiveSessionsByUserID :many
SELECT id, user_id, token_hash, expires_at, created_at
FROM sessions
WHERE user_id = ?
  AND expires_at > CURRENT_TIMESTAMP
ORDER BY created_at DESC, id DESC;

-- name: DeleteOtherSessionsByUserIDAndTokenHash :execrows
DELETE FROM sessions
WHERE user_id = ?
  AND token_hash <> ?;

-- name: DeleteSessionByIDAndUserIDAndTokenHashNot :execrows
DELETE FROM sessions
WHERE id = ?
  AND user_id = ?
  AND token_hash <> ?;

-- name: UpdateUserPasswordHash :exec
UPDATE users
SET password_hash = ?
WHERE id = ?;

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

-- name: MarkUserEmailVerified :one
UPDATE users
SET email_verified_at = ?
WHERE id = ?
RETURNING id, email, password_hash, created_at, email_verified_at;

-- name: UpdateUserEmail :one
UPDATE users
SET email = ?,
    email_verified_at = ?
WHERE id = ?
RETURNING id, email, password_hash, created_at, email_verified_at;
