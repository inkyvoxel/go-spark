# To-do

- Add user display name
- `is_admin` type flag + `requireAdmin` middleware — an is_admin boolean `NOT NULL DEFAULT false` column on the users table and a matching middleware
- Admin dashboard?
- Pagination helper? No use case in template, but this could be helpful
- Introduce Dockerfile? I personally like the idea of only using binary deploys over Docker, but can we make the app easy to be put inside a Docker container?
- Add request/response metrics and structured operational health checks for email outbox lag, job failures, rate-limit rejections, and auth error rates. This would be a private endpoint for local tools to use, e.g. prometheus
- Add a production deployment checklist that covers TLS termination, secure proxy headers, backup/restore drills, SQLite WAL files, secret rotation, and SMTP deliverability.
