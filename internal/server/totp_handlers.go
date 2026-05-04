package server

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/inkyvoxel/go-spark/internal/paths"
	"github.com/inkyvoxel/go-spark/internal/services"
)

func (s *Server) newTwoFactorTemplateData(w http.ResponseWriter, r *http.Request) templateData {
	data := s.newTemplateData(w, r, "Two-Factor Authentication")
	data.Breadcrumbs = []breadcrumbItem{
		crumb("Account", paths.Account),
		currentCrumb("Two-factor authentication"),
	}
	return data
}

// twoFactorPage renders the 2FA management page (enable / setup / disable).
func (s *Server) twoFactorPage(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	data := s.newTwoFactorTemplateData(w, r)

	pending, enabled, err := s.auth.TOTPSetupState(r.Context(), user.ID)
	if err != nil {
		s.loggerForRequest(r).Error("get TOTP setup state", "user_id", user.ID, "err", err)
		s.internalServerError(w, r)
		return
	}

	data.TOTPPending = pending
	data.TOTPEnabled = enabled

	if enabled {
		_, remaining, err := s.auth.GetTOTPStatus(r.Context(), user.ID)
		if err != nil {
			s.loggerForRequest(r).Error("get TOTP status", "user_id", user.ID, "err", err)
			s.internalServerError(w, r)
			return
		}
		data.TOTPBackupCodesRemaining = remaining
	}

	if pending {
		secret, uri, err := s.auth.GetTOTPSetupInfo(r.Context(), user.ID)
		if err != nil {
			s.loggerForRequest(r).Error("get TOTP setup info", "user_id", user.ID, "err", err)
			s.internalServerError(w, r)
			return
		}
		data.TOTPSecret = secret
		data.TOTPOtpAuthURI = template.URL(uri)
	}

	s.render(w, templateTwoFactor, data)
}

// twoFactorSetup starts a new TOTP setup (generates secret, stores pending row).
func (s *Server) twoFactorSetup(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if _, _, err := s.auth.BeginTOTPSetup(r.Context(), user.ID); err != nil {
		if errors.Is(err, services.ErrTOTPAlreadyEnabled) {
			http.Redirect(w, r, paths.AccountTwoFactor, http.StatusSeeOther)
			return
		}
		s.loggerForRequest(r).Error("begin TOTP setup", "user_id", user.ID, "err", err)
		s.internalServerError(w, r)
		return
	}

	http.Redirect(w, r, paths.AccountTwoFactor, http.StatusSeeOther)
}

// twoFactorConfirm verifies the first code and activates 2FA.
// On success it renders the backup codes page directly (not a redirect) so the
// plaintext codes can be displayed once without being stored.
func (s *Server) twoFactorConfirm(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	backupCodes, err := s.auth.ConfirmTOTPSetup(r.Context(), user.ID, code)
	if err != nil {
		data := s.newTwoFactorTemplateData(w, r)
		data.TOTPPending = true
		switch {
		case errors.Is(err, services.ErrInvalidTOTPCode):
			data.Error = "That code is incorrect. Check your authenticator app and try again."
		case errors.Is(err, services.ErrTOTPSetupNotStarted):
			http.Redirect(w, r, paths.AccountTwoFactor, http.StatusSeeOther)
			return
		case errors.Is(err, services.ErrTOTPAlreadyEnabled):
			http.Redirect(w, r, paths.AccountTwoFactor, http.StatusSeeOther)
			return
		default:
			s.loggerForRequest(r).Error("confirm TOTP setup", "user_id", user.ID, "err", err)
			s.internalServerError(w, r)
			return
		}
		// Re-fetch setup info to re-render the QR code.
		secret, uri, infoErr := s.auth.GetTOTPSetupInfo(r.Context(), user.ID)
		if infoErr != nil {
			s.loggerForRequest(r).Error("get TOTP setup info after confirm error", "user_id", user.ID, "err", infoErr)
			s.internalServerError(w, r)
			return
		}
		data.TOTPSecret = secret
		data.TOTPOtpAuthURI = template.URL(uri)
		s.renderStatus(w, http.StatusUnprocessableEntity, templateTwoFactor, data)
		return
	}

	s.loggerForRequest(r).Info("TOTP enabled", "user_id", user.ID)
	data := s.newTwoFactorTemplateData(w, r)
	data.Title = "Save Your Backup Codes"
	data.TOTPEnabled = true
	data.TOTPBackupCodes = backupCodes
	s.render(w, templateTwoFactorBackupCodes, data)
}

// twoFactorDisable disables 2FA after verifying a valid TOTP code.
func (s *Server) twoFactorDisable(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	if err := s.auth.DisableTOTP(r.Context(), user.ID, code); err != nil {
		data := s.newTwoFactorTemplateData(w, r)
		data.TOTPEnabled = true
		switch {
		case errors.Is(err, services.ErrInvalidTOTPCode):
			data.Error = "That code is incorrect. Check your authenticator app and try again."
		case errors.Is(err, services.ErrTOTPNotEnabled):
			http.Redirect(w, r, paths.AccountTwoFactor, http.StatusSeeOther)
			return
		default:
			s.loggerForRequest(r).Error("disable TOTP", "user_id", user.ID, "err", err)
			s.internalServerError(w, r)
			return
		}
		_, remaining, _ := s.auth.GetTOTPStatus(r.Context(), user.ID)
		data.TOTPBackupCodesRemaining = remaining
		s.renderStatus(w, http.StatusUnprocessableEntity, templateTwoFactor, data)
		return
	}

	s.loggerForRequest(r).Info("TOTP disabled", "user_id", user.ID)
	s.setFlash(w, r, flashSuccess("Two-factor authentication has been disabled."))
	http.Redirect(w, r, paths.AccountTwoFactor, http.StatusSeeOther)
}

// twoFactorChallengeForm renders the TOTP challenge page shown mid-login.
func (s *Server) twoFactorChallengeForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.totpPendingLoginFromCookie(r); !ok {
		http.Redirect(w, r, paths.Login, http.StatusSeeOther)
		return
	}

	data := s.newTemplateData(w, r, "Two-Factor Authentication")
	data.Next = safeRedirectPath(r.URL.Query().Get("next"))
	s.render(w, templateTwoFactorChallenge, data)
}

// twoFactorChallenge verifies the TOTP code (or backup code) and issues the session.
func (s *Server) twoFactorChallenge(w http.ResponseWriter, r *http.Request) {
	pendingLogin, ok := s.totpPendingLoginFromCookie(r)
	if !ok {
		http.Redirect(w, r, paths.Login, http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	next := safeRedirectPath(r.FormValue("next"))

	session, err := s.auth.VerifyTOTPLogin(r.Context(), pendingLogin.UserID, code)
	if err != nil {
		data := s.newTemplateData(w, r, "Two-Factor Authentication")
		data.Next = next
		switch {
		case errors.Is(err, services.ErrInvalidTOTPCode):
			data.Error = "That code is incorrect. Try again or use a backup code."
		default:
			s.loggerForRequest(r).Error("verify TOTP login", "user_id", pendingLogin.UserID, "err", err)
			s.internalServerError(w, r)
			return
		}
		s.renderStatus(w, http.StatusUnprocessableEntity, templateTwoFactorChallenge, data)
		return
	}

	s.clearTOTPPendingCookie(w, r)
	s.setSessionCookie(w, r, session, pendingLogin.RememberMe)
	if err := s.rotateCSRFCookieForSession(w, r, session.Token); err != nil {
		s.loggerForRequest(r).Error("rotate csrf token after TOTP login", "err", err)
		s.internalServerError(w, r)
		return
	}

	s.loggerForRequest(r).Info("auth TOTP login succeeded", "user_id", pendingLogin.UserID)
	if next == "" {
		next = paths.Account
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}
