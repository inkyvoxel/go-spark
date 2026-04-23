# TODO

* embed templates and static assets into the binary for simpler deployment
* introduce database-backed rate limiting only if the in-memory limiter stops being sufficient for the starter's deploymentmodel
* split auth into a reusable module only if a real second bounded context appears
* add richer observability hooks only after there is a concrete operational need
