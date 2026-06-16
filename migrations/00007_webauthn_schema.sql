-- +goose Up
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
