package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
)

// webAuthnUserHandleBytes is the length of the opaque per-user WebAuthn handle.
// The spec allows up to 64 bytes and recommends using the full length.
const webAuthnUserHandleBytes = 64

// newWebAuthnUserHandle generates a random, stable user handle stored alongside
// each user and used as the WebAuthn user ID.
func newWebAuthnUserHandle() ([]byte, error) {
	handle := make([]byte, webAuthnUserHandleBytes)
	if _, err := rand.Read(handle); err != nil {
		return nil, fmt.Errorf("generate webauthn user handle: %w", err)
	}
	return handle, nil
}

func (s *AuthStore) GetWebAuthnHandleByUserID(ctx context.Context, userID int64) ([]byte, error) {
	handle, err := s.queries.GetWebAuthnHandleByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get webauthn handle by user ID: %w", err)
	}
	return handle, nil
}

func (s *AuthStore) GetUserByWebAuthnHandle(ctx context.Context, handle []byte) (services.User, error) {
	user, err := s.queries.GetUserByWebAuthnHandle(ctx, handle)
	if err != nil {
		return services.User{}, fmt.Errorf("get user by webauthn handle: %w", err)
	}
	return services.User{
		ID:              user.ID,
		Email:           user.Email,
		EmailVerifiedAt: user.EmailVerifiedAt,
		CreatedAt:       user.CreatedAt,
	}, nil
}

func (s *AuthStore) CreateWebAuthnCredential(ctx context.Context, params services.CreateWebAuthnCredentialParams) error {
	transports, err := json.Marshal(params.Transports)
	if err != nil {
		return fmt.Errorf("marshal webauthn transports: %w", err)
	}
	if err := s.queries.CreateWebAuthnCredential(ctx, db.CreateWebAuthnCredentialParams{
		UserID:          params.UserID,
		CredentialID:    params.CredentialID,
		PublicKey:       params.PublicKey,
		AttestationType: params.AttestationType,
		Aaguid:          params.AAGUID,
		SignCount:       int64(params.SignCount),
		Transports:      string(transports),
		BackupEligible:  boolToInt64(params.BackupEligible),
		BackupState:     boolToInt64(params.BackupState),
		Name:            params.Name,
	}); err != nil {
		return fmt.Errorf("create webauthn credential: %w", err)
	}
	return nil
}

func (s *AuthStore) ListWebAuthnCredentialsByUserID(ctx context.Context, userID int64) ([]services.WebAuthnCredential, error) {
	rows, err := s.queries.ListWebAuthnCredentialsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list webauthn credentials by user ID: %w", err)
	}
	credentials := make([]services.WebAuthnCredential, 0, len(rows))
	for _, row := range rows {
		credential, err := webAuthnCredentialFromDB(row)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func (s *AuthStore) UpdateWebAuthnCredentialOnLogin(ctx context.Context, params services.UpdateWebAuthnCredentialParams) error {
	if err := s.queries.UpdateWebAuthnCredentialOnLogin(ctx, db.UpdateWebAuthnCredentialOnLoginParams{
		SignCount:    int64(params.SignCount),
		BackupState:  boolToInt64(params.BackupState),
		LastUsedAt:   sql.NullTime{Time: params.LastUsedAt, Valid: true},
		CredentialID: params.CredentialID,
	}); err != nil {
		return fmt.Errorf("update webauthn credential on login: %w", err)
	}
	return nil
}

func (s *AuthStore) RenameWebAuthnCredential(ctx context.Context, userID, credentialDBID int64, name string) (bool, error) {
	rows, err := s.queries.RenameWebAuthnCredential(ctx, db.RenameWebAuthnCredentialParams{
		Name:   name,
		ID:     credentialDBID,
		UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("rename webauthn credential: %w", err)
	}
	return rows > 0, nil
}

func (s *AuthStore) DeleteWebAuthnCredential(ctx context.Context, userID, credentialDBID int64) (bool, error) {
	rows, err := s.queries.DeleteWebAuthnCredentialByIDAndUserID(ctx, db.DeleteWebAuthnCredentialByIDAndUserIDParams{
		ID:     credentialDBID,
		UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("delete webauthn credential: %w", err)
	}
	return rows > 0, nil
}

func (s *AuthStore) CountWebAuthnCredentialsByUserID(ctx context.Context, userID int64) (int64, error) {
	count, err := s.queries.CountWebAuthnCredentialsByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count webauthn credentials by user ID: %w", err)
	}
	return count, nil
}

func webAuthnCredentialFromDB(row db.WebauthnCredential) (services.WebAuthnCredential, error) {
	var transports []string
	if row.Transports != "" {
		if err := json.Unmarshal([]byte(row.Transports), &transports); err != nil {
			return services.WebAuthnCredential{}, fmt.Errorf("unmarshal webauthn transports: %w", err)
		}
	}
	return services.WebAuthnCredential{
		ID:              row.ID,
		UserID:          row.UserID,
		CredentialID:    row.CredentialID,
		PublicKey:       row.PublicKey,
		AttestationType: row.AttestationType,
		AAGUID:          row.Aaguid,
		SignCount:       uint32(row.SignCount),
		Transports:      transports,
		BackupEligible:  row.BackupEligible != 0,
		BackupState:     row.BackupState != 0,
		Name:            row.Name,
		CreatedAt:       row.CreatedAt,
		LastUsedAt:      row.LastUsedAt,
	}, nil
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
