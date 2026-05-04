package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/inkyvoxel/go-spark/internal/totp"
)

const totpBackupCodeCount = 8

// BeginTOTPSetup generates a new TOTP secret, stores it as pending, and
// returns the raw secret and otpauth:// URI for QR code rendering.
func (s *AuthService) BeginTOTPSetup(ctx context.Context, userID int64) (secret, otpAuthURI string, err error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrInvalidCredentials
	}
	if err != nil {
		return "", "", fmt.Errorf("get user: %w", err)
	}

	existing, err := s.store.GetTOTPByUserID(ctx, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("get TOTP record: %w", err)
	}
	if err == nil && existing.EnabledAt.Valid {
		return "", "", ErrTOTPAlreadyEnabled
	}

	// crypto/rand bytes → base32 secret via totp package's BuildURI helper.
	// We generate the secret manually here so we can store it and return it.
	rawSecret, err := generateTOTPSecret()
	if err != nil {
		return "", "", fmt.Errorf("generate TOTP secret: %w", err)
	}

	if err := s.store.UpsertPendingTOTP(ctx, userID, rawSecret); err != nil {
		return "", "", fmt.Errorf("store pending TOTP: %w", err)
	}

	uri := totp.BuildURI(s.totpIssuer, user.Email, rawSecret)
	return rawSecret, uri, nil
}

// GetTOTPSetupInfo returns the secret and otpauth URI for an in-progress
// (not yet confirmed) TOTP setup. Returns ErrTOTPSetupNotStarted if no
// pending setup exists.
func (s *AuthService) GetTOTPSetupInfo(ctx context.Context, userID int64) (secret, otpAuthURI string, err error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrInvalidCredentials
	}
	if err != nil {
		return "", "", fmt.Errorf("get user: %w", err)
	}

	record, err := s.store.GetTOTPByUserID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) || record.EnabledAt.Valid {
		return "", "", ErrTOTPSetupNotStarted
	}
	if err != nil {
		return "", "", fmt.Errorf("get TOTP record: %w", err)
	}

	uri := totp.BuildURI(s.totpIssuer, user.Email, record.Secret)
	return record.Secret, uri, nil
}

// ConfirmTOTPSetup verifies the user's first TOTP code, enables 2FA, generates
// backup codes, and returns the plaintext codes (shown once).
func (s *AuthService) ConfirmTOTPSetup(ctx context.Context, userID int64, code string) ([]string, error) {
	record, err := s.store.GetTOTPByUserID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTOTPSetupNotStarted
	}
	if err != nil {
		return nil, fmt.Errorf("get TOTP record: %w", err)
	}
	if record.EnabledAt.Valid {
		return nil, ErrTOTPAlreadyEnabled
	}

	if !totp.Verify(record.Secret, code) {
		return nil, ErrInvalidTOTPCode
	}

	codes, hashes, err := generateBackupCodes(totpBackupCodeCount)
	if err != nil {
		return nil, fmt.Errorf("generate backup codes: %w", err)
	}

	if err := s.store.EnableTOTPWithBackupCodes(ctx, userID, time.Now().UTC(), hashes); err != nil {
		return nil, fmt.Errorf("enable TOTP: %w", err)
	}

	return codes, nil
}

// DisableTOTP disables 2FA for the user after verifying a valid TOTP code.
func (s *AuthService) DisableTOTP(ctx context.Context, userID int64, code string) error {
	record, err := s.store.GetEnabledTOTPByUserID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTOTPNotEnabled
	}
	if err != nil {
		return fmt.Errorf("get TOTP: %w", err)
	}

	if !totp.Verify(record.Secret, code) {
		return ErrInvalidTOTPCode
	}

	if err := s.store.DeleteTOTPAndBackupCodes(ctx, userID); err != nil {
		return fmt.Errorf("delete TOTP: %w", err)
	}

	return nil
}

// GetTOTPStatus returns whether TOTP is enabled and how many backup codes remain.
func (s *AuthService) GetTOTPStatus(ctx context.Context, userID int64) (enabled bool, backupCodesRemaining int, err error) {
	_, err = s.store.GetEnabledTOTPByUserID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("get TOTP: %w", err)
	}

	count, err := s.store.CountUnusedTOTPBackupCodes(ctx, userID)
	if err != nil {
		return false, 0, fmt.Errorf("count backup codes: %w", err)
	}

	return true, int(count), nil
}

// TOTPSetupState reports the pending/enabled state of a user's TOTP configuration.
func (s *AuthService) TOTPSetupState(ctx context.Context, userID int64) (pending, enabled bool, err error) {
	record, err := s.store.GetTOTPByUserID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("get TOTP: %w", err)
	}
	return !record.EnabledAt.Valid, record.EnabledAt.Valid, nil
}

// VerifyTOTPLogin verifies a TOTP code or backup code after password auth succeeds,
// then creates and returns the session.
func (s *AuthService) VerifyTOTPLogin(ctx context.Context, userID int64, code string) (AuthSession, error) {
	record, err := s.store.GetEnabledTOTPByUserID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, ErrTOTPNotEnabled
	}
	if err != nil {
		return AuthSession{}, fmt.Errorf("get TOTP: %w", err)
	}

	if totp.Verify(record.Secret, code) {
		return s.createSessionForUser(ctx, userID)
	}

	codeHash := hashBackupCode(code)
	consumed, err := s.store.ConsumeTOTPBackupCode(ctx, userID, codeHash, time.Now().UTC())
	if err != nil {
		return AuthSession{}, fmt.Errorf("consume backup code: %w", err)
	}
	if consumed {
		return s.createSessionForUser(ctx, userID)
	}

	return AuthSession{}, ErrInvalidTOTPCode
}

// generateTOTPSecret produces a 20-byte cryptographically random base32 secret.
func generateTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.EncodeToString(b), nil
}

// generateBackupCodes creates n one-time backup codes and their SHA256 hashes.
// Each code is 10 uppercase hex characters.
func generateBackupCodes(n int) (codes []string, hashes []string, err error) {
	codes = make([]string, n)
	hashes = make([]string, n)
	for i := range n {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		code := strings.ToUpper(hex.EncodeToString(b))
		codes[i] = code
		hashes[i] = hashBackupCode(code)
	}
	return codes, hashes, nil
}

// hashBackupCode normalises and hashes a backup code for storage/lookup.
func hashBackupCode(code string) string {
	normalised := strings.ToUpper(strings.Join(strings.Fields(code), ""))
	return hashToken(normalised)
}
