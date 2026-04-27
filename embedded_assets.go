package appassets

import "embed"

// FS contains templates, static assets, and migrations bundled into the runtime
// application binary.
//
//go:embed all:templates all:static all:migrations
var FS embed.FS

// StarterFS contains the source files copied by the go-spark project generator.
//
// Keep this list explicit so generator-only packages are not copied into
// generated applications.
//
//go:embed .env.example CHANGELOG.md CONTRIBUTING.md LICENSE Makefile README.md SECURITY.md docs embedded_assets.go go.mod go.sum sqlc.yaml cmd/app internal/app internal/config internal/database internal/db internal/email internal/jobs internal/paths internal/platform internal/server internal/services migrations static templates
var StarterFS embed.FS
