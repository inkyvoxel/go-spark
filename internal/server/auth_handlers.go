package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

const (
	emailPattern = `[^@\s]+@[^@\s]+\.[^@\s]+`

	queryKeyStatus = "status"
	queryKeyResend = "resend"

	statusChanged = "password-changed"
	statusError   = "error"
	statusSent    = "sent"

	loginPathWithStatusChanged = paths.Login + "?" + queryKeyStatus + "=" + statusChanged
)

type templateData struct {
	Title                string
	CSRFToken            string
	Routes               paths.TemplateRouteSet
	Breadcrumbs          []breadcrumbItem
	Error                string
	FieldErrors          map[string]string
	Email                string
	EmailPattern         string
	ForgotPasswordStatus string
	LoginStatus          string
	Next                 string
	ResetToken           string
	ResetTokenInvalid    bool
	Authenticated        bool
	Verified             bool
	User                 db.User
	PasswordMinLength    int
	ResendStatus         string
}

type breadcrumbItem struct {
	Label   string
	URL     string
	Current bool
}

func newTemplateData(r *http.Request, title string) templateData {
	data := templateData{
		Title:             title,
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
		s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateRegister, fragmentRegisterForm, data)
		return
	}

	if _, err := s.auth.Register(r.Context(), email, password); err != nil {
		data := s.newTemplateData(r, "Create Account")
		data.Email = strings.TrimSpace(email)
		s.handleAuthFormError(w, r, templateRegister, fragmentRegisterForm, data, err)
		return
	}

	user, session, err := s.auth.Login(r.Context(), email, password)
	if err != nil {
		data := s.newTemplateData(r, "Create Account")
		data.Email = strings.TrimSpace(email)
		s.handleAuthFormError(w, r, templateRegister, fragmentRegisterForm, data, err)
		return
	}

	s.setSessionCookie(w, r, session)
	redirect := paths.Account
	if !user.EmailVerifiedAt.Valid {
		redirect = paths.VerifyEmail
	}
	s.redirectWithHTMX(w, r, redirect)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Sign In")
	data.Next = safeRedirectPath(r.URL.Query().Get("next"))
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyStatus))
	if status == statusChanged {
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
			s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateForgotPassword, fragmentForgotPasswordForm, data)
			return
		}

		s.logger.Error("request password reset", "err", err)
		if isHXRequest(r) {
			data := s.newTemplateData(r, "Forgot Password")
			data.ForgotPasswordStatus = statusError
			s.renderFragmentStatus(w, http.StatusOK, templateForgotPassword, fragmentForgotPasswordForm, data)
			return
		}
		http.Redirect(w, r, withQueryParam(paths.ForgotPassword, queryKeyStatus, statusError), http.StatusSeeOther)
		return
	}

	if isHXRequest(r) {
		data := s.newTemplateData(r, "Forgot Password")
		data.ForgotPasswordStatus = statusSent
		s.renderFragmentStatus(w, http.StatusOK, templateForgotPassword, fragmentForgotPasswordForm, data)
		return
	}
	http.Redirect(w, r, withQueryParam(paths.ForgotPassword, queryKeyStatus, statusSent), http.StatusSeeOther)
}

func (s *Server) resetPasswordForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Reset Password")
	data.ResetToken = strings.TrimSpace(r.URL.Query().Get("token"))

	if err := s.auth.ValidatePasswordResetToken(r.Context(), data.ResetToken); err != nil {
		if errors.Is(err, services.ErrInvalidPasswordResetToken) {
			data.Error = "This password reset link is invalid or has expired."
			data.ResetTokenInvalid = true
			s.renderStatus(w, http.StatusBadRequest, templateResetPassword, data)
			return
		}

		s.logger.Error("validate password reset token", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	s.render(w, templateResetPassword, data)
}

func (s *Server) resendVerificationForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Resend Verification Email")
	status := strings.TrimSpace(r.URL.Query().Get(queryKeyStatus))
	if status == statusSent || status == statusError {
		data.ResendStatus = status
	}
	s.render(w, templateResendVerification, data)
}

func (s *Server) confirmEmail(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Confirm Email")

	if _, err := s.auth.VerifyEmail(r.Context(), r.URL.Query().Get("token")); err != nil {
		if errors.Is(err, services.ErrInvalidVerificationToken) {
			data.Error = "This confirmation link is invalid or has expired."
			s.renderStatus(w, http.StatusBadRequest, templateConfirmEmail, data)
			return
		}

		s.logger.Error("confirm email", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	s.render(w, templateConfirmEmail, data)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	user, session, err := s.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		data := s.newTemplateData(r, "Sign In")
		data.Email = strings.TrimSpace(r.FormValue("email"))
		data.Next = safeRedirectPath(r.FormValue("next"))
		s.handleAuthFormError(w, r, templateLogin, fragmentLoginForm, data, err)
		return
	}

	next := safeRedirectPath(r.FormValue("next"))
	if !user.EmailVerifiedAt.Valid {
		next = paths.VerifyEmail
	} else if next == "" {
		next = paths.Account
	}

	s.setSessionCookie(w, r, session)
	s.redirectWithHTMX(w, r, next)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		if err := s.auth.Logout(r.Context(), cookie.Value); err != nil && !errors.Is(err, services.ErrInvalidSession) {
			s.logger.Error("logout", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.clearSessionCookie(w, r)
	http.Redirect(w, r, paths.Home, http.StatusSeeOther)
}

func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	s.render(w, templateAccount, s.newTemplateData(r, "Account"))
}

func (s *Server) changePasswordForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, templateChangePassword, s.newChangePasswordTemplateData(r))
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
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

	data := s.newTemplateData(r, "Verify Email")
	if err := s.auth.ResendVerificationEmail(r.Context(), user); err != nil {
		s.logger.Error("resend verification email", "user_id", user.ID, "err", err)
		if isHXRequest(r) {
			data.ResendStatus = statusError
			s.renderFragmentStatus(w, http.StatusOK, templateVerifyEmail, fragmentVerifyEmailResend, data)
			return
		}
		http.Redirect(w, r, withQueryParam(paths.VerifyEmail, queryKeyResend, statusError), http.StatusSeeOther)
		return
	}

	if isHXRequest(r) {
		data.ResendStatus = statusSent
		s.renderFragmentStatus(w, http.StatusOK, templateVerifyEmail, fragmentVerifyEmailResend, data)
		return
	}
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
		s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateChangePassword, fragmentChangePasswordForm, data)
		return
	}

	if err := s.auth.ChangePassword(r.Context(), user, currentPassword, newPassword); err != nil {
		data := s.newChangePasswordTemplateData(r)
		data.Error = "Check your details and try again."
		switch {
		case errors.Is(err, services.ErrCurrentPasswordIncorrect):
			data.FieldErrors = map[string]string{"current_password": "Current password is not correct."}
			s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateChangePassword, fragmentChangePasswordForm, data)
			return
		case errors.Is(err, services.ErrInvalidPassword):
			data.FieldErrors = map[string]string{"new_password": fmt.Sprintf("Use at least %d characters.", data.PasswordMinLength)}
			s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateChangePassword, fragmentChangePasswordForm, data)
			return
		case errors.Is(err, services.ErrPasswordUnchanged):
			data.FieldErrors = map[string]string{"new_password": "Choose a different password."}
			s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateChangePassword, fragmentChangePasswordForm, data)
			return
		default:
			s.logger.Error("change password", "user_id", user.ID, "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.clearSessionCookie(w, r)
	s.redirectWithHTMX(w, r, loginPathWithStatusChanged)
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	token := strings.TrimSpace(r.FormValue("token"))
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	fieldErrors := s.validatePasswordPair(newPassword, confirmPassword, "new_password", "confirm_password")
	if len(fieldErrors) > 0 {
		data := s.newTemplateData(r, "Reset Password")
		data.ResetToken = token
		data.Error = "Check your details and try again."
		data.FieldErrors = fieldErrors
		s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateResetPassword, fragmentResetPassword, data)
		return
	}

	if err := s.auth.ResetPasswordWithToken(r.Context(), token, newPassword); err != nil {
		data := s.newTemplateData(r, "Reset Password")
		data.ResetToken = token
		data.Error = "Check your details and try again."
		switch {
		case errors.Is(err, services.ErrInvalidPasswordResetToken):
			data.Error = "This password reset link is invalid or has expired."
			data.ResetTokenInvalid = true
			s.renderStatusForRequest(w, r, http.StatusBadRequest, templateResetPassword, fragmentResetPassword, data)
			return
		case errors.Is(err, services.ErrInvalidPassword):
			data.FieldErrors = map[string]string{"new_password": fmt.Sprintf("Use at least %d characters.", data.PasswordMinLength)}
			s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateResetPassword, fragmentResetPassword, data)
			return
		default:
			s.logger.Error("reset password", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}

	s.redirectWithHTMX(w, r, loginPathWithStatusChanged)
}

func (s *Server) resendVerificationPublic(w http.ResponseWriter, r *http.Request) {
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
			s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateResendVerification, fragmentResendVerification, data)
			return
		}

		s.logger.Error("resend verification email (public)", "err", err)
		if isHXRequest(r) {
			data := s.newTemplateData(r, "Resend Verification Email")
			data.ResendStatus = statusError
			s.renderFragmentStatus(w, http.StatusOK, templateResendVerification, fragmentResendVerification, data)
			return
		}
		http.Redirect(w, r, withQueryParam(paths.ResendVerification, queryKeyStatus, statusError), http.StatusSeeOther)
		return
	}

	if isHXRequest(r) {
		data := s.newTemplateData(r, "Resend Verification Email")
		data.ResendStatus = statusSent
		s.renderFragmentStatus(w, http.StatusOK, templateResendVerification, fragmentResendVerification, data)
		return
	}
	http.Redirect(w, r, withQueryParam(paths.ResendVerification, queryKeyStatus, statusSent), http.StatusSeeOther)
}

func (s *Server) handleAuthFormError(w http.ResponseWriter, r *http.Request, templateName, fragmentName string, data templateData, err error) {
	if errors.Is(err, services.ErrInvalidEmail) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"email": "Enter a valid email address."}
		s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateName, fragmentName, data)
		return
	}
	if errors.Is(err, services.ErrInvalidPassword) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"password": fmt.Sprintf("Use at least %d characters.", data.PasswordMinLength)}
		s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateName, fragmentName, data)
		return
	}
	if errors.Is(err, services.ErrEmailAlreadyRegistered) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"email": "An account with this email already exists."}
		s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateName, fragmentName, data)
		return
	}
	if errors.Is(err, services.ErrInvalidCredentials) {
		data.Error = "Email or password is not correct."
		s.renderStatusForRequest(w, r, http.StatusUnprocessableEntity, templateName, fragmentName, data)
		return
	}

	s.logger.Error("auth form", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Server) redirectWithHTMX(w http.ResponseWriter, r *http.Request, destination string) {
	if isHXRequest(r) {
		w.Header().Set("HX-Redirect", destination)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, destination, http.StatusSeeOther)
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

func (s *Server) render(w http.ResponseWriter, templateName string, data templateData) {
	s.renderStatus(w, http.StatusOK, templateName, data)
}

func (s *Server) renderStatus(w http.ResponseWriter, status int, templateName string, data templateData) {
	var body bytes.Buffer
	if err := s.renderTemplate(&body, templateName, data); err != nil {
		s.logger.Error("render template", "template", templateName, "err", err)
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

func (s *Server) renderFragmentStatus(w http.ResponseWriter, status int, templateName, fragmentName string, data templateData) {
	var body bytes.Buffer
	if err := s.renderTemplateFragment(&body, templateName, fragmentName, data); err != nil {
		s.logger.Error("render template fragment", "template", templateName, "fragment", fragmentName, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

func (s *Server) renderTemplateFragment(w io.Writer, templateName, fragmentName string, data templateData) error {
	tmpl, ok := s.templates[templateName]
	if !ok {
		return fmt.Errorf("template %q not found", templateName)
	}

	return tmpl.ExecuteTemplate(w, fragmentName, data)
}

func isHXRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-Request")), "true")
}

func (s *Server) renderStatusForRequest(w http.ResponseWriter, r *http.Request, status int, templateName, fragmentName string, data templateData) {
	if isHXRequest(r) {
		s.renderFragmentStatus(w, status, templateName, fragmentName, data)
		return
	}

	s.renderStatus(w, status, templateName, data)
}

func withQueryParam(basePath, key, value string) string {
	return basePath + "?" + key + "=" + value
}
