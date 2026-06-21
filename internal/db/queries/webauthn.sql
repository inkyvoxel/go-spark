-- name: CreateWebAuthnCredential :exec
INSERT INTO webauthn_credentials (
    user_id,
    credential_id,
    public_key,
    attestation_type,
    aaguid,
    sign_count,
    transports,
    backup_eligible,
    backup_state,
    name
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
);

-- name: ListWebAuthnCredentialsByUserID :many
SELECT id, user_id, credential_id, public_key, attestation_type, aaguid,
       sign_count, transports, backup_eligible, backup_state, name,
       created_at, last_used_at
FROM webauthn_credentials
WHERE user_id = ?
ORDER BY created_at DESC, id DESC;

-- name: UpdateWebAuthnCredentialOnLogin :exec
UPDATE webauthn_credentials
SET sign_count = @sign_count,
    backup_state = @backup_state,
    last_used_at = @last_used_at
WHERE credential_id = @credential_id;

-- name: RenameWebAuthnCredential :execrows
UPDATE webauthn_credentials
SET name = @name
WHERE id = @id
  AND user_id = @user_id;

-- name: DeleteWebAuthnCredentialByIDAndUserID :execrows
DELETE FROM webauthn_credentials
WHERE id = ?
  AND user_id = ?;

-- name: CountWebAuthnCredentialsByUserID :one
SELECT COUNT(*) FROM webauthn_credentials
WHERE user_id = ?;
