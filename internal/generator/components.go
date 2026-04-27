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
//
// The source files are still copied from the full starter template in this
// first refactor. These fields make the dependency and ownership model explicit
// so later phases can move files into per-component bundles without changing
// the CLI contract.
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
			Files:       []string{"cmd/app", "internal/config", "embedded_assets.go", "Makefile", "README.md"},
			Templates:   []string{"templates/layout.html", "templates/home.html", "templates/404.html"},
			Env:         []string{"APP_ADDR", "APP_ENV", "LOG_FORMAT", "APP_BASE_URL"},
			Docs:        []string{"docs/development.md", "docs/architecture.md"},
		},
		{
			ID:          FeatureSQLite,
			Name:        "SQLite",
			Description: "SQLite connection setup, migrations, sqlc configuration, and database packages.",
			DependsOn:   []string{FeatureCore},
			Files:       []string{"internal/platform/sqlite", "internal/database", "internal/db", "sqlc.yaml"},
			Migrations:  []string{"migrations"},
			Env:         []string{"DATABASE_PATH"},
		},
		{
			ID:          FeatureWeb,
			Name:        "Web",
			Description: "net/http server, route registration, rendering helpers, and health pages.",
			DependsOn:   []string{FeatureCore},
			Files:       []string{"internal/server", "internal/paths"},
			Templates:   []string{"templates"},
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
			Files:       []string{"internal/email", "internal/database/email_outbox_store.go", "internal/db/queries/email.sql"},
			Migrations:  []string{"email_outbox"},
			Env:         []string{"EMAIL_FROM", "EMAIL_PROVIDER", "EMAIL_LOG_BODY", "SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_TLS"},
			Docs:        []string{"docs/email.md"},
		},
		{
			ID:          FeatureWorker,
			Name:        "Worker",
			Description: "Background jobs runner and worker runtime command.",
			DependsOn:   []string{FeatureCore},
			Files:       []string{"internal/jobs", "cmd/app"},
			Env:         []string{"APP_PROCESS"},
			Docs:        []string{"docs/jobs.md"},
		},
		{
			ID:          FeatureAuth,
			Name:        "Authentication",
			Description: "Users, sessions, registration, login, logout, account pages, rate limits, and password hashing.",
			DependsOn:   []string{FeatureSQLite, FeatureWeb, FeatureCSRF},
			Files:       []string{"internal/services/auth.go", "internal/services/password_hasher.go", "internal/database/auth_store.go", "internal/server/auth.go", "internal/server/auth_handlers.go"},
			Templates:   []string{"templates/account"},
			Migrations:  []string{"users", "sessions"},
			Env:         []string{"AUTH_PASSWORD_MIN_LENGTH", "AUTH_PASSWORD_PEPPER", "RATE_LIMIT_*"},
		},
		{
			ID:          FeaturePasswordReset,
			Name:        "Password Reset",
			Description: "Password reset tokens, email templates, routes, service methods, and store methods.",
			DependsOn:   []string{FeatureAuth, FeatureEmailOutbox},
			Files:       []string{"internal/services/auth.go", "internal/database/auth_store.go", "internal/email/templates/password_reset.*"},
			Templates:   []string{"templates/account/forgot_password.html", "templates/account/reset_password.html"},
			Migrations:  []string{"password_reset_tokens"},
		},
		{
			ID:          FeatureEmailVerification,
			Name:        "Email Verification",
			Description: "Account verification and resend flows with durable email delivery.",
			DependsOn:   []string{FeatureAuth, FeatureEmailOutbox, FeatureWorker},
			Files:       []string{"internal/services/email_verification_policy.go", "internal/email/templates/account_confirmation.*"},
			Templates:   []string{"templates/account/verify_email.html", "templates/account/confirm_email.html", "templates/account/resend_verification.html"},
			Migrations:  []string{"email_verification_tokens"},
			Env:         []string{"AUTH_EMAIL_VERIFICATION_REQUIRED"},
		},
		{
			ID:          FeatureEmailChange,
			Name:        "Email Change",
			Description: "Account email change confirmation and old-address notice flows.",
			DependsOn:   []string{FeatureAuth, FeatureEmailOutbox},
			Files:       []string{"internal/email/templates/email_change.*", "internal/email/templates/email_change_notice.*"},
			Templates:   []string{"templates/account/change_email.html", "templates/account/confirm_email_change.html"},
			Migrations:  []string{"email_change_tokens"},
			Env:         []string{"AUTH_EMAIL_CHANGE_NOTICE_ENABLED"},
		},
		{
			ID:          FeatureCleanup,
			Name:        "Cleanup",
			Description: "Periodic pruning jobs for sessions, tokens, and outbox rows.",
			DependsOn:   []string{FeatureSQLite, FeatureWorker},
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
