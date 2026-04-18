## Email features

What is still not implemented:
- Admin/ops visibility for failed outbox rows.
- Separate worker process mode.
- HTML email styling beyond the simple message body.

## Account features

- Change email address securely

## Security

- Security audit report completed on 2026-04-17. Scope reviewed: HTTP routing and middleware, auth/session/password reset/email verification flows, CSRF, rate limiting, templates, config, SQLite storage, migrations, email worker/SMTP, docs, and tests. Existing tests pass with `go test ./...`. `govulncheck` has since been added as a pinned Go tool; run `make vulncheck` for dependency and standard-library vulnerability scanning.

### Executive summary

- Overall foundation is solid for a starter: server-side sessions, HttpOnly cookies, SameSite=Lax, CSRF tokens, Argon2id password hashing, random 32-byte tokens, hashed reset/verification tokens, SQL parameterization through sqlc, safe redirect validation, transactional outbox writes, request timeouts, and auth rate limiting are all present.
- No obvious SQL injection, reflected XSS through templates, open redirect, or token entropy issue was found during this review.
- The biggest hardening gaps are production safety rails, browser security headers/CSP, request body limits, session token storage design, email verification policy, and operational maturity around rate limits, dependency scanning, and outbox recovery.

### High priority

- [ ] Add production config fail-fast checks instead of warnings for unsafe deployment settings.
  - Evidence: `cmd/app/main.go` only warns when `APP_ENV=production` and `AUTH_PASSWORD_PEPPER` is blank. `internal/config/config.go` defaults `APP_COOKIE_SECURE=false`, `APP_BASE_URL=http://localhost:8080`, and `EMAIL_PROVIDER=log`.
  - Risk: a project can accidentally deploy with insecure cookies behind a TLS-terminating proxy, localhost links in security emails, log-only email delivery, no pepper, or HTTP base URLs.
  - Recommendation: when `APP_ENV=production`, fail startup unless `APP_COOKIE_SECURE=true`, `APP_BASE_URL` uses `https`, `AUTH_PASSWORD_PEPPER` is non-empty, `EMAIL_PROVIDER=smtp` or an explicit production provider is configured, and `EMAIL_LOG_BODY=false`. Consider also rejecting default `EMAIL_FROM`.
  - Decision: fail startup when `AUTH_PASSWORD_PEPPER` is missing in production.
  - Progress: `AUTH_PASSWORD_PEPPER` now fails fast in production; remaining production safety checks are still pending.

- [x] Add security response headers middleware.
  - Evidence: responses currently set content type in render helpers, but no global headers are added. `templates/layout.html` loads vendored CSS and HTMX locally, so the app is well-positioned for a strict policy.
  - Risk: missing defense-in-depth against clickjacking, MIME sniffing, referrer leakage of reset tokens, and script injection impact.
  - Recommendation: add middleware for at least `Content-Security-Policy`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin` or `no-referrer`, `Frame-Options: DENY` or CSP `frame-ancestors 'none'`, and `Permissions-Policy`. In production over HTTPS, add `Strict-Transport-Security`.
  - Deployment note: set baseline app-owned headers in Go so the starter is secure by default in every hosting environment. A production reverse proxy such as Caddy or nginx may also set or override deployment-specific headers, especially `Strict-Transport-Security`, but avoid maintaining conflicting policies in two places.
  - Suggested CSP starting point: `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'self'`.

- [x] Put explicit max request body limits on all form POST routes.
  - Evidence: handlers call `r.ParseForm()` directly in register, login, forgot password, reset password, resend verification, and change password flows.
  - Risk: attackers can send large form bodies to consume memory/CPU before validation and rate limits complete.
  - Recommendation: wrap form parsing with `http.MaxBytesReader`, for example 32-64 KiB for auth forms, and return `413 Request Entity Too Large` for oversized bodies.

- [ ] Hash session tokens at rest, like reset and verification tokens.
  - Evidence: sessions store and query raw `sessions.token`, while password reset and email verification store `token_hash`.
  - Risk: a database leak immediately grants active sessions until expiry. Reset/verification tokens are better protected than session tokens.
  - Recommendation: store `session_token_hash`, compare/query by hash, and keep only the raw token in the browser cookie. Plan a migration path that invalidates existing sessions or supports both columns briefly.
  - Decision: keep this on the list. This is a common best-practice hardening pattern for database-backed opaque session tokens because it reduces the blast radius of a database-only leak.

- [x] Block login/account access until email is verified, then implement consistently.
  - Evidence: `register` immediately logs the new user in after sending a verification email, and `Login` does not check `email_verified_at`. This is also listed under Email features.
  - Risk: apps cloned from the template may assume verified email means trusted account ownership, while unverified users can still access account features.
  - Recommendation: for this security-first starter, block sensitive account access until verified, allow resend/logout only, and show a clear interstitial.
  - Decision: make verified-email gating a proper implementation task.
  - Progress: implemented with a dedicated `/verify-email` interstitial, global `requireVerifiedAuth` middleware for verified-only pages/actions, login/register redirects to `/verify-email` for unverified users, and verified-only access to `/account`.

### Medium priority

- [ ] Make CSRF protection session-bound or signed, and consider rotating after login.
  - Evidence: CSRF uses a random cookie token with a matching hidden field/header. The cookie is HttpOnly, Secure when configured/TLS, SameSite=Lax, and valid for 24 hours.
  - Risk: the current double-submit-style token is not bound to the authenticated session. SameSite=Lax gives meaningful protection, but a signed/session-bound token is stronger and easier to reason about for a starter.
  - Recommendation: sign the CSRF token with an app secret or store the token server-side/session-side. Rotate CSRF token on login/logout or session changes.

- [ ] Replace hidden reset-token form flow with a short-lived HttpOnly reset cookie flow.
  - Evidence: `/reset-password?token=...` validates the token and renders it into a hidden form field.
  - Risk: reset tokens can appear in browser history, server/proxy logs, screenshots, extensions, and page source until consumed. The app should assume reset tokens are bearer credentials.
  - Recommendation: on GET, exchange a valid URL token for a short-lived HttpOnly reset cookie and redirect to `/reset-password` without the query string. On POST, consume the reset cookie plus CSRF token.
  - Decision: keep this on the list. The exact pattern is not universal in small apps, but reducing bearer-token exposure in URLs and HTML is a strong security practice for a starter template.

- [ ] Add rate limiting to `POST /reset-password` and `POST /account/change-password`.
  - Evidence: login, register, forgot password, and resend verification are rate-limited; reset password and change password are not.
  - Risk: reset token guessing is impractical with 32 random bytes, but rate limiting still reduces abuse, CPU burn from Argon2, and online attempts against current passwords in change-password.
  - Recommendation: add policies keyed by IP plus reset-token hash prefix for reset, and IP plus user ID for change-password.

- [ ] Add a production-grade rate limiter option for multi-instance deployments.
  - Evidence: the limiter is in-memory and keyed from `RemoteAddr`; README correctly documents that it is per instance and does not trust forwarded headers.
  - Risk: limits are bypassed across multiple app instances or after restarts, and reverse proxies can collapse many users into one `RemoteAddr` unless configured carefully.
  - Recommendation: keep the in-memory limiter for local/dev, but define a `rateLimitStore` adapter for Redis/Postgres/SQLite or document single-instance assumptions prominently. Add explicit trusted proxy configuration before honoring `X-Forwarded-For`.

- [ ] Add account/session management controls.
  - Evidence: sessions have expiry and are revoked on password change/reset, but users cannot see or revoke sessions manually.
  - Risk: stolen sessions remain valid until expiry unless the password is changed/reset.
  - Recommendation: add "sign out other sessions", session listing, last-used metadata, and optional session idle timeout.

- [ ] Add database cleanup jobs for expired sessions, consumed/expired reset tokens, consumed/expired verification tokens, and old sent/failed email rows.
  - Evidence: queries ignore expired sessions/tokens, but migrations do not include cleanup behavior and the app does not schedule pruning.
  - Risk: sensitive metadata and operational data accumulate indefinitely, increasing breach impact and storage growth.
  - Recommendation: add periodic cleanup with conservative retention windows and tests.

- [ ] Improve email outbox crash recovery.
  - Evidence: `ClaimPendingEmails` moves rows to `sending`; if the process crashes after claiming but before marking sent/failed, those rows can remain stuck.
  - Risk: password reset and verification emails may silently stop for affected rows.
  - Recommendation: add `claimed_at`/lease fields, retry stale `sending` rows, and cap/trim `last_error` to avoid unbounded database growth.

- [ ] Add dependency and supply-chain security checks to CI.
  - Evidence: `govulncheck` is now available via `make vulncheck`, but CI does not run it yet. `go list -m all` shows a large transitive dependency graph because tool dependencies such as sqlc/goose bring many packages.
  - Risk: security advisories may be missed, especially in tool dependencies and browser assets.
  - Recommendation: run `make vulncheck` in CI, run `go list -m -u all` periodically, pin and review vendored browser assets, and consider Dependabot/Renovate.

### Low priority / defense in depth

- [ ] Add cache-control headers for auth-sensitive pages.
  - Evidence: account and reset pages are rendered normally without explicit cache headers.
  - Risk: shared/private browser caches may retain account pages or reset forms.
  - Recommendation: set `Cache-Control: no-store` for `/account`, `/login`, `/register`, `/forgot-password`, `/reset-password`, `/confirm-email`, and form POST responses.

- [ ] Add `MaxAge` to the session cookie in addition to `Expires`.
  - Evidence: `setSessionCookie` sets `Expires` but not `MaxAge`.
  - Risk: modern browsers handle `Expires`, but `MaxAge` is less ambiguous and aligns with clear-session behavior.
  - Recommendation: set `MaxAge` from `session.ExpiresAt.Sub(time.Now())`.

- [ ] Consider `SameSite=Strict` for session cookies if product flows allow it.
  - Evidence: session and CSRF cookies use `SameSite=Lax`.
  - Risk: Lax is a reasonable default and permits normal cross-site top-level GET navigation, but Strict further reduces ambient-cookie exposure.
  - Recommendation: keep Lax if email-link and external navigation UX matters, or make SameSite configurable with Strict recommended for high-risk apps.

- [ ] Make static file serving more restrictive.
  - Evidence: `/static/` serves the whole `static` directory using `http.FileServer`.
  - Risk: future accidental files under `static` become public.
  - Recommendation: document that `static` is public, consider an embedded filesystem for releases, and add tests/checks to prevent secrets or source maps if undesired.

- [ ] Review health endpoint exposure.
  - Evidence: `/healthz` pings the database and returns `ok` or `database unavailable`.
  - Risk: public health endpoints reveal service/database availability.
  - Recommendation: keep simple liveness public if needed, but put detailed readiness behind infrastructure access controls or split `/livez` and `/readyz`.

- [ ] Add origin/referer validation as optional CSRF defense-in-depth.
  - Evidence: CSRF currently validates token only.
  - Risk: token validation is the main control, but Origin/Referer checks catch some malformed cross-site POSTs and misconfigurations.
  - Recommendation: for unsafe methods, verify `Origin` or `Referer` matches `APP_BASE_URL` when present, especially in production.

- [ ] Add brute-force timing and enumeration tests.
  - Evidence: invalid login uses a generic message, and forgot/resend flows avoid revealing non-existent accounts for valid emails. Invalid email format still returns validation errors.
  - Risk: response timing may still reveal account existence because existing-account flows perform database writes/email enqueue work and non-existing valid emails return quickly.
  - Recommendation: add tests/benchmarks around observable differences and consider uniform response timing or async job boundaries if this template targets hostile public deployments.

- [ ] Add tests for browser/security headers, request body limits, production config validation, session-token hashing, and reset-token URL scrubbing once implemented.

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
