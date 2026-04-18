-- +goose Up
CREATE TABLE email_change_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    new_email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX email_change_tokens_user_id_idx ON email_change_tokens(user_id);
CREATE INDEX email_change_tokens_token_hash_idx ON email_change_tokens(token_hash);
CREATE INDEX email_change_tokens_new_email_idx ON email_change_tokens(new_email);

-- +goose Down
DROP INDEX email_change_tokens_new_email_idx;
DROP INDEX email_change_tokens_token_hash_idx;
DROP INDEX email_change_tokens_user_id_idx;
DROP TABLE email_change_tokens;
