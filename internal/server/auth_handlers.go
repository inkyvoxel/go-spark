package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	emailPattern = `[^@\s]+@[^@\s]+\.[^@\s]+`

	queryKeyStatus = "status"
	queryKeyResend = "resend"

	statusChanged         = "password-changed"
	statusEmailChanged    = "email-changed"
	statusSessionRevoked  = "session-revoked"
	statusSessionsRevoked = "sessions-revoked"
	statusError           = "error"
	statusSent            = "sent"

	loginPathWithStatusChanged = paths.Login + "?" + queryKeyStatus + "=" + statusChanged
	loginPathWithEmailChanged  = paths.Login + "?" + queryKeyStatus + "=" + statusEmailChanged
)

type templateData struct {
	Title                     string
	RequestID                 string
	CSRFToken                 string
	Routes                    paths.TemplateRouteSet
	Breadcrumbs               []breadcrumbItem
	Error                     string
	FieldErrors               map[string]string
	Email                     string
	EmailPattern              string
	ForgotPasswordStatus      string
	LoginStatus               string
	ChangeEmailStatus         string
	Next                      string
	ResetTokenInvalid         bool
	Authenticated             bool
	Verified                  bool
	EmailVerificationRequired bool
	User                      services.User
	PasswordMinLength         int
	ResendStatus              string
	ManagedSessions           []services.ManagedSession
	SessionManagementStatus   string
	SessionManagementError    string
}

type breadcrumbItem struct {
	Label   string
	URL     string
	Current bool
}

func newTemplateData(r *http.Request, title string) templateData {
	data := templateData{
		Title:             title,
		RequestID:         requestID(r.Context()),
		CSRFToken:         csrfToken(r.Context()),
		Routes:            paths.TemplateRoutes,
		EmailPattern:      emailPattern,
		PasswordMinLength: services.DefaultPasswordMinLength,
	}
	if user, ok := currentUser(r.Context()); ok {
		data.Authenticated = true
		data.Verified = user.EmailVerifiedAt.Valid
		data.User = user
	}
	return data
}

func (s *Server) registerForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, templateRegister, s.newTemplateData(r, "Create Account"))
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	if fieldErrors := s.validateRegistrationForm(email, password, confirmPassword); len(fieldErrors) > 0 {
		data := s.newTemplateData(r, "Create Account")
		data.Email = strings.TrimSpace(email)
		data.Error = "Check your details and try again."
		data.FieldErrors = fieldErrors
		s.renderStatus(w, http.StatusUnprocessableEntity, templateRegister, data)
		return
	}

	if _, err := s.auth.Register(r.Context(), email, password); err != nil {
		data := s.newTemplateData(r, "Create Account")
		data.Email = strings.TrimSpace(email)
		s.handleAuthFormError(w, templateRegister, data, err)
		return
	}

	user, session, err := s.auth.Login(r.Context(), email, password)
	if err != nil {
		data := s.newTemplateData(r, "Create Account")
		data.Email = strings.TrimSpace(email)
		s.handleAuthFormError(w, templateRegister, data, err)
		return
	}

	s.setSessionCookie(w, r, session)
	if err := s.rotateCSRFCookieForSession(w, r, session.Token); err != nil {
		s.loggerForRequest(r).Error("rotate csrf token after register login", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	s.loggerForRequest(r).Info("auth register succeeded", "user_id", user.ID)
	redirect := paths.Account
	if !s.emailVerificationPolicy.UserIsVerified(user) {
		redirect = paths.VerifyEmail
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Sign In")
	data.Next = safeRedirectPath(r.URL.Query().Get("next"))
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyStatus))
	if status == statusChanged || status == statusEmailChanged {
		data.LoginStatus = status
	}
	s.render(w, templateLogin, data)
}

func (s *Server) forgotPasswordForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Forgot Password")
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyStatus))
	if status == statusSent || status == statusError {
		data.ForgotPasswordStatus = status
	}
	s.render(w, templateForgotPassword, data)
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	emailAddress := r.FormValue("email")
	if err := s.auth.RequestPasswordReset(r.Context(), emailAddress); err != nil {
		if errors.Is(err, services.ErrInvalidEmail) {
			data := s.newTemplateData(r, "Forgot Password")
			data.Email = strings.TrimSpace(emailAddress)
			data.Error = "Check your details and try again."
			data.FieldErrors = map[string]string{"email": "Enter a valid email address."}
			s.renderStatus(w, http.StatusUnprocessableEntity, templateForgotPassword, data)
			return
		}

		s.loggerForRequest(r).Error("request password reset", "err", err)
		http.Redirect(w, r, withQueryParam(paths.ForgotPassword, queryKeyStatus, statusError), http.StatusSeeOther)
		return
	}

	s.loggerForRequest(r).Info("auth password reset requested")
	http.Redirect(w, r, withQueryParam(paths.ForgotPassword, queryKeyStatus, statusSent), http.StatusSeeOther)
}

func (s *Server) resetPasswordForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Reset Password")
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token != "" {
		if err := s.auth.ValidatePasswordResetToken(r.Context(), token); err != nil {
			if errors.Is(err, services.ErrInvalidPasswordResetToken) {
				s.clearResetCookie(w, r)
				data.Error = "This password reset link is invalid or has expired."
				data.ResetTokenInvalid = true
				s.renderStatus(w, http.StatusBadRequest, templateResetPassword, data)
				return
			}

			s.loggerForRequest(r).Error("validate password reset token", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		s.setResetCookie(w, r, token)
		http.Redirect(w, r, paths.ResetPassword, http.StatusSeeOther)
		return
	}

	token = resetTokenFromCookie(r)
	if err := s.auth.ValidatePasswordResetToken(r.Context(), token); err != nil {
		if errors.Is(err, services.ErrInvalidPasswordResetToken) {
			s.clearResetCookie(w, r)
			data.Error = "This password reset link is invalid or has expired."
			data.ResetTokenInvalid = true
			s.renderStatus(w, http.StatusBadRequest, templateResetPassword, data)
			return
		}

		s.loggerForRequest(r).Error("validate password reset token", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	s.render(w, templateResetPassword, data)
}

func (s *Server) resendVerificationForm(w http.ResponseWriter, r *http.Request) {
	if !s.emailVerificationPolicy.Required() {
		if _, ok := currentUser(r.Context()); ok {
			http.Redirect(w, r, paths.Account, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, paths.Login, http.StatusSeeOther)
		return
	}

	data := s.newTemplateData(r, "Resend Verification Email")
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyStatus))
	if status == statusSent || status == statusError {
		data.ResendStatus = status
	}
	s.render(w, templateResendVerification, data)
}

func (s *Server) confirmEmail(w http.ResponseWriter, r *http.Request) {
	if !s.emailVerificationPolicy.Required() {
		if _, ok := currentUser(r.Context()); ok {
			http.Redirect(w, r, paths.Account, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, paths.Login, http.StatusSeeOther)
		return
	}

	data := s.newTemplateData(r, "Confirm Email")

	if !s.confirmEmailToken(
		w,
		r,
		&data,
		templateConfirmEmail,
		"confirm email",
		s.auth.VerifyEmail,
		func(err error, data *templateData) (int, bool) {
			if errors.Is(err, services.ErrInvalidVerificationToken) {
				data.Error = "This confirmation link is invalid or has expired."
				return http.StatusBadRequest, true
			}
			return 0, false
		},
	) {
		return
	}

	s.loggerForRequest(r).Info("auth email verified")
	s.render(w, templateConfirmEmail, data)
}

func (s *Server) confirmEmailChange(w http.ResponseWriter, r *http.Request) {
	if !s.emailVerificationPolicy.RequiresEmailChangeVerification() {
		if _, ok := currentUser(r.Context()); ok {
			http.Redirect(w, r, paths.Account, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, paths.Login, http.StatusSeeOther)
		return
	}

	data := s.newTemplateData(r, "Confirm Email Change")

	if !s.confirmEmailToken(
		w,
		r,
		&data,
		templateConfirmEmailChange,
		"confirm email change",
		s.auth.ConfirmEmailChange,
		func(err error, data *templateData) (int, bool) {
			switch {
			case errors.Is(err, services.ErrInvalidEmailChangeToken):
				data.Error = "This email change link is invalid or has expired."
				return http.StatusBadRequest, true
			case errors.Is(err, services.ErrEmailAlreadyRegistered):
				data.Error = "This email address is already used by another account."
				return http.StatusConflict, true
			default:
				return 0, false
			}
		},
	) {
		return
	}

	s.clearSessionCookie(w, r)
	s.clearCSRFCookie(w, r)
	s.loggerForRequest(r).Info("auth email change confirmed")
	data.Authenticated = false
	data.Verified = false
	data.User = services.User{}
	s.render(w, templateConfirmEmailChange, data)
}

type confirmEmailErrorHandler func(err error, data *templateData) (status int, handled bool)

func (s *Server) confirmEmailToken(
	w http.ResponseWriter,
	r *http.Request,
	data *templateData,
	templateName string,
	logMessage string,
	confirm func(context.Context, string) (services.User, error),
	handleErr confirmEmailErrorHandler,
) bool {
	if _, err := confirm(r.Context(), r.URL.Query().Get("token")); err != nil {
		if status, handled := handleErr(err, data); handled {
			s.renderStatus(w, status, templateName, *data)
			return false
		}

		s.loggerForRequest(r).Error(logMessage, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return false
	}

	return true
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	user, session, err := s.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		if errors.Is(err, services.ErrInvalidCredentials) {
			s.loggerForRequest(r).Info("auth login failed")
		}
		data := s.newTemplateData(r, "Sign In")
		data.Email = strings.TrimSpace(r.FormValue("email"))
		data.Next = safeRedirectPath(r.FormValue("next"))
		s.handleAuthFormError(w, templateLogin, data, err)
		return
	}

	next := safeRedirectPath(r.FormValue("next"))
	if !s.emailVerificationPolicy.UserIsVerified(user) {
		next = paths.VerifyEmail
	} else if next == "" {
		next = paths.Account
	}

	s.setSessionCookie(w, r, session)
	if err := s.rotateCSRFCookieForSession(w, r, session.Token); err != nil {
		s.loggerForRequest(r).Error("rotate csrf token after login", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	s.loggerForRequest(r).Info("auth login succeeded", "user_id", user.ID)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil && !errors.Is(err, services.ErrInvalidSession) {
			s.loggerForRequest(r).Error("logout", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.clearSessionCookie(w, r)
	s.clearCSRFCookie(w, r)
	s.loggerForRequest(r).Info("auth logout succeeded")
	http.Redirect(w, r, paths.Home, http.StatusSeeOther)
}

func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Account")
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyStatus))
	if status == statusSessionRevoked || status == statusSessionsRevoked {
		data.SessionManagementStatus = status
	}
	if !s.populateAccountSessions(w, r, &data) {
		return
	}
	s.render(w, templateAccount, data)
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	sessionID, err := s.sessionIDFromRequest(r.FormValue("session_id"))
	if err != nil {
		data := s.newTemplateData(r, "Account")
		data.SessionManagementError = "Select a valid session to revoke."
		if !s.populateAccountSessions(w, r, &data) {
			return
		}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateAccount, data)
		return
	}

	err = s.auth.RevokeSessionByID(r.Context(), user.ID, sessionTokenFromCookie(r), sessionID)
	if err != nil {
		data := s.newTemplateData(r, "Account")
		switch {
		case errors.Is(err, services.ErrInvalidSession):
			s.clearSessionCookie(w, r)
			s.clearCSRFCookie(w, r)
			http.Redirect(w, r, paths.Login, http.StatusSeeOther)
			return
		case errors.Is(err, services.ErrCannotRevokeCurrentSession):
			data.SessionManagementError = "You cannot revoke your current session. Use Sign out for this device."
		case errors.Is(err, services.ErrInvalidSessionTarget):
			data.SessionManagementError = "The selected session is no longer available."
		default:
			s.loggerForRequest(r).Error("revoke session", "user_id", user.ID, "session_id", sessionID, "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		if !s.populateAccountSessions(w, r, &data) {
			return
		}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateAccount, data)
		return
	}

	s.loggerForRequest(r).Info("auth session revoked", "user_id", user.ID, "session_id", sessionID)
	http.Redirect(w, r, withQueryParam(paths.Account, queryKeyStatus, statusSessionRevoked), http.StatusSeeOther)
}

func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := s.auth.RevokeOtherSessions(r.Context(), user.ID, sessionTokenFromCookie(r)); err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidSession):
			s.clearSessionCookie(w, r)
			s.clearCSRFCookie(w, r)
			http.Redirect(w, r, paths.Login, http.StatusSeeOther)
			return
		default:
			s.loggerForRequest(r).Error("revoke other sessions", "user_id", user.ID, "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.loggerForRequest(r).Info("auth other sessions revoked", "user_id", user.ID)
	http.Redirect(w, r, withQueryParam(paths.Account, queryKeyStatus, statusSessionsRevoked), http.StatusSeeOther)
}

func (s *Server) changePasswordForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, templateChangePassword, s.newChangePasswordTemplateData(r))
}

func (s *Server) changeEmailForm(w http.ResponseWriter, r *http.Request) {
	data := s.newChangeEmailTemplateData(r)
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyStatus))
	if status == statusSent {
		data.ChangeEmailStatus = status
	}
	s.render(w, templateChangeEmail, data)
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	if !s.emailVerificationPolicy.Required() {
		http.Redirect(w, r, paths.Account, http.StatusSeeOther)
		return
	}

	data := s.newTemplateData(r, "Verify Email")
	if data.Verified {
		http.Redirect(w, r, paths.Account, http.StatusSeeOther)
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyResend))
	if status == statusSent || status == statusError {
		data.ResendStatus = status
	}
	s.render(w, templateVerifyEmail, data)
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if !s.emailVerificationPolicy.Required() {
		http.Redirect(w, r, paths.Account, http.StatusSeeOther)
		return
	}

	if err := s.auth.ResendVerificationEmail(r.Context(), user.ID); err != nil {
		s.loggerForRequest(r).Error("resend verification email", "user_id", user.ID, "err", err)
		http.Redirect(w, r, withQueryParam(paths.VerifyEmail, queryKeyResend, statusError), http.StatusSeeOther)
		return
	}

	s.loggerForRequest(r).Info("auth verification email resent", "user_id", user.ID)
	http.Redirect(w, r, withQueryParam(paths.VerifyEmail, queryKeyResend, statusSent), http.StatusSeeOther)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	fieldErrors := make(map[string]string)
	if currentPassword == "" {
		fieldErrors["current_password"] = "Enter your current password."
	}
	for key, value := range s.validatePasswordPair(newPassword, confirmPassword, "new_password", "confirm_password") {
		fieldErrors[key] = value
	}
	if len(fieldErrors) > 0 {
		data := s.newChangePasswordTemplateData(r)
		data.Error = "Check your details and try again."
		data.FieldErrors = fieldErrors
		s.renderStatus(w, http.StatusUnprocessableEntity, templateChangePassword, data)
		return
	}

	if err := s.auth.ChangePassword(r.Context(), user.ID, currentPassword, newPassword); err != nil {
		data := s.newChangePasswordTemplateData(r)
		data.Error = "Check your details and try again."
		switch {
		case errors.Is(err, services.ErrCurrentPasswordIncorrect):
			data.FieldErrors = map[string]string{"current_password": "Current password is not correct."}
			s.renderStatus(w, http.StatusUnprocessableEntity, templateChangePassword, data)
			return
		case errors.Is(err, services.ErrInvalidPassword):
			data.FieldErrors = map[string]string{"new_password": fmt.Sprintf("Use at least %d characters.", data.PasswordMinLength)}
			s.renderStatus(w, http.StatusUnprocessableEntity, templateChangePassword, data)
			return
		case errors.Is(err, services.ErrPasswordUnchanged):
			data.FieldErrors = map[string]string{"new_password": "Choose a different password."}
			s.renderStatus(w, http.StatusUnprocessableEntity, templateChangePassword, data)
			return
		default:
			s.loggerForRequest(r).Error("change password", "user_id", user.ID, "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.clearSessionCookie(w, r)
	s.clearCSRFCookie(w, r)
	s.loggerForRequest(r).Info("auth password changed", "user_id", user.ID)
	http.Redirect(w, r, loginPathWithStatusChanged, http.StatusSeeOther)
}

func (s *Server) changeEmail(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	newEmail := r.FormValue("email")
	currentPassword := r.FormValue("current_password")

	fieldErrors := make(map[string]string)
	if strings.TrimSpace(newEmail) == "" {
		fieldErrors["email"] = "Enter your new email address."
	}
	if currentPassword == "" {
		fieldErrors["current_password"] = "Enter your current password."
	}
	if len(fieldErrors) > 0 {
		data := s.newChangeEmailTemplateData(r)
		data.Email = strings.TrimSpace(newEmail)
		data.Error = "Check your details and try again."
		data.FieldErrors = fieldErrors
		s.renderStatus(w, http.StatusUnprocessableEntity, templateChangeEmail, data)
		return
	}

	if err := s.auth.RequestEmailChange(r.Context(), user.ID, currentPassword, newEmail); err != nil {
		data := s.newChangeEmailTemplateData(r)
		data.Email = strings.TrimSpace(newEmail)
		data.Error = "Check your details and try again."
		switch {
		case errors.Is(err, services.ErrCurrentPasswordIncorrect):
			data.FieldErrors = map[string]string{"current_password": "Current password is not correct."}
		case errors.Is(err, services.ErrInvalidEmail):
			data.FieldErrors = map[string]string{"email": "Enter a valid email address."}
		case errors.Is(err, services.ErrEmailUnchanged):
			data.FieldErrors = map[string]string{"email": "Choose a different email address."}
		case errors.Is(err, services.ErrEmailAlreadyRegistered):
			data.FieldErrors = map[string]string{"email": "An account with this email already exists."}
		default:
			s.loggerForRequest(r).Error("change email", "user_id", user.ID, "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateChangeEmail, data)
		return
	}

	if !s.emailVerificationPolicy.RequiresEmailChangeVerification() {
		s.clearSessionCookie(w, r)
		s.clearCSRFCookie(w, r)
		s.loggerForRequest(r).Info("auth email changed", "user_id", user.ID)
		http.Redirect(w, r, loginPathWithEmailChanged, http.StatusSeeOther)
		return
	}

	s.loggerForRequest(r).Info("auth email change requested", "user_id", user.ID)
	http.Redirect(w, r, withQueryParam(paths.ChangeEmail, queryKeyStatus, statusSent), http.StatusSeeOther)
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	token := resetTokenFromCookie(r)
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	fieldErrors := s.validatePasswordPair(newPassword, confirmPassword, "new_password", "confirm_password")
	if len(fieldErrors) > 0 {
		data := s.newTemplateData(r, "Reset Password")
		data.Error = "Check your details and try again."
		data.FieldErrors = fieldErrors
		s.renderStatus(w, http.StatusUnprocessableEntity, templateResetPassword, data)
		return
	}

	if err := s.auth.ResetPasswordWithToken(r.Context(), token, newPassword); err != nil {
		data := s.newTemplateData(r, "Reset Password")
		data.Error = "Check your details and try again."
		switch {
		case errors.Is(err, services.ErrInvalidPasswordResetToken):
			s.clearResetCookie(w, r)
			data.Error = "This password reset link is invalid or has expired."
			data.ResetTokenInvalid = true
			s.renderStatus(w, http.StatusBadRequest, templateResetPassword, data)
			return
		case errors.Is(err, services.ErrInvalidPassword):
			data.FieldErrors = map[string]string{"new_password": fmt.Sprintf("Use at least %d characters.", data.PasswordMinLength)}
			s.renderStatus(w, http.StatusUnprocessableEntity, templateResetPassword, data)
			return
		default:
			s.loggerForRequest(r).Error("reset password", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.clearResetCookie(w, r)
	s.loggerForRequest(r).Info("auth password reset completed")
	http.Redirect(w, r, loginPathWithStatusChanged, http.StatusSeeOther)
}

func (s *Server) resendVerificationPublic(w http.ResponseWriter, r *http.Request) {
	if !s.emailVerificationPolicy.Required() {
		if _, ok := currentUser(r.Context()); ok {
			http.Redirect(w, r, paths.Account, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, paths.Login, http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	emailAddress := r.FormValue("email")
	if err := s.auth.ResendVerificationEmailByAddress(r.Context(), emailAddress); err != nil {
		if errors.Is(err, services.ErrInvalidEmail) {
			data := s.newTemplateData(r, "Resend Verification Email")
			data.Email = strings.TrimSpace(emailAddress)
			data.Error = "Check your details and try again."
			data.FieldErrors = map[string]string{"email": "Enter a valid email address."}
			s.renderStatus(w, http.StatusUnprocessableEntity, templateResendVerification, data)
			return
		}

		s.loggerForRequest(r).Error("resend verification email (public)", "err", err)
		http.Redirect(w, r, withQueryParam(paths.ResendVerification, queryKeyStatus, statusError), http.StatusSeeOther)
		return
	}

	s.loggerForRequest(r).Info("auth verification email requested")
	http.Redirect(w, r, withQueryParam(paths.ResendVerification, queryKeyStatus, statusSent), http.StatusSeeOther)
}

func (s *Server) deleteAccountForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, templateDeleteAccount, s.newDeleteAccountTemplateData(r))
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("current_password")
	if currentPassword == "" {
		data := s.newDeleteAccountTemplateData(r)
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"current_password": "Enter your current password."}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateDeleteAccount, data)
		return
	}

	if err := s.auth.DeleteAccount(r.Context(), user.ID, currentPassword); err != nil {
		data := s.newDeleteAccountTemplateData(r)
		switch {
		case errors.Is(err, services.ErrCurrentPasswordIncorrect):
			data.Error = "Check your details and try again."
			data.FieldErrors = map[string]string{"current_password": "Current password is not correct."}
			s.renderStatus(w, http.StatusUnprocessableEntity, templateDeleteAccount, data)
			return
		default:
			s.loggerForRequest(r).Error("delete account", "user_id", user.ID, "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.clearSessionCookie(w, r)
	s.clearCSRFCookie(w, r)
	s.loggerForRequest(r).Info("auth account deleted", "user_id", user.ID)
	http.Redirect(w, r, paths.Home, http.StatusSeeOther)
}

func (s *Server) handleAuthFormError(w http.ResponseWriter, templateName string, data templateData, err error) {
	if errors.Is(err, services.ErrInvalidEmail) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"email": "Enter a valid email address."}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateName, data)
		return
	}
	if errors.Is(err, services.ErrInvalidPassword) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"password": fmt.Sprintf("Use at least %d characters.", data.PasswordMinLength)}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateName, data)
		return
	}
	if errors.Is(err, services.ErrEmailAlreadyRegistered) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"email": "An account with this email already exists."}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateName, data)
		return
	}
	if errors.Is(err, services.ErrInvalidCredentials) {
		data.Error = "Email or password is not correct."
		s.renderStatus(w, http.StatusUnprocessableEntity, templateName, data)
		return
	}

	s.loggerForRequestID(data.RequestID).Error("auth form", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Server) validateRegistrationForm(email, password, confirmPassword string) map[string]string {
	fieldErrors := make(map[string]string)
	if strings.TrimSpace(email) == "" {
		fieldErrors["email"] = "Enter your email address."
	}
	for key, value := range s.validatePasswordPair(password, confirmPassword, "password", "confirm_password") {
		fieldErrors[key] = value
	}

	return fieldErrors
}

func (s *Server) populateAccountSessions(w http.ResponseWriter, r *http.Request, data *templateData) bool {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return false
	}

	sessions, err := s.auth.ListManagedSessions(r.Context(), user.ID, sessionTokenFromCookie(r))
	if err != nil {
		if errors.Is(err, services.ErrInvalidSession) {
			s.clearSessionCookie(w, r)
			s.clearCSRFCookie(w, r)
			http.Redirect(w, r, paths.Login, http.StatusSeeOther)
			return false
		}

		s.loggerForRequest(r).Error("list account sessions", "user_id", user.ID, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return false
	}

	data.ManagedSessions = sessions
	return true
}

func (s *Server) sessionIDFromRequest(raw string) (int64, error) {
	sessionID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || sessionID <= 0 {
		return 0, errors.New("invalid session id")
	}
	return sessionID, nil
}

func (s *Server) validatePasswordPair(password, confirmPassword, passwordField, confirmPasswordField string) map[string]string {
	passwordMinLength := s.passwordMinLength
	if passwordMinLength == 0 {
		passwordMinLength = services.DefaultPasswordMinLength
	}

	fieldErrors := make(map[string]string)
	if password == "" {
		fieldErrors[passwordField] = "Enter a password."
	} else if utf8.RuneCountInString(password) < passwordMinLength {
		fieldErrors[passwordField] = fmt.Sprintf("Use at least %d characters.", passwordMinLength)
	}
	if confirmPassword == "" {
		fieldErrors[confirmPasswordField] = "Confirm your password."
	} else if password != confirmPassword {
		fieldErrors[confirmPasswordField] = "Passwords do not match."
	}

	return fieldErrors
}

func (s *Server) newTemplateData(r *http.Request, title string) templateData {
	data := newTemplateData(r, title)
	if s.passwordMinLength > 0 {
		data.PasswordMinLength = s.passwordMinLength
	}
	data.EmailVerificationRequired = s.emailVerificationPolicy.Required()
	if data.Authenticated {
		data.Verified = s.emailVerificationPolicy.UserIsVerified(data.User)
	}
	return data
}

func (s *Server) newChangePasswordTemplateData(r *http.Request) templateData {
	data := s.newTemplateData(r, "Change Password")
	data.Breadcrumbs = []breadcrumbItem{
		{Label: "Account", URL: paths.Account},
		{Label: "Change password", Current: true},
	}
	return data
}

func (s *Server) newChangeEmailTemplateData(r *http.Request) templateData {
	data := s.newTemplateData(r, "Change Email")
	data.Breadcrumbs = []breadcrumbItem{
		{Label: "Account", URL: paths.Account},
		{Label: "Change email", Current: true},
	}
	return data
}

func (s *Server) newDeleteAccountTemplateData(r *http.Request) templateData {
	data := s.newTemplateData(r, "Delete Account")
	data.Breadcrumbs = []breadcrumbItem{
		{Label: "Account", URL: paths.Account},
		{Label: "Delete account", Current: true},
	}
	return data
}

func (s *Server) render(w http.ResponseWriter, templateName string, data templateData) {
	s.renderStatus(w, http.StatusOK, templateName, data)
}

func (s *Server) renderStatus(w http.ResponseWriter, status int, templateName string, data templateData) {
	var body bytes.Buffer
	if err := s.renderTemplate(&body, templateName, data); err != nil {
		s.loggerForRequestID(data.RequestID).Error("render template", "template", templateName, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

func (s *Server) renderTemplate(w io.Writer, templateName string, data templateData) error {
	tmpl, ok := s.templates[templateName]
	if !ok {
		return fmt.Errorf("template %q not found", templateName)
	}

	return tmpl.ExecuteTemplate(w, templateLayout, data)
}

func withQueryParam(basePath, key, value string) string {
	return basePath + "?" + key + "=" + value
}
