package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/email"
	"github.com/inkyvoxel/go-spark/internal/services"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

type AuthStore struct {
	db      *sql.DB
	queries *db.Queries
}

var _ services.AuthStore = (*AuthStore)(nil)

func NewAuthStore(conn *sql.DB) *AuthStore {
	return &AuthStore{
		db:      conn,
		queries: db.New(conn),
	}
}

func (s *AuthStore) CreateUser(ctx context.Context, email, passwordHash string) (services.User, error) {
	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return services.User{}, services.ErrEmailAlreadyRegistered
		}
		return services.User{}, fmt.Errorf("create user: %w", err)
	}

	return userFromCreateUserRow(user), nil
}

func (s *AuthStore) CreateVerifiedUser(ctx context.Context, email, passwordHash string, verifiedAt time.Time) (services.User, error) {
	return withTxResult(ctx, s.db, s.queries, "create verified user", func(queries *db.Queries) (services.User, error) {
		createdUser, err := queries.CreateUser(ctx, db.CreateUserParams{
			Email:        email,
			PasswordHash: passwordHash,
		})
		if err != nil {
			if isSQLiteUniqueConstraint(err) {
				return services.User{}, services.ErrEmailAlreadyRegistered
			}
			return services.User{}, fmt.Errorf("create user: %w", err)
		}

		user, err := queries.MarkUserEmailVerified(ctx, db.MarkUserEmailVerifiedParams{
			EmailVerifiedAt: sql.NullTime{Time: verifiedAt, Valid: true},
			ID:              createdUser.ID,
		})
		if err != nil {
			return services.User{}, fmt.Errorf("mark user email verified: %w", err)
		}

		return userFromMarkUserEmailVerifiedRow(user), nil
	})
}

func (s *AuthStore) CreateUserWithEmailVerification(ctx context.Context, params services.CreateUserWithEmailVerificationParams) (services.User, error) {
	return withTxResult(ctx, s.db, s.queries, "register user", func(queries *db.Queries) (services.User, error) {
		user, err := queries.CreateUser(ctx, db.CreateUserParams{
			Email:        params.Email,
			PasswordHash: params.PasswordHash,
		})
		if err != nil {
			if isSQLiteUniqueConstraint(err) {
				return services.User{}, services.ErrEmailAlreadyRegistered
			}
			return services.User{}, fmt.Errorf("create user: %w", err)
		}

		if _, err := queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
			UserID:    user.ID,
			TokenHash: params.TokenHash,
			ExpiresAt: params.TokenExpiresAt,
		}); err != nil {
			return services.User{}, fmt.Errorf("create email verification token: %w", err)
		}

		if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
			Sender:      params.ConfirmationEmail.From,
			Recipient:   params.ConfirmationEmail.To,
			Subject:     params.ConfirmationEmail.Subject,
			TextBody:    params.ConfirmationEmail.TextBody,
			HtmlBody:    params.ConfirmationEmail.HTMLBody,
			AvailableAt: params.EmailAvailableAt,
		}); err != nil {
			return services.User{}, fmt.Errorf("enqueue confirmation email: %w", err)
		}

		return userFromCreateUserRow(user), nil
	})
}

func (s *AuthStore) GetUserByEmail(ctx context.Context, email string) (services.UserRecord, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return services.UserRecord{}, fmt.Errorf("get user by email: %w", err)
	}

	return userRecordFromGetUserByEmailRow(user), nil
}

func (s *AuthStore) GetUserByID(ctx context.Context, userID int64) (services.UserRecord, error) {
	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return services.UserRecord{}, fmt.Errorf("get user by ID: %w", err)
	}

	return userRecordFromGetUserByIDRow(user), nil
}

func (s *AuthStore) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (services.SessionRecord, error) {
	session, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return services.SessionRecord{}, fmt.Errorf("create session: %w", err)
	}

	return sessionRecordFromSession(session), nil
}

func (s *AuthStore) GetUserBySessionTokenHash(ctx context.Context, tokenHash string) (services.UserRecord, error) {
	user, err := s.queries.GetUserBySessionTokenHash(ctx, tokenHash)
	if err != nil {
		return services.UserRecord{}, fmt.Errorf("get user by session token hash: %w", err)
	}

	return userRecordFromGetUserBySessionTokenHashRow(user), nil
}

func (s *AuthStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	if err := s.queries.DeleteSessionByTokenHash(ctx, tokenHash); err != nil {
		return fmt.Errorf("delete session by token hash: %w", err)
	}

	return nil
}

func (s *AuthStore) DeleteSessionsByUserID(ctx context.Context, userID int64) error {
	if err := s.queries.DeleteSessionsByUserID(ctx, userID); err != nil {
		return fmt.Errorf("delete sessions by user ID: %w", err)
	}

	return nil
}

func (s *AuthStore) ListActiveSessionsByUserID(ctx context.Context, userID int64) ([]services.SessionRecord, error) {
	sessions, err := s.queries.ListActiveSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active sessions by user ID: %w", err)
	}

	records := make([]services.SessionRecord, 0, len(sessions))
	for _, session := range sessions {
		records = append(records, sessionRecordFromSession(session))
	}
	return records, nil
}

func (s *AuthStore) DeleteOtherSessionsByUserIDAndTokenHash(ctx context.Context, userID int64, tokenHash string) (int64, error) {
	rows, err := s.queries.DeleteOtherSessionsByUserIDAndTokenHash(ctx, db.DeleteOtherSessionsByUserIDAndTokenHashParams{
		UserID:    userID,
		TokenHash: tokenHash,
	})
	if err != nil {
		return 0, fmt.Errorf("delete other sessions by user ID and token hash: %w", err)
	}

	return rows, nil
}

func (s *AuthStore) DeleteSessionByIDAndUserIDAndTokenHashNot(ctx context.Context, sessionID, userID int64, tokenHash string) (int64, error) {
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

func (s *AuthStore) UpdateUserPasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	if err := s.queries.UpdateUserPasswordHash(ctx, db.UpdateUserPasswordHashParams{
		PasswordHash: passwordHash,
		ID:           userID,
	}); err != nil {
		return fmt.Errorf("update user password hash: %w", err)
	}

	return nil
}

func (s *AuthStore) SetPasswordAndRevokeSessions(ctx context.Context, userID int64, passwordHash string) error {
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

func (s *AuthStore) CreatePasswordResetToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (services.PasswordResetToken, error) {
	token, err := s.queries.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return services.PasswordResetToken{}, fmt.Errorf("create password reset token: %w", err)
	}

	return passwordResetTokenFromDB(token), nil
}

func (s *AuthStore) GetValidPasswordResetTokenByHash(ctx context.Context, tokenHash string, now time.Time) (services.PasswordResetToken, error) {
	token, err := s.queries.GetValidPasswordResetTokenByHash(ctx, db.GetValidPasswordResetTokenByHashParams{
		TokenHash: tokenHash,
		ExpiresAt: now,
	})
	if err != nil {
		return services.PasswordResetToken{}, fmt.Errorf("get valid password reset token by hash: %w", err)
	}

	return passwordResetTokenFromDB(token), nil
}

func (s *AuthStore) ConsumePasswordResetToken(ctx context.Context, tokenHash string, consumedAt time.Time) (services.PasswordResetToken, error) {
	token, err := s.queries.ConsumePasswordResetToken(ctx, db.ConsumePasswordResetTokenParams{
		ConsumedAt: sql.NullTime{Time: consumedAt, Valid: true},
		TokenHash:  tokenHash,
		ExpiresAt:  consumedAt,
	})
	if err != nil {
		return services.PasswordResetToken{}, fmt.Errorf("consume password reset token: %w", err)
	}

	return passwordResetTokenFromDB(token), nil
}

func (s *AuthStore) RequestPasswordReset(ctx context.Context, params services.RequestPasswordResetParams) error {
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

func (s *AuthStore) RequestEmailChange(ctx context.Context, params services.RequestEmailChangeParams) error {
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

func (s *AuthStore) ChangeEmailImmediately(ctx context.Context, params services.ChangeEmailImmediatelyParams) (services.User, error) {
	return withTxResult(ctx, s.db, s.queries, "change email", func(queries *db.Queries) (services.User, error) {
		return applyEmailChange(ctx, queries, applyEmailChangeParams{
			UserID:                 params.UserID,
			NewEmail:               params.NewEmail,
			ChangedAt:              params.ChangedAt,
			OldEmailNoticeOptions:  params.OldEmailNoticeOptions,
			NoticeEmailAvailableAt: params.NoticeEmailAvailableAt,
			SendOldEmailNotice:     params.SendOldEmailNotice,
		})
	})
}

func (s *AuthStore) CreateEmailVerificationToken(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (services.EmailVerificationToken, error) {
	token, err := s.queries.CreateEmailVerificationToken(ctx, db.CreateEmailVerificationTokenParams{
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return services.EmailVerificationToken{}, fmt.Errorf("create email verification token: %w", err)
	}

	return emailVerificationTokenFromDB(token), nil
}

func (s *AuthStore) ResendEmailVerification(ctx context.Context, params services.ResendEmailVerificationParams) error {
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

func (s *AuthStore) VerifyEmailByTokenHash(ctx context.Context, tokenHash string, verifiedAt time.Time) (services.User, error) {
	return withTxResult(ctx, s.db, s.queries, "verify email", func(queries *db.Queries) (services.User, error) {
		token, err := queries.ConsumeEmailVerificationToken(ctx, db.ConsumeEmailVerificationTokenParams{
			ConsumedAt: sql.NullTime{Time: verifiedAt, Valid: true},
			TokenHash:  tokenHash,
			ExpiresAt:  verifiedAt,
		})
		if err != nil {
			return services.User{}, fmt.Errorf("consume email verification token: %w", err)
		}

		user, err := queries.MarkUserEmailVerified(ctx, db.MarkUserEmailVerifiedParams{
			EmailVerifiedAt: sql.NullTime{Time: verifiedAt, Valid: true},
			ID:              token.UserID,
		})
		if err != nil {
			return services.User{}, fmt.Errorf("mark user email verified: %w", err)
		}

		return userFromMarkUserEmailVerifiedRow(user), nil
	})
}

func userFromCreateUserRow(row db.CreateUserRow) services.User {
	return services.User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		CreatedAt:       row.CreatedAt,
	}
}

func userRecordFromGetUserByEmailRow(row db.GetUserByEmailRow) services.UserRecord {
	return services.UserRecord{
		User: services.User{
			ID:              row.ID,
			Email:           row.Email,
			EmailVerifiedAt: row.EmailVerifiedAt,
			CreatedAt:       row.CreatedAt,
		},
		PasswordHash: row.PasswordHash,
	}
}

func userRecordFromGetUserBySessionTokenHashRow(row db.GetUserBySessionTokenHashRow) services.UserRecord {
	return services.UserRecord{
		User: services.User{
			ID:              row.ID,
			Email:           row.Email,
			EmailVerifiedAt: row.EmailVerifiedAt,
			CreatedAt:       row.CreatedAt,
		},
		PasswordHash: row.PasswordHash,
	}
}

func userFromMarkUserEmailVerifiedRow(row db.MarkUserEmailVerifiedRow) services.User {
	return services.User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		CreatedAt:       row.CreatedAt,
	}
}

func userFromUpdateUserEmailRow(row db.UpdateUserEmailRow) services.User {
	return services.User{
		ID:              row.ID,
		Email:           row.Email,
		EmailVerifiedAt: row.EmailVerifiedAt,
		CreatedAt:       row.CreatedAt,
	}
}

func userRecordFromGetUserByIDRow(row db.GetUserByIDRow) services.UserRecord {
	return services.UserRecord{
		User: services.User{
			ID:              row.ID,
			Email:           row.Email,
			EmailVerifiedAt: row.EmailVerifiedAt,
			CreatedAt:       row.CreatedAt,
		},
		PasswordHash: row.PasswordHash,
	}
}

func sessionRecordFromSession(row db.Session) services.SessionRecord {
	return services.SessionRecord{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}
}

func emailVerificationTokenFromDB(row db.EmailVerificationToken) services.EmailVerificationToken {
	return services.EmailVerificationToken{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func passwordResetTokenFromDB(row db.PasswordResetToken) services.PasswordResetToken {
	return services.PasswordResetToken{
		ID:         row.ID,
		UserID:     row.UserID,
		TokenHash:  row.TokenHash,
		ExpiresAt:  row.ExpiresAt,
		ConsumedAt: row.ConsumedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func (s *AuthStore) ConfirmEmailChange(ctx context.Context, params services.ConfirmEmailChangeParams) (services.User, error) {
	return withTxResult(ctx, s.db, s.queries, "confirm email change", func(queries *db.Queries) (services.User, error) {
		token, err := queries.ConsumeEmailChangeToken(ctx, db.ConsumeEmailChangeTokenParams{
			ConsumedAt: sql.NullTime{Time: params.ChangedAt, Valid: true},
			TokenHash:  params.TokenHash,
			ExpiresAt:  params.ChangedAt,
		})
		if err != nil {
			return services.User{}, fmt.Errorf("consume email change token: %w", err)
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
	OldEmailNoticeOptions  email.EmailChangeNoticeOptions
	NoticeEmailAvailableAt time.Time
	SendOldEmailNotice     bool
}

func applyEmailChange(ctx context.Context, queries *db.Queries, params applyEmailChangeParams) (services.User, error) {
	oldUser, err := queries.GetUserByID(ctx, params.UserID)
	if err != nil {
		return services.User{}, fmt.Errorf("get user by ID: %w", err)
	}

	user, err := queries.UpdateUserEmail(ctx, db.UpdateUserEmailParams{
		Email:           params.NewEmail,
		EmailVerifiedAt: sql.NullTime{Time: params.ChangedAt, Valid: true},
		ID:              params.UserID,
	})
	if err != nil {
		if isSQLiteUniqueConstraint(err) {
			return services.User{}, services.ErrEmailAlreadyRegistered
		}
		return services.User{}, fmt.Errorf("update user email: %w", err)
	}

	if err := queries.DeleteSessionsByUserID(ctx, params.UserID); err != nil {
		return services.User{}, fmt.Errorf("delete sessions by user ID: %w", err)
	}

	if params.SendOldEmailNotice {
		notice, err := email.NewEmailChangeNoticeMessage(params.OldEmailNoticeOptions, oldUser.Email)
		if err != nil {
			return services.User{}, fmt.Errorf("build old email change notice: %w", err)
		}
		if _, err := queries.EnqueueEmail(ctx, db.EnqueueEmailParams{
			Sender:      notice.From,
			Recipient:   notice.To,
			Subject:     notice.Subject,
			TextBody:    notice.TextBody,
			HtmlBody:    notice.HTMLBody,
			AvailableAt: params.NoticeEmailAvailableAt,
		}); err != nil {
			return services.User{}, fmt.Errorf("enqueue old email change notice: %w", err)
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
