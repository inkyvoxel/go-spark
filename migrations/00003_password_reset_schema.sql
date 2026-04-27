-- +goose Up
CREATE TABLE password_reset_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX password_reset_tokens_user_id_idx ON password_reset_tokens(user_id);
CREATE INDEX password_reset_tokens_token_hash_idx ON password_reset_tokens(token_hash);

-- +goose Down
DROP INDEX password_reset_tokens_token_hash_idx;
DROP INDEX password_reset_tokens_user_id_idx;
DROP TABLE password_reset_tokens;
