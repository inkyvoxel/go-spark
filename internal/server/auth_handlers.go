package server

import (
	"errors"
	"fmt"
	"net/http"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
)

type templateData struct {
	Title     string
	CSRFToken string
	Error     string
	User      db.User
}

func (s *Server) registerForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, "register.html", templateData{
		Title:     "Create Account",
		CSRFToken: csrfToken(r.Context()),
	})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	if _, err := s.auth.Register(r.Context(), email, password); err != nil {
		s.handleAuthFormError(w, r, "register.html", "Create Account", err)
		return
	}

	_, session, err := s.auth.Login(r.Context(), email, password)
	if err != nil {
		s.handleAuthFormError(w, r, "register.html", "Create Account", err)
		return
	}

	setSessionCookie(w, r, session)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login.html", templateData{
		Title:     "Sign In",
		CSRFToken: csrfToken(r.Context()),
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	_, session, err := s.auth.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		s.handleAuthFormError(w, r, "login.html", "Sign In", err)
		return
	}

	setSessionCookie(w, r, session)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
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

	clearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	s.render(w, "account.html", templateData{
		Title:     "Account",
		CSRFToken: csrfToken(r.Context()),
		User:      user,
	})
}

func (s *Server) handleAuthFormError(w http.ResponseWriter, r *http.Request, templateName, title string, err error) {
	if errors.Is(err, services.ErrInvalidEmail) ||
		errors.Is(err, services.ErrInvalidPassword) ||
		errors.Is(err, services.ErrInvalidCredentials) {
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, templateName, templateData{
			Title:     title,
			CSRFToken: csrfToken(r.Context()),
			Error:     "Check your details and try again.",
		})
		return
	}

	s.logger.Error("auth form", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *Server) render(w http.ResponseWriter, templateName string, data templateData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, templateName, data); err != nil {
		s.logger.Error("render template", "template", templateName, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, data templateData) error {
	tmpl, ok := s.templates[templateName]
	if !ok {
		return fmt.Errorf("template %q not found", templateName)
	}

	return tmpl.ExecuteTemplate(w, "layout.html", data)
}
