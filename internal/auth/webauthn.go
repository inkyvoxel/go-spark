package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// DefaultPasskeyName is used when a user registers a passkey without a label.
const DefaultPasskeyName = "Passkey"

// passkeyCeremonyTimeout bounds how long a begin→finish ceremony may take.
const passkeyCeremonyTimeout = 5 * time.Minute

// WebAuthnCredential is the persisted form of a registered passkey.
type WebAuthnCredential struct {
	ID              int64
	UserID          int64
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	Transports      []string
	BackupEligible  bool
	BackupState     bool
	Name            string
	CreatedAt       time.Time
	LastUsedAt      sql.NullTime
}

// CreateWebAuthnCredentialParams carries a newly registered credential to the store.
type CreateWebAuthnCredentialParams struct {
	UserID          int64
	CredentialID    []byte
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	Transports      []string
	BackupEligible  bool
	BackupState     bool
	Name            string
}

// UpdateWebAuthnCredentialParams carries the mutable fields written back after each
// successful assertion (sign counter, backup state, last-used timestamp).
type UpdateWebAuthnCredentialParams struct {
	CredentialID []byte
	SignCount    uint32
	BackupState  bool
	LastUsedAt   time.Time
}

// newWebAuthn builds the relying-party instance. It returns (nil, nil) when no RP ID
// or origins are configured, which leaves passkeys disabled rather than erroring.
func newWebAuthn(opts AuthOptions) (*webauthn.WebAuthn, error) {
	rpID := strings.TrimSpace(opts.WebAuthnRPID)
	origins := make([]string, 0, len(opts.WebAuthnRPOrigins))
	for _, origin := range opts.WebAuthnRPOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins = append(origins, origin)
		}
	}
	if rpID == "" || len(origins) == 0 {
		return nil, nil
	}

	displayName := strings.TrimSpace(opts.WebAuthnRPDisplayName)
	if displayName == "" {
		displayName = rpID
	}

	requireResidentKey := true
	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: displayName,
		RPOrigins:     origins,
		// Enforce a ceremony timeout server-side so the stored session data
		// (and its signed cookie) has a bounded, populated expiry.
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyCeremonyTimeout, TimeoutUVD: passkeyCeremonyTimeout},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyCeremonyTimeout, TimeoutUVD: passkeyCeremonyTimeout},
		},
		// Consumer passkeys: don't request attestation.
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			// Discoverable credentials + required user verification make a
			// passkey assertion phishing-resistant MFA on its own.
			RequireResidentKey: &requireResidentKey,
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		},
	})
}

// PasskeysEnabled reports whether passkey support is configured.
func (s *AuthService) PasskeysEnabled() bool {
	return s.webAuthn != nil
}

// webAuthnUser adapts an application user to the webauthn.User interface.
type webAuthnUser struct {
	id          int64
	email       string
	handle      []byte
	credentials []webauthn.Credential
}

func (u *webAuthnUser) WebAuthnID() []byte                         { return u.handle }
func (u *webAuthnUser) WebAuthnName() string                       { return u.email }
func (u *webAuthnUser) WebAuthnDisplayName() string                { return u.email }
func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *AuthService) webAuthnUserByID(ctx context.Context, userID int64) (*webAuthnUser, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	handle, err := s.store.GetWebAuthnHandleByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get webauthn handle: %w", err)
	}
	credentials, err := s.loadCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &webAuthnUser{
		id:          userID,
		email:       user.Email,
		handle:      handle,
		credentials: credentials,
	}, nil
}

func (s *AuthService) loadCredentials(ctx context.Context, userID int64) ([]webauthn.Credential, error) {
	stored, err := s.store.ListWebAuthnCredentialsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list webauthn credentials: %w", err)
	}
	credentials := make([]webauthn.Credential, 0, len(stored))
	for _, credential := range stored {
		credentials = append(credentials, toLibraryCredential(credential))
	}
	return credentials, nil
}

// BeginPasskeyRegistration starts the credential-creation ceremony for a logged-in
// user, excluding the authenticators they have already registered.
func (s *AuthService) BeginPasskeyRegistration(ctx context.Context, userID int64) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	if s.webAuthn == nil {
		return nil, nil, ErrPasskeysDisabled
	}
	user, err := s.webAuthnUserByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	exclusions := make([]protocol.CredentialDescriptor, 0, len(user.credentials))
	for _, credential := range user.credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}

	creation, session, err := s.webAuthn.BeginRegistration(user, webauthn.WithExclusions(exclusions))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrPasskeyCeremony, err)
	}
	return creation, session, nil
}

// FinishPasskeyRegistration validates the authenticator response against the stored
// session data and persists the new credential under the given label.
func (s *AuthService) FinishPasskeyRegistration(ctx context.Context, userID int64, session webauthn.SessionData, response *protocol.ParsedCredentialCreationData, name string) error {
	if s.webAuthn == nil {
		return ErrPasskeysDisabled
	}
	user, err := s.webAuthnUserByID(ctx, userID)
	if err != nil {
		return err
	}

	credential, err := s.webAuthn.CreateCredential(user, session, response)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPasskeyCeremony, err)
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultPasskeyName
	}
	if err := s.store.CreateWebAuthnCredential(ctx, credentialToCreateParams(userID, credential, name)); err != nil {
		return fmt.Errorf("create webauthn credential: %w", err)
	}
	return nil
}

// BeginPasskeyLogin starts a usernameless (discoverable) assertion ceremony.
func (s *AuthService) BeginPasskeyLogin(ctx context.Context) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	if s.webAuthn == nil {
		return nil, nil, ErrPasskeysDisabled
	}
	assertion, session, err := s.webAuthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrPasskeyCeremony, err)
	}
	return assertion, session, nil
}

// FinishPasskeyLogin validates a discoverable assertion, resolves the owning user,
// updates the credential's mutable fields, and issues a session. A user-verified
// passkey assertion is treated as full authentication (no password or TOTP).
func (s *AuthService) FinishPasskeyLogin(ctx context.Context, session webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (User, AuthSession, error) {
	if s.webAuthn == nil {
		return User{}, AuthSession{}, ErrPasskeysDisabled
	}

	handler := func(_, userHandle []byte) (webauthn.User, error) {
		user, err := s.store.GetUserByWebAuthnHandle(ctx, userHandle)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPasskeyNotFound
		}
		if err != nil {
			return nil, err
		}
		credentials, err := s.loadCredentials(ctx, user.ID)
		if err != nil {
			return nil, err
		}
		return &webAuthnUser{
			id:          user.ID,
			email:       user.Email,
			handle:      userHandle,
			credentials: credentials,
		}, nil
	}

	waUser, credential, err := s.webAuthn.ValidatePasskeyLogin(handler, session, response)
	if err != nil {
		return User{}, AuthSession{}, fmt.Errorf("%w: %v", ErrPasskeyCeremony, err)
	}
	resolved, ok := waUser.(*webAuthnUser)
	if !ok || resolved == nil {
		return User{}, AuthSession{}, ErrPasskeyCeremony
	}

	// Persist the rolling sign counter and backup state. The library has already
	// flagged a non-increasing counter (possible clone) on the credential; we
	// still update so legitimate authenticators that report 0 keep working.
	if err := s.store.UpdateWebAuthnCredentialOnLogin(ctx, UpdateWebAuthnCredentialParams{
		CredentialID: credential.ID,
		SignCount:    credential.Authenticator.SignCount,
		BackupState:  credential.Flags.BackupState,
		LastUsedAt:   time.Now().UTC(),
	}); err != nil {
		return User{}, AuthSession{}, fmt.Errorf("update webauthn credential: %w", err)
	}

	user, err := s.store.GetUserByID(ctx, resolved.id)
	if err != nil {
		return User{}, AuthSession{}, fmt.Errorf("get user: %w", err)
	}

	authSession, err := s.createSessionForUser(ctx, resolved.id)
	if err != nil {
		return User{}, AuthSession{}, err
	}
	return user.User, authSession, nil
}

// ListPasskeys returns the user's registered credentials for display.
func (s *AuthService) ListPasskeys(ctx context.Context, userID int64) ([]WebAuthnCredential, error) {
	credentials, err := s.store.ListWebAuthnCredentialsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list webauthn credentials: %w", err)
	}
	return credentials, nil
}

// CountPasskeys reports how many passkeys the user has registered.
func (s *AuthService) CountPasskeys(ctx context.Context, userID int64) (int, error) {
	count, err := s.store.CountWebAuthnCredentialsByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count webauthn credentials: %w", err)
	}
	return int(count), nil
}

// RenamePasskey updates a credential's label. Deleting the last passkey is safe in
// this template because a password is always present as a fallback.
func (s *AuthService) RenamePasskey(ctx context.Context, userID, credentialDBID int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultPasskeyName
	}
	updated, err := s.store.RenameWebAuthnCredential(ctx, userID, credentialDBID, name)
	if err != nil {
		return fmt.Errorf("rename webauthn credential: %w", err)
	}
	if !updated {
		return ErrPasskeyNotFound
	}
	return nil
}

// DeletePasskey removes a credential owned by the user.
func (s *AuthService) DeletePasskey(ctx context.Context, userID, credentialDBID int64) error {
	deleted, err := s.store.DeleteWebAuthnCredential(ctx, userID, credentialDBID)
	if err != nil {
		return fmt.Errorf("delete webauthn credential: %w", err)
	}
	if !deleted {
		return ErrPasskeyNotFound
	}
	return nil
}

func toLibraryCredential(c WebAuthnCredential) webauthn.Credential {
	transports := make([]protocol.AuthenticatorTransport, len(c.Transports))
	for i, transport := range c.Transports {
		transports[i] = protocol.AuthenticatorTransport(transport)
	}
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.AAGUID,
			SignCount: c.SignCount,
		},
	}
}

func credentialToCreateParams(userID int64, credential *webauthn.Credential, name string) CreateWebAuthnCredentialParams {
	transports := make([]string, len(credential.Transport))
	for i, transport := range credential.Transport {
		transports[i] = string(transport)
	}
	return CreateWebAuthnCredentialParams{
		UserID:          userID,
		CredentialID:    credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       credential.Authenticator.SignCount,
		Transports:      transports,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		Name:            name,
	}
}
