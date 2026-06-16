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

CREATE TABLE email_outbox (
    id INTEGER PRIMARY KEY,
    sender TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    text_body TEXT NOT NULL,
    html_body TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    available_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    claimed_at TIMESTAMP,
    claim_expires_at TIMESTAMP,
    claim_token TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX email_outbox_pending_idx ON email_outbox(status, available_at);
CREATE INDEX email_outbox_claim_expiry_idx ON email_outbox(status, claim_expires_at);

CREATE TABLE user_totp (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL UNIQUE,
    secret TEXT NOT NULL,
    enabled_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- Most recent accepted TOTP time-step counter so a captured code cannot
    -- be replayed within its validity window (RFC 6238 section 5.2).
    last_used_counter INTEGER,
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

CREATE TABLE webauthn_credentials (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    -- Raw credential ID returned by the authenticator; the lookup key during
    -- discoverable (passkey) login.
    credential_id BLOB NOT NULL UNIQUE,
    -- COSE-encoded public key used to verify assertion signatures.
    public_key BLOB NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    aaguid BLOB,
    -- Signature counter for clone detection (§6.1.1). Many synced passkeys
    -- always report 0, so a non-increasing counter is logged, not rejected.
    sign_count INTEGER NOT NULL DEFAULT 0,
    -- JSON array of authenticator transports (e.g. ["internal","hybrid"]).
    transports TEXT NOT NULL DEFAULT '[]',
    -- BE flag: credential can be backed up / synced. Immutable once set.
    backup_eligible INTEGER NOT NULL DEFAULT 0,
    -- BS flag: credential is currently backed up / synced. May change per login.
    backup_state INTEGER NOT NULL DEFAULT 0,
    -- User-supplied label shown on the passkey management page.
    name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX webauthn_credentials_user_id_idx ON webauthn_credentials(user_id);

-- +goose Down
DROP INDEX webauthn_credentials_user_id_idx;
DROP TABLE webauthn_credentials;

DROP INDEX totp_backup_codes_user_id_idx;
DROP TABLE totp_backup_codes;
DROP TABLE user_totp;

DROP INDEX email_outbox_claim_expiry_idx;
DROP INDEX email_outbox_pending_idx;
DROP TABLE email_outbox;

DROP INDEX email_change_tokens_new_email_idx;
DROP INDEX email_change_tokens_token_hash_idx;
DROP INDEX email_change_tokens_user_id_idx;
DROP TABLE email_change_tokens;

DROP INDEX password_reset_tokens_token_hash_idx;
DROP INDEX password_reset_tokens_user_id_idx;
DROP TABLE password_reset_tokens;

DROP INDEX email_verification_tokens_token_hash_idx;
DROP INDEX email_verification_tokens_user_id_idx;
DROP TABLE email_verification_tokens;

DROP INDEX sessions_expires_at_idx;
DROP INDEX sessions_token_hash_idx;
DROP INDEX sessions_user_id_idx;
DROP TABLE sessions;

DROP TABLE users;
