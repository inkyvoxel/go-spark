# To-do

- Add user display name
- `is_admin` type flag + `requireAdmin` middleware — an is_admin boolean `NOT NULL DEFAULT false` column on the users table and a matching middleware
- Admin dashboard?
- Pagination helper? Can use for 'sessions' in `/account`, and admin dashboard 
- Add request/response `/metrics` (behind basic auth, IP whitelist, or an internal network-only path) and structured operational health checks for email outbox lag, job failures, rate-limit rejections, and auth error rates. This would be a private endpoint for local tools to use, e.g. use OpenTelemetry (OTel), with a `.env` variable to configure output type, and default to prometheus. Prometheus could be set up using the default production dockerfile/docker composer.
- Litestream backups

## Auth hardening (from auth audit)

- Add a per-account login limiter to slow distributed brute force. The per-email login limiter in `internal/server/rate_limit.go` keys on `ip|email` (`rateLimitKeyByIPAndEmail`), so each IP gets a fresh allowance against the same account — many IPs can still guess one target account's password. Layer a limiter keyed on email alone (no IP) alongside the existing per-IP and per-`ip|email` ones (mind that an attacker could weaponise this to lock a victim out; pair with account lockout/backoff thinking, and keep the threshold high).
