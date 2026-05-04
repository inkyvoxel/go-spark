-- name: GetTOTPByUserID :one
SELECT id, user_id, secret, enabled_at, created_at
FROM user_totp
WHERE user_id = ?
LIMIT 1;

-- name: GetEnabledTOTPByUserID :one
SELECT id, user_id, secret, enabled_at, created_at
FROM user_totp
WHERE user_id = ?
  AND enabled_at IS NOT NULL
LIMIT 1;

-- name: UpsertPendingTOTP :exec
INSERT INTO user_totp (user_id, secret)
VALUES (?, ?)
ON CONFLICT(user_id) DO UPDATE SET secret = excluded.secret, enabled_at = NULL;

-- name: EnableTOTP :exec
UPDATE user_totp
SET enabled_at = ?
WHERE user_id = ?
  AND enabled_at IS NULL;

-- name: DeleteTOTPByUserID :exec
DELETE FROM user_totp
WHERE user_id = ?;

-- name: CreateTOTPBackupCode :exec
INSERT INTO totp_backup_codes (user_id, code_hash)
VALUES (?, ?);

-- name: DeleteTOTPBackupCodesByUserID :exec
DELETE FROM totp_backup_codes
WHERE user_id = ?;

-- name: ConsumeTOTPBackupCode :execrows
UPDATE totp_backup_codes
SET used_at = ?
WHERE user_id = ?
  AND code_hash = ?
  AND used_at IS NULL;

-- name: CountUnusedTOTPBackupCodes :one
SELECT COUNT(*) FROM totp_backup_codes
WHERE user_id = ?
  AND used_at IS NULL;
