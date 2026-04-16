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
	"github.com/inkyvoxel/go-spark/internal/services"
)

const emailPattern = `[^@\s]+@[^@\s]+\.[^@\s]+`

type templateData struct {
	Title             string
	CSRFToken         string
	Error             string
	FieldErrors       map[string]string
	Email             string
	EmailPattern      string
	Next              string
	Authenticated     bool
	User              db.User
	PasswordMinLength int
	ResendStatus      string
}

func newTemplateData(r *http.Request, title string) templateData {
	data := templateData{
		Title:             title,
		CSRFToken:         csrfToken(r.Context()),
		EmailPattern:      emailPattern,
		PasswordMinLength: services.DefaultPasswordMinLength,
	}
	if user, ok := currentUser(r.Context()); ok {
		data.Authenticated = true
		data.User = user
	}
	return data
}

func (s *Server) registerForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, "register.html", s.newTemplateData(r, "Create Account"))
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
		s.renderStatus(w, http.StatusBadRequest, "register.html", data)
		return
	}

	if _, err := s.auth.Register(r.Context(), email, password); err != nil {
		data := s.newTemplateData(r, "Create Account")
		data.Email = strings.TrimSpace(email)
		s.handleAuthFormError(w, "register.html", data, err)
		return
	}

	_, session, err := s.auth.Login(r.Context(), email, password)
	if err != nil {
		data := s.newTemplateData(r, "Create Account")
		data.Email = strings.TrimSpace(email)
		s.handleAuthFormError(w, "register.html", data, err)
		return
	}

	s.setSessionCookie(w, r, session)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Sign In")
	data.Next = safeRedirectPath(r.URL.Query().Get("next"))
	s.render(w, "login.html", data)
}

func (s *Server) resendVerificationForm(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Resend Verification Email")
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "sent" || status == "error" {
		data.ResendStatus = status
	}
	s.render(w, "resend_verification.html", data)
}

func (s *Server) confirmEmail(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Confirm Email")

	if _, err := s.auth.VerifyEmail(r.Context(), r.URL.Query().Get("token")); err != nil {
		if errors.Is(err, services.ErrInvalidVerificationToken) {
			data.Error = "This confirmation link is invalid or has expired."
			s.renderStatus(w, http.StatusBadRequest, "confirm_email.html", data)
			return
		}

		s.logger.Error("confirm email", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	s.render(w, "confirm_email.html", data)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	_, session, err := s.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		data := s.newTemplateData(r, "Sign In")
		data.Email = strings.TrimSpace(r.FormValue("email"))
		data.Next = safeRedirectPath(r.FormValue("next"))
		s.handleAuthFormError(w, "login.html", data, err)
		return
	}

	next := safeRedirectPath(r.FormValue("next"))
	if next == "" {
		next = "/account"
	}

	s.setSessionCookie(w, r, session)
	http.Redirect(w, r, next, http.StatusSeeOther)
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
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	data := s.newTemplateData(r, "Account")
	status := strings.TrimSpace(r.URL.Query().Get("resend"))
	if status == "sent" || status == "error" {
		data.ResendStatus = status
	}
	s.render(w, "account.html", data)
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := s.auth.ResendVerificationEmail(r.Context(), user); err != nil {
		s.logger.Error("resend verification email", "user_id", user.ID, "err", err)
		http.Redirect(w, r, "/account?resend=error", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/account?resend=sent", http.StatusSeeOther)
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
			s.renderStatus(w, http.StatusBadRequest, "resend_verification.html", data)
			return
		}

		s.logger.Error("resend verification email (public)", "err", err)
		http.Redirect(w, r, "/resend-verification?status=error", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/resend-verification?status=sent", http.StatusSeeOther)
}

func (s *Server) handleAuthFormError(w http.ResponseWriter, templateName string, data templateData, err error) {
	if errors.Is(err, services.ErrInvalidEmail) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"email": "Enter a valid email address."}
		s.renderStatus(w, http.StatusBadRequest, templateName, data)
		return
	}
	if errors.Is(err, services.ErrInvalidPassword) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"password": fmt.Sprintf("Use at least %d characters.", data.PasswordMinLength)}
		s.renderStatus(w, http.StatusBadRequest, templateName, data)
		return
	}
	if errors.Is(err, services.ErrEmailAlreadyRegistered) {
		data.Error = "Check your details and try again."
		data.FieldErrors = map[string]string{"email": "An account with this email already exists."}
		s.renderStatus(w, http.StatusBadRequest, templateName, data)
		return
	}
	if errors.Is(err, services.ErrInvalidCredentials) {
		data.Error = "Email or password is not correct."
		s.renderStatus(w, http.StatusBadRequest, templateName, data)
		return
	}

	s.logger.Error("auth form", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Server) validateRegistrationForm(email, password, confirmPassword string) map[string]string {
	passwordMinLength := s.passwordMinLength
	if passwordMinLength == 0 {
		passwordMinLength = services.DefaultPasswordMinLength
	}

	fieldErrors := make(map[string]string)
	if strings.TrimSpace(email) == "" {
		fieldErrors["email"] = "Enter your email address."
	}
	if password == "" {
		fieldErrors["password"] = "Enter a password."
	} else if utf8.RuneCountInString(password) < passwordMinLength {
		fieldErrors["password"] = fmt.Sprintf("Use at least %d characters.", passwordMinLength)
	}
	if confirmPassword == "" {
		fieldErrors["confirm_password"] = "Confirm your password."
	} else if password != confirmPassword {
		fieldErrors["confirm_password"] = "Passwords do not match."
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

	return tmpl.ExecuteTemplate(w, "layout.html", data)
}
