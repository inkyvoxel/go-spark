-- +goose Up
-- Example feature. Demonstrates the migration -> sqlc -> service ->
-- handler -> template path described in docs/extending.md. Safe to delete along
-- with the rest of the "projects" example (see the header comment in
-- internal/server/project_handlers.go for the full list of files to remove).
CREATE TABLE projects (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- ON DELETE CASCADE lets account deletion remove a user's rows
    -- automatically. Every user-owned table should declare it; see
    -- docs/extending.md ("Account deletion").
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX projects_user_id_idx ON projects(user_id);

-- +goose Down
DROP INDEX projects_user_id_idx;
DROP TABLE projects;
