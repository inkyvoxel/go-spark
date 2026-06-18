# To-do

Working list toward a v1 feature-complete release. Tackle top to bottom.
Each item notes the why and the main files to touch.

## 1. Consolidate the signed-cookie helpers

Three signed cookies still each re-implement `base64(payload).base64(hmac)`
sign/verify — flash (`internal/server/flash.go`), the TOTP-pending cookie
(`internal/server/auth.go`), and the passkey ceremony cookie
(`internal/server/webauthn.go`, keyed via `cookieSigningKey`). Collapse into one
`signValue(key, payload)` / `verifyValue(key, token)` helper so the MAC logic
lives in one auditable place.

- Files: `internal/server/flash.go`, `internal/server/auth.go`,
  `internal/server/webauthn.go`.

## 2. Decide email-change session revocation policy

Password change revokes all sessions (`SetPasswordAndRevokeSessions`); email
change does not. Defensible (change requires current password + link to the new
address), but if email is treated as a login identifier, revoking on change is
more consistent. Pick a stance and leave a comment recording it.

- Files: `internal/services/auth.go` (`ConfirmEmailChange` / store path).

## 3. Harden init.sh substring + sed edge cases

`scripts/init.sh` replaces module path, then `Go Spark`, then `go-spark`. If a
user's module path itself contains `go-spark` (e.g.
`github.com/me/go-spark-fork`), the third pass corrupts it. `sed` replacement
also breaks if the module path/name contains `&` or `|`.

- Add a guard/escape or at least a documented note. CI `init-script` job already
  covers the common case.
- Files: `scripts/init.sh`.

## 4. Minor cleanups

- `.gitignore` references `.gospark-init-state`, which nothing creates. Remove
  the line.
- `internal/server/architecture_boundary_test.go` calls `t.Helper()` inside a
  top-level `Test` function (no-op there). Drop it.

## 5. Write the CHANGELOG before tagging v1

`CHANGELOG.md` is still all "Unreleased / Initial scaffold". Fill in a real v1
entry before shipping a forkable release.

- Files: `CHANGELOG.md`.
