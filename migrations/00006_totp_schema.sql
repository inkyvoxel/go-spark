-- +goose Up
CREATE TABLE user_totp (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL UNIQUE,
    secret TEXT NOT NULL,
    enabled_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE totp_backup_codes (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    code_hash TEXT NOT NULL UNIQUE,
    used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX totp_backup_codes_user_id_idx ON totp_backup_codes(user_id);

-- +goose Down
DROP INDEX totp_backup_codes_user_id_idx;
DROP TABLE totp_backup_codes;
DROP TABLE user_totp;
