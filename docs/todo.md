# To-do

## Auth hardening (from auth audit)

- Add a per-account login limiter to slow distributed brute force. The per-email login limiter in `internal/server/rate_limit.go` keys on `ip|email` (`rateLimitKeyByIPAndEmail`), so each IP gets a fresh allowance against the same account — many IPs can still guess one target account's password. Layer a limiter keyed on email alone (no IP) alongside the existing per-IP and per-`ip|email` ones (mind that an attacker could weaponise this to lock a victim out; pair with account lockout/backoff thinking, and keep the threshold high).
