# To-do

Working list toward a v1 feature-complete release. Tackle top to bottom.
Each item notes the why and the main files to touch.

## 1. Ship an example non-auth feature (the "projects" CRUD)

Biggest feature-completeness / fork-ergonomics win. The repo currently ships
only auth, so a forker reconstructs a vertical slice from prose in
`docs/extending.md`. Implement one real authenticated CRUD end-to-end so they
can copy a working example.

- path → route → handler → service → store → SQL → migration → template.
- Mirror the exact layering `docs/extending.md` already describes ("projects").
- Files: `internal/paths/paths.go`, `internal/server/`, `internal/services/`,
  `internal/database/`, `internal/db/queries/`, `migrations/`, `templates/`,
  `internal/app/build.go`. Add focused tests at each layer.

## 2. Apply security headers to static + health endpoints

`/static/`, `/healthz`, `/readyz`, `/robots.txt` are registered on the outer
mux before the `securityHeaders` wrapper (which only wraps the `paths.Home`
subtree), so served JS/CSS go out without `X-Content-Type-Options: nosniff`.
Low impact (same-origin, correct content types) but cheap to close.

- Either wrap the whole mux, or set `nosniff` in `staticFileHandler`.
- Files: `internal/server/server.go` (`Routes`, `staticFileHandler`,
  `securityHeaders`).

## 3. Add a maximum password length

`validatePassword` checks only the minimum. Long-password Argon2id DoS is
currently prevented *only* because the mandatory pepper HMAC-pre-hashes input to
32 bytes. A forker who drops the pepper feeds up to 64KB straight into 19 MiB
Argon2id. Add an explicit upper bound (e.g. 4096 chars) so the protection
doesn't silently depend on the pepper.

- Files: `internal/services/auth.go` (`validatePassword`, registration path),
  matching form-level message in `internal/server/auth_handlers.go`.

## 4. Decide: hand-rolled OTP vs battle-tested library

`internal/totp/totp.go` is ~90 lines of own-rolled RFC 6238 (HMAC-SHA1). It's
tested and adds no dependency. A maintained library (e.g. `pquerna/otp`) means
no crypto to own/audit, at the cost of a dependency. Slight edge to a library
for a security-focused template (forkers don't have to audit it), but keeping
the current impl is defensible. Make the call and record it.

- If switching: replace `internal/totp`, keep the at-rest encryption and
  replay-counter logic in `internal/services/totp.go` unchanged.
- Files: `internal/totp/totp.go`, `internal/services/totp.go`.

## 5. Consolidate the signed-cookie helpers

Three signed cookies still each re-implement `base64(payload).base64(hmac)`
sign/verify — flash (`internal/server/flash.go`), the TOTP-pending cookie
(`internal/server/auth.go`), and the passkey ceremony cookie
(`internal/server/webauthn.go`, keyed via `cookieSigningKey`). Collapse into one
`signValue(key, payload)` / `verifyValue(key, token)` helper so the MAC logic
lives in one auditable place.

- Files: `internal/server/flash.go`, `internal/server/auth.go`,
  `internal/server/webauthn.go`.

## 6. Decide email-change session revocation policy

Password change revokes all sessions (`SetPasswordAndRevokeSessions`); email
change does not. Defensible (change requires current password + link to the new
address), but if email is treated as a login identifier, revoking on change is
more consistent. Pick a stance and leave a comment recording it.

- Files: `internal/services/auth.go` (`ConfirmEmailChange` / store path).

## 7. Harden init.sh substring + sed edge cases

`scripts/init.sh` replaces module path, then `Go Spark`, then `go-spark`. If a
user's module path itself contains `go-spark` (e.g.
`github.com/me/go-spark-fork`), the third pass corrupts it. `sed` replacement
also breaks if the module path/name contains `&` or `|`.

- Add a guard/escape or at least a documented note. CI `init-script` job already
  covers the common case.
- Files: `scripts/init.sh`.

## 8. Minor cleanups

- `.gitignore` references `.gospark-init-state`, which nothing creates. Remove
  the line.
- `internal/server/architecture_boundary_test.go` calls `t.Helper()` inside a
  top-level `Test` function (no-op there). Drop it.

## 9. Write the CHANGELOG before tagging v1

`CHANGELOG.md` is still all "Unreleased / Initial scaffold". Fill in a real v1
entry before shipping a forkable release.

- Files: `CHANGELOG.md`.
