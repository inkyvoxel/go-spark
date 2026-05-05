# To-do

- Add user display name
- `is_admin` type flag + `requireAdmin` middleware — an is_admin boolean `NOT NULL DEFAULT false` column on the users table and a matching middleware
- Admin dashboard?
- Pagination helper? No use case in template, but this could be helpful
- Dockerfile? I do like the idea of only using binary deploys
- Consider storing TOTP backup codes with a keyed hash/pepper and increasing entropy beyond the current 10 hex characters.
- Add short-lived, server-side state for pending TOTP login challenges so a copied pending cookie cannot be reused until expiry after the first successful challenge.
- Add request/response metrics and structured operational health checks for email outbox lag, job failures, rate-limit rejections, and auth error rates. This would be a private endpoint for local tools to use, e.g. prometheus
- Add a production deployment checklist that covers TLS termination, secure proxy headers, backup/restore drills, SQLite WAL files, secret rotation, and SMTP deliverability.
- Review using TOTP for changing password, changing email
- Review backup codes and what they can be used for. Do we need a way to regenerate new backup codes?
