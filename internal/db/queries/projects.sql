-- name: CreateProject :one
INSERT INTO projects (user_id, name)
VALUES (?, ?)
RETURNING id, user_id, name, created_at;

-- name: ListProjectsByUserID :many
SELECT id, user_id, name, created_at
FROM projects
WHERE user_id = ?
ORDER BY created_at DESC, id DESC;

-- name: DeleteProjectByIDAndUserID :execrows
DELETE FROM projects
WHERE id = ? AND user_id = ?;
