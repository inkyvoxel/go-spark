package generator

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/mail"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	appassets "github.com/inkyvoxel/go-spark"
)

const (
	defaultProjectName      = "Go Spark"
	defaultModulePath       = "github.com/example/my-app"
	defaultDatabasePath     = "./data/app.db"
	defaultEmailFrom        = "Go Spark <hello@example.com>"
	sourceModulePath        = "github.com/inkyvoxel/go-spark"
	sourceBinaryName        = "go-spark"
	generatedTodo           = "# TODO\n\nNo open TODOs.\n"
	generatorImplementation = "Copied component source bundles. Some generated feature sets still need the follow-up bootstrap refactor before they compile as standalone runtime apps.\n"
)

type ProjectOptions struct {
	TargetPath   string
	ProjectName  string
	ModulePath   string
	DatabasePath string
	EmailFrom    string
	Features     []string
	Yes          bool
	Force        bool
}

type Generator struct {
	Manifest Manifest
	Stdin    io.Reader
	Stdout   io.Writer
}

type Result struct {
	TargetPath string
	Components []Component
	Files      []string
}

func New() Generator {
	return Generator{
		Manifest: DefaultManifest(),
		Stdin:    os.Stdin,
		Stdout:   os.Stdout,
	}
}

func (g Generator) NewProject(opts ProjectOptions) (Result, error) {
	opts.TargetPath = strings.TrimSpace(opts.TargetPath)
	if opts.TargetPath == "" {
		return Result{}, fmt.Errorf("target path is required")
	}

	opts, err := g.resolveOptions(opts)
	if err != nil {
		return Result{}, err
	}

	components, err := g.Manifest.Resolve(opts.Features)
	if err != nil {
		return Result{}, err
	}

	if err := validateProjectOptions(opts); err != nil {
		return Result{}, err
	}

	targetPath, err := filepath.Abs(opts.TargetPath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve target path: %w", err)
	}
	if err := ensureWritableTarget(targetPath, opts.Force); err != nil {
		return Result{}, err
	}

	files, err := copyComponents(targetPath, opts, components)
	if err != nil {
		return Result{}, err
	}

	if g.Stdout != nil {
		fmt.Fprintf(g.Stdout, "Created %s at %s.\n", opts.ProjectName, targetPath)
		fmt.Fprintf(g.Stdout, "Selected components: %s\n", strings.Join(ComponentIDs(components), ", "))
		fmt.Fprint(g.Stdout, generatorImplementation)
	}

	return Result{
		TargetPath: targetPath,
		Components: components,
		Files:      files,
	}, nil
}

func (g Generator) resolveOptions(opts ProjectOptions) (ProjectOptions, error) {
	if opts.Yes {
		opts.ProjectName = defaultString(opts.ProjectName, defaultProjectName)
		opts.ModulePath = defaultString(opts.ModulePath, defaultModulePath)
		opts.DatabasePath = defaultString(opts.DatabasePath, defaultDatabasePath)
		opts.EmailFrom = defaultString(opts.EmailFrom, defaultEmailFrom)
		if len(opts.Features) == 0 {
			opts.Features = []string{FeatureAll}
		}
		return opts, nil
	}

	reader := bufio.NewReader(g.Stdin)
	var err error
	opts.ProjectName, err = promptString(reader, g.Stdout, "Project name", opts.ProjectName, defaultProjectName)
	if err != nil {
		return ProjectOptions{}, err
	}
	opts.ModulePath, err = promptString(reader, g.Stdout, "Go module path", opts.ModulePath, defaultModulePath)
	if err != nil {
		return ProjectOptions{}, err
	}
	opts.DatabasePath, err = promptString(reader, g.Stdout, "Default database path", opts.DatabasePath, defaultDatabasePath)
	if err != nil {
		return ProjectOptions{}, err
	}
	opts.EmailFrom, err = promptString(reader, g.Stdout, "Default email sender", opts.EmailFrom, defaultEmailFrom)
	if err != nil {
		return ProjectOptions{}, err
	}
	features := strings.Join(opts.Features, ",")
	features, err = promptString(reader, g.Stdout, "Features", features, FeatureAll)
	if err != nil {
		return ProjectOptions{}, err
	}
	opts.Features = []string{features}
	return opts, nil
}

func validateProjectOptions(opts ProjectOptions) error {
	if strings.TrimSpace(opts.ProjectName) == "" {
		return fmt.Errorf("project name is required")
	}
	if strings.TrimSpace(opts.ModulePath) == "" {
		return fmt.Errorf("module path is required")
	}
	if !strings.Contains(opts.ModulePath, "/") {
		return fmt.Errorf("module path must look like a Go module path")
	}
	if strings.TrimSpace(opts.DatabasePath) == "" {
		return fmt.Errorf("database path is required")
	}
	if _, err := mail.ParseAddress(opts.EmailFrom); err != nil {
		return fmt.Errorf("email-from must be a valid email address: %w", err)
	}
	return nil
}

func ensureWritableTarget(targetPath string, force bool) error {
	entries, err := os.ReadDir(targetPath)
	if err == nil {
		if len(entries) > 0 && !force {
			return fmt.Errorf("target path %s is not empty; pass -force to write into it", targetPath)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return os.MkdirAll(targetPath, 0o755)
	}
	return fmt.Errorf("read target path: %w", err)
}

func copyComponents(targetPath string, opts ProjectOptions, components []Component) ([]string, error) {
	sourceFiles, err := componentSourceFiles(components)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(sourceFiles))
	for _, name := range sourceFiles {
		body, err := fs.ReadFile(appassets.StarterFS, name)
		if err != nil {
			return nil, fmt.Errorf("read template file %s: %w", name, err)
		}
		body, err = renderStarterFile(name, body, opts)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(targetPath, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("create parent for %s: %w", name, err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
		files = append(files, name)
	}
	return files, nil
}

func componentSourceFiles(components []Component) ([]string, error) {
	selected := make(map[string]bool)
	for _, component := range components {
		for _, source := range componentSources(component) {
			if err := addSourceFiles(selected, source); err != nil {
				return nil, fmt.Errorf("component %s source %s: %w", component.ID, source, err)
			}
		}
	}

	files := make([]string, 0, len(selected))
	for file := range selected {
		files = append(files, file)
	}
	sort.Strings(files)
	return files, nil
}

func componentSources(component Component) []string {
	total := len(component.Files) + len(component.Templates) + len(component.Migrations) + len(component.Docs) + len(component.Tests)
	sources := make([]string, 0, total)
	sources = append(sources, component.Files...)
	sources = append(sources, component.Templates...)
	sources = append(sources, component.Migrations...)
	sources = append(sources, component.Docs...)
	sources = append(sources, component.Tests...)
	return sources
}

func addSourceFiles(selected map[string]bool, source string) error {
	source = path.Clean(strings.TrimSpace(filepath.ToSlash(source)))
	if source == "." || source == "" {
		return fmt.Errorf("source path is required")
	}
	if strings.ContainsAny(source, "*?[") {
		return addGlobbedSourceFiles(selected, source)
	}

	info, err := fs.Stat(appassets.StarterFS, source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		selected[source] = true
		return nil
	}

	return fs.WalkDir(appassets.StarterFS, source, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		selected[name] = true
		return nil
	})
}

func addGlobbedSourceFiles(selected map[string]bool, pattern string) error {
	matched := false
	err := fs.WalkDir(appassets.StarterFS, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		ok, err := path.Match(pattern, name)
		if err != nil {
			return err
		}
		if ok {
			matched = true
			selected[name] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !matched {
		return fmt.Errorf("no files matched")
	}
	return nil
}

func renderStarterFile(name string, body []byte, opts ProjectOptions) ([]byte, error) {
	if name == "docs/todo.md" {
		return []byte(generatedTodo), nil
	}
	if name == "docs/development.md" {
		return []byte(generatedDevelopmentDoc()), nil
	}
	if name == "docs/architecture.md" {
		return []byte(generatedArchitectureDoc(opts)), nil
	}
	if name == "README.md" {
		return []byte(generatedREADME(opts)), nil
	}
	if name == "go.mod" {
		return []byte(replaceModuleDeclaration(string(body), opts.ModulePath)), nil
	}
	if name == "Makefile" {
		return []byte(renderGeneratedMakefile(string(body))), nil
	}
	if name == "templates/home.html" && opts.ProjectName != defaultProjectName {
		return []byte(homeTemplate(opts.ProjectName)), nil
	}

	content := string(body)
	content = strings.ReplaceAll(content, sourceModulePath, opts.ModulePath)
	content = strings.ReplaceAll(content, strconv.Quote(defaultEmailFrom), strconv.Quote(opts.EmailFrom))
	content = strings.ReplaceAll(content, defaultEmailFrom, opts.EmailFrom)
	content = strings.ReplaceAll(content, defaultProjectName, opts.ProjectName)
	content = strings.ReplaceAll(content, defaultDatabasePath, opts.DatabasePath)
	content = strings.ReplaceAll(content, sourceBinaryName, path.Base(opts.ModulePath))
	return []byte(content), nil
}

func renderGeneratedMakefile(content string) string {
	content = strings.ReplaceAll(content, " build-generator", "")
	content = strings.ReplaceAll(content, "GENERATOR_BIN ?= ./bin/go-spark\n", "")
	content = strings.ReplaceAll(content, "\nbuild-generator:\n\tmkdir -p $(dir $(GENERATOR_BIN))\n\tgo build -trimpath -o $(GENERATOR_BIN) ./cmd/go-spark\n", "\n")
	return content
}

func generatedREADME(opts ProjectOptions) string {
	return fmt.Sprintf(`# %s

A SQLite-first server-rendered Go web application generated by Go Spark.

## Quick Start

`+"```sh"+`
cp .env.example .env
make migrate-up
make start
`+"```"+`

Open `+"`http://localhost:8080`"+`.

## Runtime Commands

`+"```sh"+`
./%s all
./%s serve
./%s worker
./%s migrate status
`+"```"+`

## Development

`+"```sh"+`
make start
make start-web
make start-worker
make test
make check
make sqlc
`+"```"+`

This app is plain Go code. The generator was only used to create the initial
project files.
`, opts.ProjectName, path.Base(opts.ModulePath), path.Base(opts.ModulePath), path.Base(opts.ModulePath), path.Base(opts.ModulePath))
}

func generatedDevelopmentDoc() string {
	return strings.Join([]string{
		"# Development",
		"",
		"## Requirements",
		"",
		"* Go 1.26 or newer",
		"",
		"## First Run",
		"",
		"```sh",
		"cp .env.example .env",
		"make migrate-up",
		"make start",
		"```",
		"",
		"The default SQLite database path is configured in `.env.example`.",
		"",
		"## Common Commands",
		"",
		"```sh",
		"make start",
		"make start-web",
		"make start-worker",
		"make build-prod",
		"make migrate-status",
		"make migrate-up",
		"make migrate-down",
		"make test",
		"make check",
		"make sqlc",
		"```",
		"",
		"The app CLI uses explicit runtime commands: `all`, `serve`, `worker`, and `migrate`.",
		"",
	}, "\n")
}

func generatedArchitectureDoc(opts ProjectOptions) string {
	return fmt.Sprintf(`# Architecture

%s is a server-rendered Go web application.

## Package Boundaries

`+"```text"+`
/cmd/app            application entrypoint
/internal/app       application bootstrap and runtime assembly
/internal/config    environment config
/internal/database  SQLite-backed domain stores
/internal/db        SQL queries and generated sqlc package
/internal/email     email messages, senders, and outbox processor
/internal/jobs      jobs runner and periodic background jobs
/internal/platform  engine-specific platform code such as SQLite setup
/internal/paths     canonical public URL paths
/internal/server    HTTP handlers, middleware, templates
/internal/services  business logic
/migrations         goose SQL migrations
/templates          server-rendered HTML templates
/static             CSS and static assets
`+"```"+`

Handlers own HTTP concerns, services own business logic, and stores own
persistence concerns.
`, opts.ProjectName)
}

func replaceModuleDeclaration(content, modulePath string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "module ") {
			lines[i] = "module " + modulePath
			return strings.Join(lines, "\n")
		}
	}
	return content
}

func homeTemplate(projectName string) string {
	const tmpl = `{{ define "content" }}
<section>
  <h1>Welcome to {{ .ProjectName }}.</h1>
  <p>
    Your Go web app is ready. Start by replacing this page with the first workflow your users need.
  </p>
</section>
	{{ end }}
`
	var out bytes.Buffer
	parsed := template.Must(template.New("home").Parse(tmpl))
	err := parsed.ExecuteTemplate(&out, "content", map[string]string{
		"ProjectName": projectName,
	})
	if err != nil {
		panic(err)
	}
	return out.String()
}

func promptString(reader *bufio.Reader, stdout io.Writer, label, provided, fallback string) (string, error) {
	if strings.TrimSpace(provided) != "" {
		return strings.TrimSpace(provided), nil
	}
	if reader == nil {
		return "", fmt.Errorf("missing %s", strings.ToLower(label))
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "%s [%s]: ", label, fallback)
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return fallback, nil
	}
	return line, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
