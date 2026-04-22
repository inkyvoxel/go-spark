## Email features

- Admin/ops visibility for failed outbox rows.

## Security

- Convention: public paths live in `internal/paths`, templates receive links through `.Routes`, and server template/fragment names are centralized as package-level constants. Avoid inline string literals for routable endpoints, template links/actions, and render target templates/fragments.

- Security audit report completed on 2026-04-17. Scope reviewed: HTTP routing and middleware, auth/session/password reset/email verification flows, CSRF, rate limiting, templates, config, SQLite storage, migrations, email worker/SMTP, docs, and tests. Existing tests pass with `go test ./...`. `govulncheck` has since been added as a pinned Go tool; run `make vulncheck` for dependency and standard-library vulnerability scanning.

### Executive summary

- Overall foundation is solid for a starter: server-side sessions, HttpOnly cookies, SameSite=Lax, CSRF tokens, Argon2id password hashing, random 32-byte tokens, hashed reset/verification tokens, SQL parameterization through sqlc, safe redirect validation, transactional outbox writes, request timeouts, and auth rate limiting are all present.
- No obvious SQL injection, reflected XSS through templates, open redirect, or token entropy issue was found during this review.
- The biggest remaining hardening gaps are production safety rails, session token storage design, CSRF strengthening, cache-control for auth-sensitive pages, and operational maturity around shared rate limits, dependency scanning, and outbox recovery.

### Medium priority

- [ ] Add database cleanup jobs for expired sessions, consumed/expired reset tokens, consumed/expired verification tokens, and old sent/failed email rows.
  - Evidence: queries ignore expired sessions/tokens, but migrations do not include cleanup behavior and the app does not schedule pruning.
  - Risk: sensitive metadata and operational data accumulate indefinitely, increasing breach impact and storage growth.
  - Recommendation: add periodic cleanup with conservative retention windows and tests.

### Positive findings to preserve

- Passwords use Argon2id with OWASP-minimum parameters, unique salts, constant-time hash comparison, optional peppering, and a 12-character default minimum.
- Tokens are generated from `crypto/rand` with a minimum of 32 bytes and hex encoded.
- Password reset and email verification tokens are stored hashed and consumed atomically with expiry checks.
- Password change/reset revokes existing sessions.
- SQL uses generated parameterized queries; no dynamic SQL construction was found.
- Templates use `html/template`; user-controlled values reviewed here are escaped by default.
- Redirects through `next` are constrained to safe relative paths.
- Auth POST routes have CSRF protection, and session cookies are HttpOnly with SameSite=Lax.
- SMTP uses STARTTLS by default, requires TLS support when enabled, sets `ServerName`, and uses TLS 1.2 minimum.

### Decisions recorded

- Production mode should fail startup when `AUTH_PASSWORD_PEPPER` is blank.
- The starter should block account access until email verification, with only verification-related actions and logout available to unverified users.
- Session tokens should be hashed at rest because it is a best-practice hardening measure for database-backed opaque sessions.
- Password reset should move toward a short-lived HttpOnly reset cookie flow because it reduces bearer-token exposure in URLs, logs, browser history, and page HTML.
