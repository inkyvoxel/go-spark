# Reusable Components

This template ships two built-in UI components — flash messages and breadcrumbs — that follow a consistent pattern. Both are template partials registered globally, so any page gets them for free.

---

## Flash Messages

Flash messages display a one-time notification after a redirect (the Post/Redirect/Get pattern). They are stored in a short-lived signed cookie, consumed on the very next page load, and then cleared automatically.

### How it works

1. In a handler, call `s.setFlash(w, r, msg)` before redirecting.
2. On the next request, `s.newTemplateData(w, r, title)` calls `s.popFlash` internally — it reads the cookie, verifies the HMAC signature, clears the cookie, and sets `data.Flash`.
3. The `{{ template "flash" . }}` call in `layout.html` renders the message at the top of every page without any per-template changes.

### Usage

```go
// Success message
s.setFlash(w, r, flashSuccess("Your password has been changed."))
http.Redirect(w, r, paths.Login, http.StatusSeeOther)

// Error message
s.setFlash(w, r, flashError("Something went wrong. Please try again."))
http.Redirect(w, r, paths.SomePage, http.StatusSeeOther)
```

### Constructor functions

| Function | Rendered as |
|---|---|
| `flashSuccess(msg)` | `<p role="status">` (Pico CSS success style) |
| `flashError(msg)` | `<p role="alert">` (Pico CSS error style) |

### Security

Flash cookies are HMAC-SHA256 signed using a key derived from `SECRET_KEY_BASE`. Tampered or unsigned cookies are silently ignored.

---

## Breadcrumbs

Breadcrumbs show the user's position within the page hierarchy. They are set per-handler and rendered via the `{{ template "breadcrumb" . }}` partial.

### How it works

Set `data.Breadcrumbs` in the handler (or in a `newXxxTemplateData` helper). The `breadcrumb.html` partial renders a `<nav aria-label="breadcrumb">` only when the slice is non-empty — pages without breadcrumbs need no special handling.

### Usage

```go
func (s *Server) newSettingsTemplateData(w http.ResponseWriter, r *http.Request) templateData {
    data := s.newTemplateData(w, r, "Settings")
    data.Breadcrumbs = []breadcrumbItem{
        crumb("Account", paths.Account),
        currentCrumb("Settings"),
    }
    return data
}
```

Use `{{ template "breadcrumb" . }}` in the page template wherever the breadcrumb trail should appear (typically just before the `<h1>`):

```html
{{ define "content" }}
<article>
  {{ template "breadcrumb" . }}
  <h1>Settings</h1>
  ...
</article>
{{ end }}
```

### Helper functions

| Function | Description |
|---|---|
| `crumb(label, url)` | A linked breadcrumb item |
| `currentCrumb(label)` | The current page item (no link, `aria-current="page"`) |

---

## Secret Key Derivation

Keys are derived from two roots, split by **rotation profile** rather than by
feature. This keeps the cheap-to-rotate signing keys separate from the keys that
protect data at rest, so rotating one does not collaterally invalidate the other.

`SECRET_KEY_BASE` — signing and ephemeral state. Rotating it is cheap: no stored
data depends on it.

```
cookieSigningKey  = HMAC-SHA256(SECRET_KEY_BASE, "cookie-signing")
flashKey          = HMAC-SHA256(SECRET_KEY_BASE, "flash")
totpPendingKey    = HMAC-SHA256(cookieSigningKey, "totp_pending")
passkeySessionKey = HMAC-SHA256(cookieSigningKey, "passkey_session")
```

`AUTH_TOTP_KEY` — TOTP data at rest. Rotating it forces every 2FA user to
re-enrol, so it is rotated rarely and deliberately (the same contract as
`AUTH_PASSWORD_PEPPER` for passwords).

```
totpSecretKey    = HMAC-SHA256(AUTH_TOTP_KEY, "totp_secret")
totpBackupKey    = HMAC-SHA256(AUTH_TOTP_KEY, "totp_backup_code")
```

`totpSecretKey` is the AES-256-GCM key used to encrypt TOTP shared secrets at
rest (see `internal/database/crypto.go`). Unlike backup codes, which are one-way
hashed with `totpBackupKey`, the shared secret must be recoverable to verify
codes, so it is encrypted rather than hashed — a leaked database backup does not
expose users' second factors. Both derive from `AUTH_TOTP_KEY` because they share
one rotation profile: there is no scenario where you would rotate one but not the
other.

Note that session validity does **not** depend on either root — session tokens
are random values stored as unkeyed SHA-256 hashes — so rotating `SECRET_KEY_BASE`
does not sign users out.

Every key above is produced by `services.DeriveKey(root, "purpose")` — a single
HMAC-SHA256 helper — so all derivations share one implementation. To add a new
signing purpose, call `services.DeriveKey` with a new, unique purpose string:
in the server package derive it during server initialisation and store it as a
field on `Server`; application-wide service keys are derived during runtime
assembly in `internal/app`. This follows the Rails `secret_key_base` pattern —
one root secret, isolated derived keys.
