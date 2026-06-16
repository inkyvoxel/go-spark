-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    email_verified_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Opaque, stable 64-byte random handle used as the WebAuthn user ID
    -- (§5.4.3). Kept separate from the integer primary key so the value the
    -- authenticator stores contains no PII and is not enumerable.
    webauthn_user_handle BLOB NOT NULL UNIQUE
);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_token_hash_idx ON sessions(token_hash);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

-- +goose Down
DROP INDEX sessions_expires_at_idx;
DROP INDEX sessions_token_hash_idx;
DROP INDEX sessions_user_id_idx;
DROP TABLE sessions;

DROP TABLE users;
