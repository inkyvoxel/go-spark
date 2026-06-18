# Extending Go Spark

This is the quick onboarding map for adding your first app feature after you
fork the starter.

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
    rateLimit("archive-ip", s.rateLimitPolicies.ArchivePerIP, s.rateLimitKeyByIP()),
    rateLimit("archive", s.rateLimitPolicies.Archive, s.rateLimitKeyByIPAndUser()),
)
```

Add the policy fields and defaults in `internal/server/rate_limit.go`, the
config plumbing in `internal/config/config.go` and `internal/app/build.go`, and
the env vars in `.env` / `.env.example`.

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

Forms need CSRF:

```html
<input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
```

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
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX projects_user_id_idx ON projects(user_id);

-- +goose Down
DROP INDEX projects_user_id_idx;
DROP TABLE projects;
```

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
   `internal/email/templates`
2. add a message builder in `internal/email/email.go`
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
