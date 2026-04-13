-- name: CreateUser :one
INSERT INTO users (
    email,
    password_hash
) VALUES (
    ?,
    ?
)
RETURNING id, email, password_hash, created_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at
FROM users
WHERE email = ?
LIMIT 1;

-- name: GetUserBySessionToken :one
SELECT users.id, users.email, users.password_hash, users.created_at
FROM users
JOIN sessions ON sessions.user_id = users.id
WHERE sessions.token = ?
  AND sessions.expires_at > CURRENT_TIMESTAMP
LIMIT 1;

-- name: CreateSession :one
INSERT INTO sessions (
    user_id,
    token,
    expires_at
) VALUES (
    ?,
    ?,
    ?
)
RETURNING id, user_id, token, expires_at, created_at;

-- name: DeleteSessionByToken :exec
DELETE FROM sessions
WHERE token = ?;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at <= CURRENT_TIMESTAMP;
