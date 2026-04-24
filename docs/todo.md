# TODO

## Starter-template priorities:

* add sensible cache headers for `/static/` without introducing an asset pipeline

## Auth boundaries:

* tighten auth package boundaries and naming without forcing a large package reorg
* keep HTTP concerns in handlers/middleware, business rules in services, and persistence in stores
* avoid leaking storage details outside auth-related code paths
* keep auth flows observable with structured logs and request IDs
* only extract a reusable auth module if a second real consumer appears
