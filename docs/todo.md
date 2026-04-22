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

- [ ] Add a production-grade rate limiter option for multi-instance deployments.
  - Evidence: the limiter is in-memory and keyed from `RemoteAddr`; README correctly documents that it is per instance and does not trust forwarded headers.
  - Risk: limits are bypassed across multiple app instances or after restarts, and reverse proxies can collapse many users into one `RemoteAddr` unless configured carefully.
  - Recommendation: keep the in-memory limiter for local/dev, but define a `rateLimitStore` adapter for Redis/Postgres/SQLite or document single-instance assumptions prominently. Add explicit trusted proxy configuration before honoring `X-Forwarded-For`.

- [ ] Add database cleanup jobs for expired sessions, consumed/expired reset tokens, consumed/expired verification tokens, and old sent/failed email rows.
  - Evidence: queries ignore expired sessions/tokens, but migrations do not include cleanup behavior and the app does not schedule pruning.
  - Risk: sensitive metadata and operational data accumulate indefinitely, increasing breach impact and storage growth.
  - Recommendation: add periodic cleanup with conservative retention windows and tests.

- [ ] Improve email outbox crash recovery.
  - Evidence: `ClaimPendingEmails` moves rows to `sending`; if the process crashes after claiming but before marking sent/failed, those rows can remain stuck.
  - Risk: password reset and verification emails may silently stop for affected rows.
  - Recommendation: add `claimed_at`/lease fields, retry stale `sending` rows, and cap/trim `last_error` to avoid unbounded database growth.

- [x] Add dependency and supply-chain security checks to CI.
  - Evidence: `govulncheck` is now available via `make vulncheck`, but CI does not run it yet. `go list -m all` shows a large transitive dependency graph because tool dependencies such as sqlc/goose bring many packages.
  - Risk: security advisories may be missed, especially in tool dependencies and browser assets.
  - Recommendation: run `make vulncheck` in CI, run `go list -m -u all` periodically, pin and review vendored browser assets, and consider Dependabot/Renovate.
  - Progress: CI now runs `make vulncheck` on every push/PR in `.github/workflows/test.yml`. Added weekly/manual `.github/workflows/dependency-checks.yml` to run `go list -m -u all`. Dependabot is configured for both Go modules and GitHub Actions in `.github/dependabot.yml`.

### Low priority / defense in depth

- [ ] Consider `SameSite=Strict` for session cookies if product flows allow it.
  - Evidence: session and CSRF cookies use `SameSite=Lax`.
  - Risk: Lax is a reasonable default and permits normal cross-site top-level GET navigation, but Strict further reduces ambient-cookie exposure.
  - Recommendation: keep Lax if email-link and external navigation UX matters, or make SameSite configurable with Strict recommended for high-risk apps.

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
