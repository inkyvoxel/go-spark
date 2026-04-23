package appassets

import "embed"

// FS contains templates, static assets, and migrations bundled into the binary.
//
//go:embed all:templates all:static all:migrations
var FS embed.FS
