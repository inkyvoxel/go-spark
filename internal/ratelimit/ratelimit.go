// Package ratelimit holds the rate-limit vocabulary shared by the config
// loader (which reads overrides from the environment) and the server (which
// enforces them). Keeping the policy type, the action names, and the defaults
// in one leaf package is what lets those two packages agree without either
// importing the other or hand-copying parallel structs.
package ratelimit

import (
	"strings"
	"time"
)

// Policy is a fixed-window rate-limit rule. A zero value disables the limit.
type Policy struct {
	MaxRequests int
	Window      time.Duration
}

// Name identifies a rate-limited action. Its string value is also the suffix
// of the environment variable that overrides it (see EnvPrefix).
type Name string

const (
	Login                     Name = "login"
	LoginPerIP                Name = "login_per_ip"
	Register                  Name = "register"
	RegisterPerIP             Name = "register_per_ip"
	ForgotPassword            Name = "forgot_password"
	ForgotPasswordPerIP       Name = "forgot_password_per_ip"
	ResetPassword             Name = "reset_password"
	PublicResendVerification  Name = "public_resend_verification"
	PublicResendVerifyPerIP   Name = "public_resend_verification_per_ip"
	AccountResendVerification Name = "account_resend_verification"
	ChangePassword            Name = "change_password"
	ChangeEmail               Name = "change_email"
	RevokeSession             Name = "revoke_session"
	RevokeOtherSessions       Name = "revoke_other_sessions"
	DeleteAccount             Name = "delete_account"
	TOTPChallenge             Name = "totp_challenge"
	TOTPDisable               Name = "totp_disable"
	TOTPConfirm               Name = "totp_confirm"
	TOTPRegenerateCodes       Name = "totp_regenerate_codes"
	PasskeyRegister           Name = "passkey_register"
	PasskeyLogin              Name = "passkey_login"
	PasskeyManage             Name = "passkey_manage"
)

// Names is the canonical iteration order over every action.
var Names = []Name{
	Login, LoginPerIP, Register, RegisterPerIP,
	ForgotPassword, ForgotPasswordPerIP, ResetPassword,
	PublicResendVerification, PublicResendVerifyPerIP, AccountResendVerification,
	ChangePassword, ChangeEmail, RevokeSession, RevokeOtherSessions, DeleteAccount,
	TOTPChallenge, TOTPDisable, TOTPConfirm, TOTPRegenerateCodes,
	PasskeyRegister, PasskeyLogin, PasskeyManage,
}

// EnvPrefix is the environment-variable prefix for a policy, e.g.
// RATE_LIMIT_LOGIN_PER_IP, whose _MAX_REQUESTS and _WINDOW vars override it.
func EnvPrefix(name Name) string {
	return "RATE_LIMIT_" + strings.ToUpper(string(name))
}

// Policies maps each action to its resolved policy.
type Policies map[Name]Policy

// The *PerIP policies are coarse per-IP ceilings layered on top of the
// fine-grained per-email limiters. The per-email limiter keys on ip|email, so
// a single IP gets a fresh allowance per distinct email and can spread one
// password across many accounts (credential stuffing) or mass-mail many
// addresses. The per-IP ceiling caps total attempts from one IP across all
// emails. Thresholds are deliberately generous to tolerate shared NAT egress
// (offices, mobile gateways) where many legitimate users share one IP.
var Defaults = Policies{
	Login:                     {MaxRequests: 5, Window: time.Minute},
	LoginPerIP:                {MaxRequests: 50, Window: 15 * time.Minute},
	Register:                  {MaxRequests: 3, Window: 10 * time.Minute},
	RegisterPerIP:             {MaxRequests: 10, Window: 15 * time.Minute},
	ForgotPassword:            {MaxRequests: 3, Window: 15 * time.Minute},
	ForgotPasswordPerIP:       {MaxRequests: 20, Window: 15 * time.Minute},
	ResetPassword:             {MaxRequests: 5, Window: 15 * time.Minute},
	PublicResendVerification:  {MaxRequests: 3, Window: 15 * time.Minute},
	PublicResendVerifyPerIP:   {MaxRequests: 20, Window: 15 * time.Minute},
	AccountResendVerification: {MaxRequests: 5, Window: 15 * time.Minute},
	ChangePassword:            {MaxRequests: 5, Window: 15 * time.Minute},
	ChangeEmail:               {MaxRequests: 5, Window: 15 * time.Minute},
	RevokeSession:             {MaxRequests: 20, Window: 15 * time.Minute},
	RevokeOtherSessions:       {MaxRequests: 10, Window: 15 * time.Minute},
	DeleteAccount:             {MaxRequests: 3, Window: 15 * time.Minute},
	TOTPChallenge:             {MaxRequests: 5, Window: time.Minute},
	TOTPDisable:               {MaxRequests: 5, Window: 15 * time.Minute},
	TOTPConfirm:               {MaxRequests: 10, Window: 15 * time.Minute},
	TOTPRegenerateCodes:       {MaxRequests: 5, Window: 15 * time.Minute},
	PasskeyRegister:           {MaxRequests: 10, Window: 15 * time.Minute},
	PasskeyLogin:              {MaxRequests: 10, Window: time.Minute},
	PasskeyManage:             {MaxRequests: 20, Window: 15 * time.Minute},
}

// Resolve returns a full policy set: every action present, with each field of
// overrides applied only when set (> 0), otherwise taken from Defaults.
func Resolve(overrides Policies) Policies {
	resolved := make(Policies, len(Names))
	for _, name := range Names {
		policy := Defaults[name]
		if override, ok := overrides[name]; ok {
			if override.MaxRequests > 0 {
				policy.MaxRequests = override.MaxRequests
			}
			if override.Window > 0 {
				policy.Window = override.Window
			}
		}
		resolved[name] = policy
	}
	return resolved
}
