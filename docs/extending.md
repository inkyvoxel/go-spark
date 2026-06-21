# Extending Go Spark

This is the quick onboarding map for adding your first app feature after you
fork the starter.

> **A working version of this walkthrough ships in the repo** as the example
> `projects` feature (create / list / delete). Read it alongside this guide —
> every layer below has a real counterpart. When you no longer need it, see
> [Removing the example feature](#removing-the-example-feature) below.

Most features follow this path:

```text
paths -> routes -> handlers -> services -> stores -> SQL
                  |
                  +-> templates
                  +-> optional email
                  +-> optional worker job
```

The short version:

* routes and handlers live in `internal/server`
* business rules live in `internal/services`
* database access lives in `internal/database`
* SQL lives in `internal/db/queries`
* migrations live in `migrations`
* URL constants live in `internal/paths`
* rate-limit policies live in `internal/ratelimit`
* HTML templates live in `templates`

## Common Feature Checklist

For a user-owned feature like "projects", you will usually add or update:

```text
internal/paths/paths.go              URL constants and template routes
internal/server/server.go            route registration
internal/server/project_handlers.go  HTTP handlers
templates/projects/*.html            pages and forms
internal/services/projects.go        domain types and business rules
internal/database/project_store.go   SQLite store
internal/db/queries/projects.sql     sqlc queries
migrations/00002_projects_schema.sql schema changes
internal/app/build.go                service/store wiring
internal/ratelimit/ratelimit.go      rate-limit policy (only for limited routes)
```

Not every feature needs every layer. A static public page only needs a path,
route, handler, and template. A cleanup job may only need a job, store, SQL,
and wiring.

## 1. Add Paths

Add public URL constants in `internal/paths/paths.go`:

```go
const (
	Projects       = "/projects"
	NewProject     = Projects + "/new"
	ProjectArchive = Projects + "/archive"
)
```

If templates need the route, add an entry to the `TemplateRoutes` map in the
same file (e.g. `"NewProject": NewProject`). Templates should use
`.Routes.NewProject`, not hard-coded URLs.

## 2. Register Routes

Register feature routes in `internal/server/server.go`, before the catch-all
404 route:

```go
s.registerAuthRoutes(dynamic)
s.registerProjectRoutes(dynamic)
dynamic.HandleFunc(route(http.MethodGet, "/{$}"), s.home)
dynamic.HandleFunc(route(http.MethodGet, "/{path...}"), s.notFoundPage)
```

Choose the wrapper based on who can access the route:

```go
dynamic.HandleFunc(route(http.MethodGet, paths.About), s.about)
dynamic.Handle(route(http.MethodGet, paths.Projects), s.requireVerifiedAuth(http.HandlerFunc(s.projectsIndex)))
```

Use `s.postOnly` for POST routes that have no matching GET route:

```go
s.postOnly(dynamic, paths.ProjectArchive, s.requireVerifiedAuth(http.HandlerFunc(s.archiveProject)))
```

Use rate limiting for public or sensitive POST routes. Wrap the handler with
`s.withRateLimits`, passing one or more limiters evaluated in order. Put a
coarse per-IP ceiling first so an abusive IP is rejected before it consumes a
narrower per-email or per-user bucket:

```go
s.withRateLimits(http.HandlerFunc(s.archiveProject),
    rateLimit("archive-ip", s.rateLimitPolicies[ratelimit.ArchivePerIP], s.rateLimitKeyByIP()),
    rateLimit("archive", s.rateLimitPolicies[ratelimit.Archive], s.rateLimitKeyByIPAndUser()),
)
```

A policy lives in one place — `internal/ratelimit/ratelimit.go`. Add a `Name`
constant, append it to `Names`, and give it a `Defaults` entry:

```go
const Archive Name = "archive"           // + ArchivePerIP, etc.
// add to Names, then:
Archive: {MaxRequests: 5, Window: 15 * time.Minute},
```

That is all the wiring needed. The config loader iterates `Names`, so the env
overrides `RATE_LIMIT_ARCHIVE_MAX_REQUESTS` and `RATE_LIMIT_ARCHIVE_WINDOW`
(uppercased name, see `EnvPrefix`) work automatically — no changes to
`config.go` or `build.go`. Document the new vars in `.env.example`.

### Why there is no email-only login limiter

Login is guarded by a per-IP ceiling and a per-`ip|email` limiter, but
deliberately *not* by a limiter keyed on email alone. An email-only bucket is
shared across every IP, so it would slow a distributed (many-IP) brute force —
but it is also trivially weaponisable: an attacker can flood that one bucket to
lock the real account holder out. There is no way around this with a plain
counter. A rate limiter only helps by short-circuiting the request *before* the
password is checked; once the shared bucket is exhausted, the legitimate user's
correct password is rejected too.

The realistic single-source attack is already covered by the per-IP and
`ip|email` limiters. If your threat model includes genuine distributed brute
force, reach for **step-up auth** instead of a hard block — e.g. require a
CAPTCHA or an emailed challenge after N failed attempts on an account. The real
user can pass it; a bot army mostly cannot, and nobody gets locked out.

## 3. Add Templates

Add page templates under `templates/`:

```html
{{ define "content" }}
<article>
  <h1>Projects</h1>
  <p><a href="{{ .Routes.NewProject }}">New project</a></p>
</article>
{{ end }}
```

Register each page in:

* `internal/server/template_constants.go`
* the `pages` map in `parseTemplates`

Forms need no CSRF token. State-changing requests are protected by origin
checks (`http.CrossOriginProtection` plus `SameSite=Lax` cookies), so a normal
same-origin form `POST` is covered automatically — see
[architecture.md](architecture.md#csrf-protection).

If a page needs feature-specific data, add a small field to `templateData`,
such as `Projects []services.Project`.

## 4. Add Database Changes

If the feature stores data, add a goose migration:

```sql
-- +goose Up
CREATE TABLE projects (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX projects_user_id_idx ON projects(user_id);

-- +goose Down
DROP INDEX projects_user_id_idx;
DROP TABLE projects;
```

**Always add `ON DELETE CASCADE` to a user-owned table's `user_id` foreign
key.** Account deletion (`AuthStore.DeleteAccount`) deletes only the `users`
row and relies on the database to cascade the delete to every child table. If
you omit `ON DELETE CASCADE`, the `users` delete fails with a foreign-key
constraint error the moment a user who owns rows in your table tries to delete
their account. `foreign_keys` is enabled on every connection (see
`internal/platform/sqlite/open.go`), so the cascade is enforced — there is no
per-table cleanup code to remember to update.

Add explicit SQL in `internal/db/queries/projects.sql`:

```sql
-- name: CreateProject :one
INSERT INTO projects (user_id, name)
VALUES (?, ?)
RETURNING id, user_id, name, created_at;

-- name: ListProjectsByUserID :many
SELECT id, user_id, name, created_at
FROM projects
WHERE user_id = ?
ORDER BY created_at DESC, id DESC;
```

Then run:

```sh
make sqlc
make migrate-up
```

Do not edit `internal/db/generated/` by hand.

## 5. Add Store and Service

Stores wrap generated SQL and map rows into service-owned types:

```go
func NewProjectStore(conn *sql.DB) *ProjectStore {
	return &ProjectStore{queries: db.New(conn)}
}
```

Services own validation and business rules:

```go
func (s *ProjectService) Create(ctx context.Context, userID int64, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, ErrProjectNameRequired
	}
	return s.store.Create(ctx, userID, name)
}
```

Handlers should call services, not generated database queries.

## 6. Wire the Feature

Create stores and services in `internal/app/build.go`, then pass services into
`server.New` through `server.Options`.

In the server package, define the smallest interface the handlers need:

```go
type projectService interface {
	Create(context.Context, int64, string) (services.Project, error)
	ListByUserID(context.Context, int64) ([]services.Project, error)
}
```

This keeps handlers away from database details.

## 7. Handler Pattern

GET handlers load data and render a template.

POST handlers usually:

1. call `r.ParseForm()`
2. read the current user with `currentUser(r.Context())`, if authenticated
3. call a service method
4. translate known service errors into form errors or status codes
5. set a flash message and redirect after success

Use `s.renderStatus(..., http.StatusUnprocessableEntity, ...)` for invalid
form submissions and `http.Redirect(..., http.StatusSeeOther)` after a
successful POST.

## 8. Emails

For a new transactional email:

1. add `name.subject.txt`, `name.text.txt`, and `name.html.tmpl` in
   `internal/email/templates` — the three files are parsed at startup and keyed
   by their shared `name` prefix, with no registration step
2. add a message builder in `internal/email/email.go` that calls
   `renderEmailTemplates("name", data)` with that same prefix
3. let the service decide when to send it
4. enqueue the message into `email_outbox` through a store

Request handlers should not call SMTP directly. The worker sends queued email.

## 9. Worker Jobs

For recurring work, add a periodic job in `internal/jobs` and register it in
`internal/app/build.go`.

For retryable per-item work, use a durable table-backed processor. The email
outbox is the example to copy.

## 10. Tests and Checks

Add focused tests where the behavior lives:

* server tests for routes, auth, redirects, and form errors
* service tests for validation and business rules
* store tests for SQL and transactions
* email tests for rendered messages and links
* job tests for background behavior

## Removing the Example Feature

The `projects` feature is a demo. When you are ready to replace it with your own
work, remove it cleanly — it is isolated in `project*`-named files plus a few
tagged wiring lines.

Delete these files:

```sh
rm internal/server/project_handlers.go internal/server/project_handlers_test.go
rm internal/services/projects.go internal/services/projects_test.go
rm internal/database/project_store.go internal/database/project_store_test.go
rm internal/db/queries/projects.sql
rm migrations/00002_projects_schema.sql
rm -r templates/projects
```

Then remove the small, comment-tagged wiring in each of these files (search for
`projects example` / `example feature`):

* `internal/paths/paths.go` — the `Projects` / `ProjectsDelete` constants and their `TemplateRoutes` entries
* `internal/server/template_constants.go` — the `templateProjects` constant
* `internal/server/server.go` — the `parseTemplates` entry, the `Server` and `Options` `projects` fields, the `New` assignment, and the `registerProjectRoutes` call
* `internal/server/auth_handlers.go` — the `Projects` field on `templateData`
* `internal/app/build.go` — the project store/service wiring
* `templates/layout.html` — the "Projects" nav link

Finally, regenerate the SQL layer and verify:

```sh
make sqlc     # drops internal/db/generated/projects.sql.go and the Project model
make check
```

If you have **not** applied the example migration yet, deleting
`migrations/00002_projects_schema.sql` is enough. If you **already** ran
`make migrate-up`, also drop the table from your local database (e.g.
`make migrate-down` once, before deleting the file, or `DROP TABLE projects;`)
so your schema matches the migrations.

## Before finishing

Before committing changes:

```sh
make check
```

If you changed migrations:

```sh
make migrate-up
make migrate-status
```
