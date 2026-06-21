package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type authStore struct {
	db          *sql.DB
	queries     *db.Queries
	totpSecrets *secretCipher
}

// newAuthStore builds the store. totpSecretKey is a 32-byte key (derived from
// SECRET_KEY_BASE) used to encrypt TOTP shared secrets at rest.
func NewAuthStore(conn *sql.DB, totpSecretKey []byte) (*authStore, error) {
	totpSecrets, err := newSecretCipher(totpSecretKey)
	if err != nil {
		return nil, fmt.Errorf("configure TOTP secret cipher: %w", err)
	}
	return &authStore{
		db:          conn,
		queries:     db.New(conn),
		totpSecrets: totpSecrets,
	}, nil
}

func (s *authStore) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	handle, err := newWebAuthnUserHandle()
	if err != nil {
		return User{}, err
	}
	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:              email,
		PasswordHash:       passwordHash,
		WebauthnUserHandle: handle,
	})
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return User{}, ErrEmailAlreadyRegistered
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}

	return userFromCreateUserRow(user), nil
}

func (s *authStore) CreateVerifiedUser(ctx context.Context, email, passwordHash string, verifiedAt time.Time) (User, error) {
	handle, err := newWebAuthnUserHandle()
	if err != nil {
		return User{}, err
	}
	return withTxResult(ctx, s.db, s.queries, "create verified user", func(queries *db.Queries) (User, error) {
		createdUser, err := queries.CreateUser(ctx, db.CreateUserParams{
			Email:              email,
			PasswordHash:       passwordHash,
			WebauthnUserHandle: handle,
		})
		if err != nil {
			if isSQLiteUniqueConstraint(err) {
				return User{}, ErrEmailAlreadyRegistered
			}
			return User{}, fmt.Errorf("create user: %w", err)
		}

		user, err := queries.MarkUserEmailVerified(ctx, db.MarkUserEmailVerifiedParams{
			EmailVerifiedAt: sql.NullTime{Time: verifiedAt, Valid: true},
			ID:              createdUser.ID,
		})
		if err != nil {
			return User{}, fmt.Errorf("mark user email verified: %w", err)
		}

		return userFromMarkUserEmailVerifiedRow(user), nil
	})
}

func (s *authStore) CreateUserWithEmailVerification(ctx context.Context, params CreateUserWithEmailVerificationParams) (User, error) {
	handle, err := newWebAuthnUserHandle()
	if err != nil {
		return User{}, err
	}
	return withTxResult(ctx, s.db, s.queries, "register user", func(queries *db.Queries) (User, error) {
		user, err := queries.CreateUser(ctx, db.CreateUserParams{
			Email:              params.Email,
			PasswordHash:       params.PasswordHash,
			WebauthnUserHandle: handle,
		})
		if err != nil {
			if isSQLiteUniqueConstraint(err) {
				return User{}, ErrEmailAlreadyRegistered
			}
			return User{}, fmt.Errorf("create user: %w", err)
		}

		if _, err := queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
			UserID:    user.ID,
			TokenHash: params.TokenHash,
			ExpiresAt: params.TokenExpiresAt,
		}); err != nil {
			return User{}, fmt.Errorf("create email verification token: %w", err)
		}

		if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
			Sender:      params.ConfirmationEmail.From,
			Recipient:   params.ConfirmationEmail.To,
			Subject:     params.ConfirmationEmail.Subject,
			TextBody:    params.ConfirmationEmail.TextBody,
			HtmlBody:    params.ConfirmationEmail.HTMLBody,
			AvailableAt: params.EmailAvailableAt,
		}); err != nil {
			return User{}, fmt.Errorf("enqueue confirmation email: %w", err)
		}

		return userFromCreateUserRow(user), nil
	})
}

func (s *authStore) GetUserByEmail(ctx context.Context, email string) (UserRecord, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return UserRecord{}, fmt.Errorf("get user by email: %w", err)
	}

	return userRecordFromGetUserByEmailRow(user), nil
}

func (s *authStore) GetUserByID(ctx context.Context, userID int64) (UserRecord, error) {
	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return UserRecord{}, fmt.Errorf("get user by ID: %w", err)
	}

	return userRecordFromGetUserByIDRow(user), nil
}

func (s *authStore) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (SessionRecord, error) {
	session, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return SessionRecord{}, fmt.Errorf("create session: %w", err)
	}

	return sessionRecordFromSession(session), nil
}

func (s *authStore) GetUserBySessionTokenHash(ctx context.Context, tokenHash string) (UserRecord, error) {
	user, err := s.queries.GetUserBySessionTokenHash(ctx, tokenHash)
	if err != nil {
		return UserRecord{}, fmt.Errorf("get user by session token hash: %w", err)
	}

	return userRecordFromGetUserBySessionTokenHashRow(user), nil
}

func (s *authStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	if err := s.queries.DeleteSessionByTokenHash(ctx, tokenHash); err != nil {
		return fmt.Errorf("delete session by token hash: %w", err)
	}

	return nil
}

func (s *authStore) DeleteSessionsByUserID(ctx context.Context, userID int64) error {
	if err := s.queries.DeleteSessionsByUserID(ctx, userID); err != nil {
		return fmt.Errorf("delete sessions by user ID: %w", err)
	}

	return nil
}

func (s *authStore) ListActiveSessionsByUserID(ctx context.Context, userID int64) ([]SessionRecord, error) {
	sessions, err := s.queries.ListActiveSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active sessions by user ID: %w", err)
	}

	records := make([]SessionRecord, 0, len(sessions))
	for _, session := range sessions {
		records = append(records, sessionRecordFromSession(session))
	}
	return records, nil
}

func (s *authStore) DeleteOtherSessionsByUserIDAndTokenHash(ctx context.Context, userID int64, tokenHash string) (int64, error) {
	rows, err := s.queries.DeleteOtherSessionsByUserIDAndTokenHash(ctx, db.DeleteOtherSessionsByUserIDAndTokenHashParams{
		UserID:    userID,
		TokenHash: tokenHash,
	})
	if err != nil {
		return 0, fmt.Errorf("delete other sessions by user ID and token hash: %w", err)
	}

	return rows, nil
}

func (s *authStore) DeleteSessionByIDAndUserIDAndTokenHashNot(ctx context.Context, sessionID, userID int64, tokenHash string) (int64, error) {
	rows, err := s.queries.DeleteSessionByIDAndUserIDAndTokenHashNot(ctx, db.DeleteSessionByIDAndUserIDAndTokenHashNotParams{
		ID:        sessionID,
		UserID:    userID,
		TokenHash: tokenHash,
	})
	if err != nil {
		return 0, fmt.Errorf("delete session by ID and user ID and token hash not: %w", err)
	}

	return rows, nil
}

func (s *authStore) UpdateUserPasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	if err := s.queries.UpdateUserPasswordHash(ctx, db.UpdateUserPasswordHashParams{
		PasswordHash: passwordHash,
		ID:           userID,
	}); err != nil {
		return fmt.Errorf("update user password hash: %w", err)
	}

	return nil
}

func (s *authStore) SetPasswordAndRevokeSessions(ctx context.Context, userID int64, passwordHash string) error {
	return withTx(ctx, s.db, s.queries, "set password and revoke sessions", func(queries *db.Queries) error {
		if err := queries.UpdateUserPasswordHash(ctx, db.UpdateUserPasswordHashParams{
			PasswordHash: passwordHash,
			ID:           userID,
		}); err != nil {
			return fmt.Errorf("update user password hash: %w", err)
		}

		if err := queries.DeleteSessionsByUserID(ctx, userID); err != nil {
			return fmt.Errorf("delete sessions by user ID: %w", err)
		}

		return nil
	})
}

func (s *authStore) CreatePasswordResetToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (PasswordResetToken, error) {
	token, err := s.queries.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return PasswordResetToken{}, fmt.Errorf("create password reset token: %w", err)
	}

	return passwordResetTokenFromDB(token), nil
}

func (s *authStore) GetValidPasswordResetTokenByHash(ctx context.Context, tokenHash string, now time.Time) (PasswordResetToken, error) {
	token, err := s.queries.GetValidPasswordResetTokenByHash(ctx, db.GetValidPasswordResetTokenByHashParams{
		TokenHash: tokenHash,
		ExpiresAt: now,
	})
	if err != nil {
		return PasswordResetToken{}, fmt.Errorf("get valid password reset token by hash: %w", err)
	}

	return passwordResetTokenFromDB(token), nil
}

func (s *authStore) ConsumePasswordResetToken(ctx context.Context, tokenHash string, consumedAt time.Time) (PasswordResetToken, error) {
	token, err := s.queries.ConsumePasswordResetToken(ctx, db.ConsumePasswordResetTokenParams{
		ConsumedAt: sql.NullTime{Time: consumedAt, Valid: true},
		TokenHash:  tokenHash,
		ExpiresAt:  consumedAt,
	})
	if err != nil {
		return PasswordResetToken{}, fmt.Errorf("consume password reset token: %w", err)
	}

	return passwordResetTokenFromDB(token), nil
}

func (s *authStore) RequestPasswordReset(ctx context.Context, params RequestPasswordResetParams) error {
	return withTx(ctx, s.db, s.queries, "password reset request", func(queries *db.Queries) error {
		if _, err := queries.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
			UserID:    params.UserID,
			TokenHash: params.TokenHash,
			ExpiresAt: params.TokenExpiresAt,
		}); err != nil {
			return fmt.Errorf("create password reset token: %w", err)
		}

		if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
			Sender:      params.PasswordResetEmail.From,
			Recipient:   params.PasswordResetEmail.To,
			Subject:     params.PasswordResetEmail.Subject,
			TextBody:    params.PasswordResetEmail.TextBody,
			HtmlBody:    params.PasswordResetEmail.HTMLBody,
			AvailableAt: params.EmailAvailableAt,
		}); err != nil {
			return fmt.Errorf("enqueue password reset email: %w", err)
		}

		return nil
	})
}

func (s *authStore) RequestEmailChange(ctx context.Context, params RequestEmailChangeParams) error {
	return withTx(ctx, s.db, s.queries, "email change request", func(queries *db.Queries) error {
		if _, err := queries.CreateEmailChangeToken(ctx, db.CreateEmailChangeTokenParams{
			UserID:    params.UserID,
			NewEmail:  params.NewEmail,
			TokenHash: params.TokenHash,
			ExpiresAt: params.TokenExpiresAt,
		}); err != nil {
			return fmt.Errorf("create email change token: %w", err)
		}

		if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
			Sender:      params.EmailChangeVerifyEmail.From,
			Recipient:   params.EmailChangeVerifyEmail.To,
			Subject:     params.EmailChangeVerifyEmail.Subject,
			TextBody:    params.EmailChangeVerifyEmail.TextBody,
			HtmlBody:    params.EmailChangeVerifyEmail.HTMLBody,
			AvailableAt: params.EmailAvailableAt,
		}); err != nil {
			return fmt.Errorf("enqueue email change verification email: %w", err)
		}

		return nil
	})
}

func (s *authStore) CreateEmailVerificationToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (EmailVerificationToken, error) {
	token, err := s.queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return EmailVerificationToken{}, fmt.Errorf("create email verification token: %w", err)
	}

	return emailVerificationTokenFromDB(token), nil
}

func (s *authStore) ResendEmailVerification(ctx context.Context, params ResendEmailVerificationParams) error {
	return withTx(ctx, s.db, s.queries, "resend email verification", func(queries *db.Queries) error {
		if _, err := queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
			UserID:    params.UserID,
			TokenHash: params.TokenHash,
			ExpiresAt: params.TokenExpiresAt,
		}); err != nil {
			return fmt.Errorf("create email verification token: %w", err)
		}

		if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
			Sender:      params.ConfirmationEmail.From,
			Recipient:   params.ConfirmationEmail.To,
			Subject:     params.ConfirmationEmail.Subject,
			TextBody:    params.ConfirmationEmail.TextBody,
			HtmlBody:    params.ConfirmationEmail.HTMLBody,
			AvailableAt: params.EmailAvailableAt,
		}); err != nil {
			return fmt.Errorf("enqueue confirmation email: %w", err)
		}

		return nil
	})
}

// DeleteAccount removes the user. Every user-owned table declares
// ON DELETE CASCADE (see migrations), so the database deletes the user's
// sessions, tokens, TOTP, passkeys, and example "projects" rows automatically.
// Add ON DELETE CASCADE to any new user-owned table you introduce and it will
// be cleaned up here for free; see docs/extending.md ("Account deletion").
func (s *authStore) DeleteAccount(ctx context.Context, userID int64) error {
	if err := s.queries.DeleteUserByID(ctx, userID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (s *authStore) VerifyEmailByTokenHash(ctx context.Context, tokenHash string, verifiedAt time.Time) (User, error) {
	return withTxResult(ctx, s.db, s.queries, "verify email", func(queries *db.Queries) (User, error) {
		token, err := queries.ConsumeEmailVerificationToken(ctx, db.ConsumeEmailVerificationTokenParams{
			ConsumedAt: sql.NullTime{Time: verifiedAt, Valid: true},
			TokenHash:  tokenHash,
			ExpiresAt:  verifiedAt,
		})
		if err != nil {
			return User{}, fmt.Errorf("consume email verification token: %w", err)
		}

		user, err := queries.MarkUserEmailVerified(ctx, db.MarkUserEmailVerifiedParams{
			EmailVerifiedAt: sql.NullTime{Time: verifiedAt, Valid: true},
			ID:              token.UserID,
		})
		if err != nil {
			return User{}, fmt.Errorf("mark user email verified: %w", err)
		}

		return userFromMarkUserEmailVerifiedRow(user), nil
	})
}

func userFromCreateUserRow(row db.CreateUserRow) User {
	return User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		CreatedAt:       row.CreatedAt,
	}
}

func userRecordFromGetUserByEmailRow(row db.GetUserByEmailRow) UserRecord {
	return UserRecord{
		User: User{
			ID:              row.ID,
			Email:           row.Email,
			EmailVerifiedAt: row.EmailVerifiedAt,
			CreatedAt:       row.CreatedAt,
		},
		PasswordHash: row.PasswordHash,
	}
}

func userRecordFromGetUserBySessionTokenHashRow(row db.GetUserBySessionTokenHashRow) UserRecord {
	return UserRecord{
		User: User{
			ID:              row.ID,
			Email:           row.Email,
			EmailVerifiedAt: row.EmailVerifiedAt,
			CreatedAt:       row.CreatedAt,
		},
		PasswordHash: row.PasswordHash,
	}
}

func userFromMarkUserEmailVerifiedRow(row db.MarkUserEmailVerifiedRow) User {
	return User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		CreatedAt:       row.CreatedAt,
	}
}

func userFromUpdateUserEmailRow(row db.UpdateUserEmailRow) User {
	return User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		CreatedAt:       row.CreatedAt,
	}
}

func userRecordFromGetUserByIDRow(row db.GetUserByIDRow) UserRecord {
	return UserRecord{
		User: User{
			ID:              row.ID,
			Email:           row.Email,
			EmailVerifiedAt: row.EmailVerifiedAt,
			CreatedAt:       row.CreatedAt,
		},
		PasswordHash: row.PasswordHash,
	}
}

func sessionRecordFromSession(row db.Session) SessionRecord {
	return SessionRecord{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}
}

func emailVerificationTokenFromDB(row db.EmailVerificationToken) EmailVerificationToken {
	return EmailVerificationToken{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func passwordResetTokenFromDB(row db.PasswordResetToken) PasswordResetToken {
	return PasswordResetToken{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func (s *authStore) ConfirmEmailChange(ctx context.Context, params ConfirmEmailChangeParams) (User, error) {
	return withTxResult(ctx, s.db, s.queries, "confirm email change", func(queries *db.Queries) (User, error) {
		token, err := queries.ConsumeEmailChangeToken(ctx, db.ConsumeEmailChangeTokenParams{
			ConsumedAt: sql.NullTime{Time: params.ChangedAt, Valid: true},
			TokenHash:  params.TokenHash,
			ExpiresAt:  params.ChangedAt,
		})
		if err != nil {
			return User{}, fmt.Errorf("consume email change token: %w", err)
		}

		return applyEmailChange(ctx, queries, applyEmailChangeParams{
			UserID:                 token.UserID,
			NewEmail:               token.NewEmail,
			ChangedAt:              params.ChangedAt,
			OldEmailNoticeOptions:  params.OldEmailNoticeOptions,
			NoticeEmailAvailableAt: params.NoticeEmailAvailableAt,
			SendOldEmailNotice:     params.SendOldEmailNotice,
		})
	})
}

type applyEmailChangeParams struct {
	UserID                 int64
	NewEmail               string
	ChangedAt              time.Time
	OldEmailNoticeOptions  email.MessageOptions
	NoticeEmailAvailableAt time.Time
	SendOldEmailNotice     bool
}

func applyEmailChange(ctx context.Context, queries *db.Queries, params applyEmailChangeParams) (User, error) {
	oldUser, err := queries.GetUserByID(ctx, params.UserID)
	if err != nil {
		return User{}, fmt.Errorf("get user by ID: %w", err)
	}

	user, err := queries.UpdateUserEmail(ctx, db.UpdateUserEmailParams{
		Email:           params.NewEmail,
		EmailVerifiedAt: sql.NullTime{Time: params.ChangedAt, Valid: true},
		ID:              params.UserID,
	})
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return User{}, ErrEmailAlreadyRegistered
		}
		return User{}, fmt.Errorf("update user email: %w", err)
	}

	// / Email is a login identifier, so changing it revokes all sessions (same as password change)
	if err := queries.DeleteSessionsByUserID(ctx, params.UserID); err != nil {
		return User{}, fmt.Errorf("delete sessions by user ID: %w", err)
	}

	if params.SendOldEmailNotice {
		notice, err := email.NewEmailChangeNoticeMessage(params.OldEmailNoticeOptions, oldUser.Email)
		if err != nil {
			return User{}, fmt.Errorf("build old email change notice: %w", err)
		}
		if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
			Sender:      notice.From,
			Recipient:   notice.To,
			Subject:     notice.Subject,
			TextBody:    notice.TextBody,
			HtmlBody:    notice.HTMLBody,
			AvailableAt: params.NoticeEmailAvailableAt,
		}); err != nil {
			return User{}, fmt.Errorf("enqueue old email change notice: %w", err)
		}
	}

	return userFromUpdateUserEmailRow(user), nil
}

func isSQLiteUniqueConstraint(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}

	if sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return true
	}

	return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT && strings.Contains(strings.ToLower(sqliteErr.Error()), "unique")
}
