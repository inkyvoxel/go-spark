# To-do

- Add user display name
- `is_admin` type flag + `requireAdmin` middleware — an is_admin boolean `NOT NULL DEFAULT false` column on the users table and a matching middleware
- Admin dashboard?
- Pagination helper? Can use for 'sessions' in `/account`, and admin dashboard 
- Add request/response `/metrics` (behind basic auth, IP whitelist, or an internal network-only path) and structured operational health checks for email outbox lag, job failures, rate-limit rejections, and auth error rates. This would be a private endpoint for local tools to use, e.g. use OpenTelemetry (OTel), with a `.env` variable to configure output type, and default to prometheus. Prometheus could be set up using the default production dockerfile/docker composer.
- Litestream backups