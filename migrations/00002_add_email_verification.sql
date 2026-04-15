-- +goose Up
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMP;

CREATE TABLE email_verification_tokens (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX email_verification_tokens_user_id_idx ON email_verification_tokens(user_id);
CREATE INDEX email_verification_tokens_token_hash_idx ON email_verification_tokens(token_hash);

-- +goose Down
DROP INDEX email_verification_tokens_token_hash_idx;
DROP INDEX email_verification_tokens_user_id_idx;
DROP TABLE email_verification_tokens;
ALTER TABLE users DROP COLUMN email_verified_at;
