# TODO

## Starter-template priorities:

* add simple `/healthz` and `/readyz` endpoints. No need to add a link anywhere in the app, but document.
* add sensible cache headers for `/static/` without introducing an asset pipeline

## Auth boundaries:

* tighten auth package boundaries and naming without forcing a large package reorg
* keep HTTP concerns in handlers/middleware, business rules in services, and persistence in stores
* avoid leaking storage details outside auth-related code paths
* keep auth flows observable with structured logs and request IDs
* only extract a reusable auth module if a second real consumer appears

## Improve Logs

Add minimal structured HTTP access logs via slog:

- Add HTTP middleware that logs one record per request after the response completes
- Include stable, low-cardinality fields:
  - request_id
  - method
  - route pattern, not raw URL path where possible
  - status
  - duration_ms
  - response_bytes
  - remote_ip
  - user_agent optional
  - authenticated true/false
- Log at:
  - INFO for normal requests
  - WARN for 4xx/5xx if useful
  - ERROR only for unexpected server-side failures
- Avoid logging sensitive data:
  - cookies
  - session IDs
  - CSRF tokens
  - password reset tokens
  - email verification tokens
  - raw query strings
  - request/response bodies
- Make logging configurable:
  - LOG_LEVEL=info
  - LOG_FORMAT=json|text
- Ensure request_id is attached to the request context so deeper app logs can include it

Tighten worker lifecycle and email outbox logging for easier debugging.