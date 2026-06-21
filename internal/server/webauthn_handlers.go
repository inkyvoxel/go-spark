package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/inkyvoxel/go-spark/internal/auth"
	"github.com/inkyvoxel/go-spark/internal/paths"
)

func (s *Server) newPasskeysTemplateData(w http.ResponseWriter, r *http.Request) templateData {
	data := s.newTemplateData(w, r, "Passkeys")
	data.Breadcrumbs = []breadcrumbItem{
		crumb("Account", paths.Account),
		currentCrumb("Passkeys"),
	}
	return data
}

// passkeysPage renders the passkey management page.
func (s *Server) passkeysPage(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	data := s.newPasskeysTemplateData(w, r)
	if data.PasskeysEnabled {
		passkeys, err := s.auth.ListPasskeys(r.Context(), user.ID)
		if err != nil {
			s.loggerForRequest(r).Error("list passkeys", "user_id", user.ID, "err", err)
			s.internalServerError(w, r)
			return
		}
		data.Passkeys = passkeys
	}
	s.render(w, templatePasskeys, data)
}

// passkeyRegisterBegin returns the credential-creation options as JSON and stores
// the ceremony session data in a signed cookie.
func (s *Server) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	creation, session, err := s.auth.BeginPasskeyRegistration(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, auth.ErrPasskeysDisabled) {
			s.writeJSONError(w, http.StatusNotFound, "Passkeys are not available.")
			return
		}
		s.loggerForRequest(r).Error("begin passkey registration", "user_id", user.ID, "err", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Could not start passkey registration.")
		return
	}

	if err := s.setPasskeySessionCookie(w, r, passkeyRegisterCookieName, passkeyRegisterCookiePath, session); err != nil {
		s.loggerForRequest(r).Error("store passkey registration session", "user_id", user.ID, "err", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Could not start passkey registration.")
		return
	}
	s.writeJSON(w, http.StatusOK, creation)
}

// passkeyRegisterFinish verifies the authenticator response and stores the
// credential. The label is taken from the "name" query parameter.
func (s *Server) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	session, ok := s.passkeySessionFromCookie(r, passkeyRegisterCookieName)
	if !ok {
		s.writeJSONError(w, http.StatusBadRequest, "Your passkey registration expired. Please try again.")
		return
	}
	s.clearPasskeySessionCookie(w, r, passkeyRegisterCookieName, passkeyRegisterCookiePath)

	parsed, err := protocol.ParseCredentialCreationResponseBody(r.Body)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "That passkey could not be read. Please try again.")
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if err := s.auth.FinishPasskeyRegistration(r.Context(), user.ID, session, parsed, name); err != nil {
		if errors.Is(err, auth.ErrPasskeyCeremony) {
			s.writeJSONError(w, http.StatusUnprocessableEntity, "That passkey could not be verified. Please try again.")
			return
		}
		s.loggerForRequest(r).Error("finish passkey registration", "user_id", user.ID, "err", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Could not save your passkey.")
		return
	}

	s.loggerForRequest(r).Info("passkey registered", "user_id", user.ID)
	s.setFlash(w, r, flashSuccess("Passkey added."))
	s.writeJSON(w, http.StatusOK, map[string]string{"redirect": paths.AccountPasskeys})
}

// passkeyRename updates a credential's label from a standard form post.
func (s *Server) passkeyRename(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	credentialID, err := s.passkeyIDFromForm(r.FormValue("credential_id"))
	if err != nil {
		s.setFlash(w, r, flashError("Select a valid passkey to rename."))
		http.Redirect(w, r, paths.AccountPasskeys, http.StatusSeeOther)
		return
	}

	if err := s.auth.RenamePasskey(r.Context(), user.ID, credentialID, r.FormValue("name")); err != nil {
		if errors.Is(err, auth.ErrPasskeyNotFound) {
			s.setFlash(w, r, flashError("That passkey is no longer available."))
			http.Redirect(w, r, paths.AccountPasskeys, http.StatusSeeOther)
			return
		}
		s.loggerForRequest(r).Error("rename passkey", "user_id", user.ID, "err", err)
		s.internalServerError(w, r)
		return
	}

	s.loggerForRequest(r).Info("passkey renamed", "user_id", user.ID, "credential_id", credentialID)
	s.setFlash(w, r, flashSuccess("Passkey renamed."))
	http.Redirect(w, r, paths.AccountPasskeys, http.StatusSeeOther)
}

// passkeyDelete removes a credential from a standard form post.
func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	credentialID, err := s.passkeyIDFromForm(r.FormValue("credential_id"))
	if err != nil {
		s.setFlash(w, r, flashError("Select a valid passkey to remove."))
		http.Redirect(w, r, paths.AccountPasskeys, http.StatusSeeOther)
		return
	}

	if err := s.auth.DeletePasskey(r.Context(), user.ID, credentialID); err != nil {
		if errors.Is(err, auth.ErrPasskeyNotFound) {
			s.setFlash(w, r, flashError("That passkey is no longer available."))
			http.Redirect(w, r, paths.AccountPasskeys, http.StatusSeeOther)
			return
		}
		s.loggerForRequest(r).Error("delete passkey", "user_id", user.ID, "err", err)
		s.internalServerError(w, r)
		return
	}

	s.loggerForRequest(r).Info("passkey deleted", "user_id", user.ID, "credential_id", credentialID)
	s.setFlash(w, r, flashSuccess("Passkey removed."))
	http.Redirect(w, r, paths.AccountPasskeys, http.StatusSeeOther)
}

// passkeyLoginBegin returns discoverable assertion options as JSON and stores the
// ceremony session data in a signed cookie.
func (s *Server) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	assertion, session, err := s.auth.BeginPasskeyLogin(r.Context())
	if err != nil {
		if errors.Is(err, auth.ErrPasskeysDisabled) {
			s.writeJSONError(w, http.StatusNotFound, "Passkeys are not available.")
			return
		}
		s.loggerForRequest(r).Error("begin passkey login", "err", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Could not start passkey sign-in.")
		return
	}

	if err := s.setPasskeySessionCookie(w, r, passkeyLoginCookieName, passkeyLoginCookiePath, session); err != nil {
		s.loggerForRequest(r).Error("store passkey login session", "err", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Could not start passkey sign-in.")
		return
	}
	s.writeJSON(w, http.StatusOK, assertion)
}

// passkeyLoginFinish validates the assertion, issues a session, and returns the
// redirect target as JSON for the client to follow.
func (s *Server) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	session, ok := s.passkeySessionFromCookie(r, passkeyLoginCookieName)
	if !ok {
		s.writeJSONError(w, http.StatusBadRequest, "Your sign-in attempt expired. Please try again.")
		return
	}
	s.clearPasskeySessionCookie(w, r, passkeyLoginCookieName, passkeyLoginCookiePath)

	parsed, err := protocol.ParseCredentialRequestResponseBody(r.Body)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "That passkey could not be read. Please try again.")
		return
	}

	user, authSession, err := s.auth.FinishPasskeyLogin(r.Context(), session, parsed)
	if err != nil {
		if errors.Is(err, auth.ErrPasskeyCeremony) || errors.Is(err, auth.ErrPasskeyNotFound) {
			s.loggerForRequest(r).Info("passkey login failed")
			s.writeJSONError(w, http.StatusUnauthorized, "That passkey could not be verified. Please try again.")
			return
		}
		s.loggerForRequest(r).Error("finish passkey login", "err", err)
		s.writeJSONError(w, http.StatusInternalServerError, "Could not complete passkey sign-in.")
		return
	}

	s.setSessionCookie(w, r, authSession, false)

	next := safeRedirectPath(r.URL.Query().Get("next"))
	if !user.EmailVerifiedAt.Valid {
		next = paths.VerifyEmail
	} else if next == "" {
		next = paths.Account
	}

	s.loggerForRequest(r).Info("auth passkey login succeeded", "user_id", user.ID)
	s.writeJSON(w, http.StatusOK, map[string]string{"redirect": next})
}

func (s *Server) passkeyIDFromForm(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid passkey id")
	}
	return id, nil
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) writeJSONError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]string{"error": message})
}
