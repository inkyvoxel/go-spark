package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
)

func (s *authStore) GetTOTPByUserID(ctx context.Context, userID int64) (TOTPRecord, error) {
	row, err := s.queries.GetTOTPByUserID(ctx, userID)
	if err != nil {
		return TOTPRecord{}, fmt.Errorf("get TOTP by user ID: %w", err)
	}
	record := totpRecordFromDB(row)
	if record.Secret, err = s.totpSecrets.decrypt(record.Secret); err != nil {
		return TOTPRecord{}, fmt.Errorf("decrypt TOTP secret: %w", err)
	}
	return record, nil
}

func (s *authStore) GetEnabledTOTPByUserID(ctx context.Context, userID int64) (TOTPRecord, error) {
	row, err := s.queries.GetEnabledTOTPByUserID(ctx, userID)
	if err != nil {
		return TOTPRecord{}, fmt.Errorf("get enabled TOTP by user ID: %w", err)
	}
	record := totpRecordFromDB(row)
	if record.Secret, err = s.totpSecrets.decrypt(record.Secret); err != nil {
		return TOTPRecord{}, fmt.Errorf("decrypt TOTP secret: %w", err)
	}
	return record, nil
}

func (s *authStore) UpsertPendingTOTP(ctx context.Context, userID int64, secret string) error {
	encryptedSecret, err := s.totpSecrets.encrypt(secret)
	if err != nil {
		return fmt.Errorf("encrypt TOTP secret: %w", err)
	}
	if err := s.queries.UpsertPendingTOTP(ctx, db.UpsertPendingTOTPParams{
		UserID: userID,
		Secret: encryptedSecret,
	}); err != nil {
		return fmt.Errorf("upsert pending TOTP: %w", err)
	}
	return nil
}

// EnableTOTPWithBackupCodes atomically marks TOTP as enabled and replaces
// any existing backup codes with the newly generated set.
func (s *authStore) EnableTOTPWithBackupCodes(ctx context.Context, userID int64, enabledAt time.Time, codeHashes []string) error {
	return withTx(ctx, s.db, s.queries, "enable TOTP with backup codes", func(queries *db.Queries) error {
		if err := queries.EnableTOTP(ctx, db.EnableTOTPParams{
			EnabledAt: sql.NullTime{Time: enabledAt, Valid: true},
			UserID:    userID,
		}); err != nil {
			return fmt.Errorf("enable TOTP: %w", err)
		}

		if err := queries.DeleteTOTPBackupCodesByUserID(ctx, userID); err != nil {
			return fmt.Errorf("delete old backup codes: %w", err)
		}

		for _, hash := range codeHashes {
			if err := queries.CreateTOTPBackupCode(ctx, db.CreateTOTPBackupCodeParams{
				UserID:   userID,
				CodeHash: hash,
			}); err != nil {
				return fmt.Errorf("create backup code: %w", err)
			}
		}

		return nil
	})
}

// ReplaceBackupCodes atomically deletes all existing backup codes for the user
// and inserts the new set, leaving the TOTP record itself unchanged.
func (s *authStore) ReplaceBackupCodes(ctx context.Context, userID int64, codeHashes []string) error {
	return withTx(ctx, s.db, s.queries, "replace backup codes", func(queries *db.Queries) error {
		if err := queries.DeleteTOTPBackupCodesByUserID(ctx, userID); err != nil {
			return fmt.Errorf("delete backup codes: %w", err)
		}
		for _, hash := range codeHashes {
			if err := queries.CreateTOTPBackupCode(ctx, db.CreateTOTPBackupCodeParams{
				UserID:   userID,
				CodeHash: hash,
			}); err != nil {
				return fmt.Errorf("create backup code: %w", err)
			}
		}
		return nil
	})
}

// DeleteTOTPAndBackupCodes atomically removes all TOTP and backup code data.
func (s *authStore) DeleteTOTPAndBackupCodes(ctx context.Context, userID int64) error {
	return withTx(ctx, s.db, s.queries, "delete TOTP and backup codes", func(queries *db.Queries) error {
		if err := queries.DeleteTOTPBackupCodesByUserID(ctx, userID); err != nil {
			return fmt.Errorf("delete backup codes: %w", err)
		}
		if err := queries.DeleteTOTPByUserID(ctx, userID); err != nil {
			return fmt.Errorf("delete TOTP: %w", err)
		}
		return nil
	})
}

func (s *authStore) ConsumeTOTPBackupCode(ctx context.Context, userID int64, codeHash string, usedAt time.Time) (bool, error) {
	rows, err := s.queries.ConsumeTOTPBackupCode(ctx, db.ConsumeTOTPBackupCodeParams{
		UsedAt:   sql.NullTime{Time: usedAt, Valid: true},
		UserID:   userID,
		CodeHash: codeHash,
	})
	if err != nil {
		return false, fmt.Errorf("consume TOTP backup code: %w", err)
	}
	return rows > 0, nil
}

// ClaimTOTPCounter records the accepted TOTP time-step counter. It reports
// false when the counter is not strictly newer than the stored one, meaning
// the code was already used and must be rejected as a replay.
func (s *authStore) ClaimTOTPCounter(ctx context.Context, userID, counter int64) (bool, error) {
	rows, err := s.queries.ClaimTOTPCounter(ctx, db.ClaimTOTPCounterParams{
		Counter: sql.NullInt64{Int64: counter, Valid: true},
		UserID:  userID,
	})
	if err != nil {
		return false, fmt.Errorf("claim TOTP counter: %w", err)
	}
	return rows > 0, nil
}

func (s *authStore) CountUnusedTOTPBackupCodes(ctx context.Context, userID int64) (int64, error) {
	count, err := s.queries.CountUnusedTOTPBackupCodes(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count unused TOTP backup codes: %w", err)
	}
	return count, nil
}

func totpRecordFromDB(row db.UserTotp) TOTPRecord {
	return TOTPRecord{
		ID:              row.ID,
		UserID:          row.UserID,
		Secret:          row.Secret,
		EnabledAt:       row.EnabledAt,
		LastUsedCounter: row.LastUsedCounter,
		CreatedAt:       row.CreatedAt,
	}
}
