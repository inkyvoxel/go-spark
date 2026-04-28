package generator

import (
	"fmt"
	"sort"
	"strings"
)

const (
	FeatureAll               = "all"
	FeatureCore              = "core"
	FeatureSQLite            = "sqlite"
	FeatureWeb               = "web"
	FeatureCSRF              = "csrf"
	FeatureAuth              = "auth"
	FeaturePasswordReset     = "password-reset"
	FeatureEmailOutbox       = "email-outbox"
	FeatureEmailVerification = "email-verification"
	FeatureEmailChange       = "email-change"
	FeatureWorker            = "worker"
	FeatureCleanup           = "cleanup"
)

// Component describes a generator feature bundle.
type Component struct {
	ID          string
	Name        string
	Description string
	DependsOn   []string
	Files       []string
	Templates   []string
	Migrations  []string
	Env         []string
	Docs        []string
	Tests       []string
}

type Manifest struct {
	Components []Component
}

func DefaultManifest() Manifest {
	return Manifest{Components: []Component{
		{
			ID:          FeatureCore,
			Name:        "Core",
			Description: "App shell, config basics, logging, docs, static assets, and Makefile.",
			Files: []string{
				".env.example", "CHANGELOG.md", "CONTRIBUTING.md", "LICENSE", "Makefile", "README.md", "SECURITY.md",
				"embedded_assets.go", "go.mod", "go.sum", "cmd/app/main.go",
				"internal/app/build.go", "internal/config/config.go", "internal/features",
				"internal/database/auth_store.go", "internal/database/cleanup_store.go", "internal/database/email_outbox_store.go", "internal/database/tx.go",
				"internal/db/generated",
				"internal/email/email.go", "internal/email/processor.go", "internal/email/smtp.go", "internal/email/templates",
				"internal/jobs/cleanup.go", "internal/jobs/email.go", "internal/jobs/runner.go",
				"internal/platform/sqlite/open.go",
				"internal/services/auth.go", "internal/services/email_verification_policy.go", "internal/services/password_hasher.go",
			},
			Env:  []string{"APP_ADDR", "APP_ENV", "LOG_FORMAT", "APP_BASE_URL"},
			Docs: []string{"docs/architecture.md", "docs/development.md", "docs/generated-features.md", "docs/production.md", "docs/todo.md"},
		},
		{
			ID:          FeatureSQLite,
			Name:        "SQLite",
			Description: "SQLite connection setup, migrations, sqlc configuration, and database packages.",
			DependsOn:   []string{FeatureCore},
			Files:       []string{"sqlc.yaml", "internal/platform/sqlite", "internal/database/tx.go", "internal/db/generated/db.go", "internal/db/generated/models.go"},
			Env:         []string{"DATABASE_PATH"},
		},
		{
			ID:          FeatureWeb,
			Name:        "Web",
			Description: "net/http server, route registration, rendering helpers, and health pages.",
			DependsOn:   []string{FeatureSQLite},
			Files:       []string{"internal/paths", "internal/server/assets.go", "internal/server/auth.go", "internal/server/auth_handlers.go", "internal/server/csrf.go", "internal/server/csrf_context.go", "internal/server/rate_limit.go", "internal/server/request_auth_state.go", "internal/server/request_id.go", "internal/server/request_id_context.go", "internal/server/server.go", "internal/server/template_constants.go", "static"},
			Templates:   []string{"templates/404.html", "templates/breadcrumb.html", "templates/home.html", "templates/layout.html"},
		},
		{
			ID:          FeatureCSRF,
			Name:        "CSRF",
			Description: "CSRF middleware and signing key configuration for form flows.",
			DependsOn:   []string{FeatureWeb},
			Files:       []string{"internal/server/csrf.go", "internal/server/csrf_context.go"},
			Env:         []string{"CSRF_SIGNING_KEY", "APP_COOKIE_SECURE"},
		},
		{
			ID:          FeatureEmailOutbox,
			Name:        "Email Outbox",
			Description: "Transactional email templates, log/SMTP senders, outbox store, and processor.",
			DependsOn:   []string{FeatureSQLite},
			Files:       []string{"internal/email/email.go", "internal/email/processor.go", "internal/email/smtp.go", "internal/database/email_outbox_store.go", "internal/db/queries/email.sql", "internal/db/generated/email.sql.go", "internal/email/templates/README.md"},
			Migrations:  []string{"migrations/00005_email_outbox_schema.sql"},
			Env:         []string{"EMAIL_FROM", "EMAIL_PROVIDER", "EMAIL_LOG_BODY", "SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_TLS"},
			Docs:        []string{"docs/email.md"},
		},
		{
			ID:          FeatureWorker,
			Name:        "Worker",
			Description: "Background jobs runner and worker runtime command.",
			DependsOn:   []string{FeatureCore},
			Files:       []string{"internal/jobs/email.go", "internal/jobs/runner.go"},
			Env:         []string{"APP_PROCESS"},
			Docs:        []string{"docs/jobs.md"},
		},
		{
			ID:          FeatureAuth,
			Name:        "Authentication",
			Description: "Users, sessions, registration, login, logout, account pages, rate limits, and password hashing.",
			DependsOn:   []string{FeatureSQLite, FeatureWeb, FeatureCSRF, FeatureEmailOutbox},
			Files:       []string{"internal/services/auth.go", "internal/services/email_verification_policy.go", "internal/services/password_hasher.go", "internal/database/auth_store.go", "internal/server/auth.go", "internal/server/auth_handlers.go", "internal/server/rate_limit.go", "internal/db/queries/auth.sql", "internal/db/generated/auth.sql.go", "internal/db/queries/password_reset.sql", "internal/db/generated/password_reset.sql.go", "internal/db/queries/email_change.sql", "internal/db/generated/email_change.sql.go"},
			Templates:   []string{"templates/account/account.html", "templates/account/change_password.html", "templates/account/login.html", "templates/account/register.html"},
			Migrations:  []string{"migrations/00001_auth_schema.sql", "migrations/00003_password_reset_schema.sql", "migrations/00004_email_change_schema.sql"},
			Env:         []string{"AUTH_PASSWORD_MIN_LENGTH", "AUTH_PASSWORD_PEPPER", "RATE_LIMIT_*"},
		},
		{
			ID:          FeaturePasswordReset,
			Name:        "Password Reset",
			Description: "Password reset tokens, email templates, routes, service methods, and store methods.",
			DependsOn:   []string{FeatureAuth, FeatureEmailOutbox},
			Files:       []string{"internal/db/queries/password_reset.sql", "internal/db/generated/password_reset.sql.go"},
			Templates:   []string{"internal/email/templates/password_reset.*", "templates/account/forgot_password.html", "templates/account/reset_password.html"},
			Migrations:  []string{"migrations/00003_password_reset_schema.sql"},
		},
		{
			ID:          FeatureEmailVerification,
			Name:        "Email Verification",
			Description: "Account verification and resend flows with durable email delivery.",
			DependsOn:   []string{FeatureAuth, FeatureEmailOutbox, FeatureWorker},
			Files:       []string{"internal/db/queries/email_verification.sql", "internal/db/generated/email_verification.sql.go"},
			Templates:   []string{"internal/email/templates/account_confirmation.*", "templates/account/confirm_email.html", "templates/account/resend_verification.html", "templates/account/verify_email.html"},
			Migrations:  []string{"migrations/00002_email_verification_schema.sql"},
			Env:         []string{"AUTH_EMAIL_VERIFICATION_REQUIRED"},
		},
		{
			ID:          FeatureEmailChange,
			Name:        "Email Change",
			Description: "Account email change confirmation and old-address notice flows.",
			DependsOn:   []string{FeatureAuth, FeatureEmailOutbox},
			Files:       []string{"internal/db/queries/email_change.sql", "internal/db/generated/email_change.sql.go"},
			Templates:   []string{"internal/email/templates/email_change.*", "internal/email/templates/email_change_notice.*", "templates/account/change_email.html", "templates/account/confirm_email_change.html"},
			Migrations:  []string{"migrations/00004_email_change_schema.sql"},
			Env:         []string{"AUTH_EMAIL_CHANGE_NOTICE_ENABLED"},
		},
		{
			ID:          FeatureCleanup,
			Name:        "Cleanup",
			Description: "Periodic pruning jobs for sessions, tokens, and outbox rows.",
			DependsOn:   []string{FeatureAuth, FeaturePasswordReset, FeatureEmailVerification, FeatureEmailChange, FeatureEmailOutbox, FeatureWorker},
			Files:       []string{"internal/jobs/cleanup.go", "internal/database/cleanup_store.go"},
			Env:         []string{"JOBS_CLEANUP_INTERVAL", "JOBS_CLEANUP_TOKEN_RETENTION", "JOBS_CLEANUP_SENT_EMAIL_RETENTION", "JOBS_CLEANUP_FAILED_EMAIL_RETENTION"},
		},
	}}
}

func (m Manifest) Resolve(selected []string) ([]Component, error) {
	byID := make(map[string]Component, len(m.Components))
	for _, component := range m.Components {
		if strings.TrimSpace(component.ID) == "" {
			return nil, fmt.Errorf("component ID is required")
		}
		if _, exists := byID[component.ID]; exists {
			return nil, fmt.Errorf("duplicate component %q", component.ID)
		}
		byID[component.ID] = component
	}

	ids := normalizeFeatureList(selected)
	if len(ids) == 0 {
		ids = []string{FeatureAll}
	}
	if contains(ids, FeatureAll) {
		ids = make([]string, 0, len(m.Components))
		for _, component := range m.Components {
			ids = append(ids, component.ID)
		}
	}

	seen := make(map[string]bool)
	var visit func(string) error
	visit = func(id string) error {
		component, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown component %q", id)
		}
		if seen[id] {
			return nil
		}
		for _, dep := range component.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		seen[id] = true
		return nil
	}

	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	resolved := make([]Component, 0, len(seen))
	for _, component := range m.Components {
		if seen[component.ID] {
			resolved = append(resolved, component)
		}
	}
	return resolved, nil
}

func normalizeFeatureList(selected []string) []string {
	var ids []string
	for _, value := range selected {
		for _, part := range strings.Split(value, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			if part == "" {
				continue
			}
			ids = append(ids, part)
		}
	}
	return ids
}

func ComponentIDs(components []Component) []string {
	ids := make([]string, 0, len(components))
	for _, component := range components {
		ids = append(ids, component.ID)
	}
	sort.Strings(ids)
	return ids
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
