package projectinit

import (
	"bufio"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const stateFileName = ".gospark-init-state"

type Options struct {
	ProjectName               string
	ModulePath                string
	EmailFromName             string
	EmailFromAddress          string
	DatabasePath              string
	EmailVerificationRequired *bool
}

type state struct {
	ProjectName               string
	ModulePath                string
	BinaryName                string
	EmailFromName             string
	EmailFromAddress          string
	DatabasePath              string
	EmailVerificationRequired bool
}

type operation struct {
	path      string
	transform func(string, state, state) (string, error)
}

func Run(repoRoot string, opts Options, stdin io.Reader, stdout io.Writer) error {
	current, err := detectState(repoRoot)
	if err != nil {
		return err
	}

	target, err := resolveOptions(current, opts, stdin, stdout)
	if err != nil {
		return err
	}

	applied, err := applyOperations(repoRoot, current, target)
	if err != nil {
		return err
	}

	if err := writeStateFile(repoRoot, target); err != nil {
		return err
	}
	applied = append(applied, stateFileName)

	if stdout != nil {
		fmt.Fprintln(stdout, "Project initialization complete.")
		fmt.Fprintf(stdout, "Updated module path to %s and app branding to %s.\n", target.ModulePath, target.ProjectName)
		fmt.Fprintf(stdout, "Updated files: %s\n", strings.Join(applied, ", "))
	}

	return nil
}

func detectState(repoRoot string) (state, error) {
	modulePath, err := detectModulePath(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return state{}, err
	}

	projectName := detectProjectName(filepath.Join(repoRoot, "README.md"))
	emailFromName, emailFromAddress := detectEmailFrom(filepath.Join(repoRoot, ".env.example"))
	if emailFromName == "" {
		emailFromName = projectName
	}

	emailVerificationRequired := detectEnvBool(filepath.Join(repoRoot, ".env.example"), "AUTH_EMAIL_VERIFICATION_REQUIRED", true)
	databasePath := detectEnvValue(filepath.Join(repoRoot, ".env.example"), "DATABASE_PATH", "./data/app.db")

	return state{
		ProjectName:               defaultString(projectName, "Go Spark"),
		ModulePath:                modulePath,
		BinaryName:                path.Base(modulePath),
		EmailFromName:             defaultString(emailFromName, "Go Spark"),
		EmailFromAddress:          defaultString(emailFromAddress, "hello@example.com"),
		DatabasePath:              defaultString(databasePath, "./data/app.db"),
		EmailVerificationRequired: emailVerificationRequired,
	}, nil
}

func detectModulePath(goModPath string) (string, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modulePath := strings.TrimSpace(strings.TrimPrefix(line, "module "))
			if modulePath == "" {
				break
			}
			return modulePath, nil
		}
	}

	return "", fmt.Errorf("read go.mod: missing module declaration")
}

func detectProjectName(readmePath string) string {
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}

	return ""
}

func detectEmailFrom(envPath string) (string, string) {
	value := detectEnvValue(envPath, "EMAIL_FROM", "")
	if value == "" {
		return "", ""
	}

	addr, err := mail.ParseAddress(value)
	if err != nil {
		return "", ""
	}

	return addr.Name, addr.Address
}

func detectEnvValue(envPath, key, fallback string) string {
	content, err := os.ReadFile(envPath)
	if err != nil {
		return fallback
	}

	prefix := key + "="
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		value := strings.TrimPrefix(line, prefix)
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		return value
	}

	return fallback
}

func detectEnvBool(envPath, key string, fallback bool) bool {
	value := detectEnvValue(envPath, key, "")
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func resolveOptions(current state, opts Options, stdin io.Reader, stdout io.Writer) (state, error) {
	reader := bufio.NewReader(stdin)

	projectName, err := resolveStringOption(reader, stdout, "Project name", opts.ProjectName, current.ProjectName)
	if err != nil {
		return state{}, err
	}

	modulePath, err := resolveStringOption(reader, stdout, "Go module path", opts.ModulePath, current.ModulePath)
	if err != nil {
		return state{}, err
	}

	emailFromNameDefault := current.EmailFromName
	if strings.TrimSpace(opts.ProjectName) != "" {
		emailFromNameDefault = projectName
	}
	emailFromName, err := resolveStringOption(reader, stdout, "Default email sender name", opts.EmailFromName, emailFromNameDefault)
	if err != nil {
		return state{}, err
	}

	emailFromAddress, err := resolveStringOption(reader, stdout, "Default email sender address", opts.EmailFromAddress, current.EmailFromAddress)
	if err != nil {
		return state{}, err
	}

	databasePath, err := resolveStringOption(reader, stdout, "Default database path", opts.DatabasePath, current.DatabasePath)
	if err != nil {
		return state{}, err
	}

	emailVerificationRequired, err := resolveBoolOption(reader, stdout, "Require email verification by default", opts.EmailVerificationRequired, current.EmailVerificationRequired)
	if err != nil {
		return state{}, err
	}

	modulePath = strings.TrimSpace(modulePath)
	if modulePath == "" {
		return state{}, fmt.Errorf("module path cannot be empty")
	}
	if !strings.Contains(modulePath, "/") {
		return state{}, fmt.Errorf("module path must look like a Go module path")
	}

	projectName = strings.TrimSpace(projectName)
	emailFromName = strings.TrimSpace(emailFromName)
	emailFromAddress = strings.TrimSpace(emailFromAddress)
	databasePath = strings.TrimSpace(databasePath)

	if projectName == "" || emailFromName == "" || emailFromAddress == "" || databasePath == "" {
		return state{}, fmt.Errorf("project name, email sender, and database path are required")
	}

	if _, err := mail.ParseAddress(formatAddress(emailFromName, emailFromAddress)); err != nil {
		return state{}, fmt.Errorf("invalid default email sender: %w", err)
	}

	return state{
		ProjectName:               projectName,
		ModulePath:                modulePath,
		BinaryName:                path.Base(modulePath),
		EmailFromName:             emailFromName,
		EmailFromAddress:          emailFromAddress,
		DatabasePath:              databasePath,
		EmailVerificationRequired: emailVerificationRequired,
	}, nil
}

func resolveStringOption(reader *bufio.Reader, stdout io.Writer, label, provided, fallback string) (string, error) {
	if strings.TrimSpace(provided) != "" {
		return strings.TrimSpace(provided), nil
	}

	return prompt(reader, stdout, label, fallback)
}

func resolveBoolOption(reader *bufio.Reader, stdout io.Writer, label string, provided *bool, fallback bool) (bool, error) {
	if provided != nil {
		return *provided, nil
	}

	response, err := prompt(reader, stdout, label, formatBoolDefault(fallback))
	if err != nil {
		return false, err
	}

	value, err := parseBool(response)
	if err != nil {
		return false, fmt.Errorf("%s: %w", label, err)
	}

	return value, nil
}

func prompt(reader *bufio.Reader, stdout io.Writer, label, fallback string) (string, error) {
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
		if fallback == "" {
			return "", fmt.Errorf("missing %s", strings.ToLower(label))
		}
		return fallback, nil
	}

	return line, nil
}

func formatBoolDefault(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y":
		return true, nil
	case "0", "false", "f", "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("expected yes or no")
	}
}

func applyOperations(repoRoot string, current, target state) ([]string, error) {
	ops := []operation{
		{
			path: "go.mod",
			transform: func(content string, _, target state) (string, error) {
				return replaceModuleDeclaration(content, target.ModulePath)
			},
		},
		{
			path: ".env.example",
			transform: func(content string, _, target state) (string, error) {
				content = replaceEnvValue(content, "DATABASE_PATH", target.DatabasePath)
				content = replaceEnvValue(content, "AUTH_EMAIL_VERIFICATION_REQUIRED", strconv.FormatBool(target.EmailVerificationRequired))
				content = replaceEnvValue(content, "EMAIL_FROM", fmt.Sprintf("%q", formatAddress(target.EmailFromName, target.EmailFromAddress)))
				return content, nil
			},
		},
		{
			path: "README.md",
			transform: func(content string, current, target state) (string, error) {
				content = replaceHeading(content, current.ProjectName, target.ProjectName)
				content = strings.ReplaceAll(content, current.ProjectName, target.ProjectName)
				content = strings.ReplaceAll(content, current.DatabasePath, target.DatabasePath)
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" all", "./"+target.BinaryName+" all")
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" serve", "./"+target.BinaryName+" serve")
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" worker", "./"+target.BinaryName+" worker")
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" migrate status", "./"+target.BinaryName+" migrate status")
				return content, nil
			},
		},
		{
			path: "CONTRIBUTING.md",
			transform: func(content string, current, target state) (string, error) {
				content = strings.ReplaceAll(content, current.ProjectName, target.ProjectName)
				content = strings.ReplaceAll(content, current.BinaryName+"-contrib.db", target.BinaryName+"-contrib.db")
				content = strings.ReplaceAll(content, current.DatabasePath, target.DatabasePath)
				return content, nil
			},
		},
		{
			path: "docs/architecture.md",
			transform: func(content string, current, target state) (string, error) {
				return strings.ReplaceAll(content, current.ProjectName, target.ProjectName), nil
			},
		},
		{
			path: "docs/jobs.md",
			transform: func(content string, current, target state) (string, error) {
				content = strings.ReplaceAll(content, current.ProjectName, target.ProjectName)
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" all", "./"+target.BinaryName+" all")
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" worker", "./"+target.BinaryName+" worker")
				return content, nil
			},
		},
		{
			path: "templates/layout.html",
			transform: func(content string, current, target state) (string, error) {
				return strings.ReplaceAll(content, current.ProjectName, target.ProjectName), nil
			},
		},
		{
			path: "templates/home.html",
			transform: func(_ string, _ state, target state) (string, error) {
				return homeTemplate(target.ProjectName), nil
			},
		},
		{
			path: "Makefile",
			transform: func(content string, current, target state) (string, error) {
				return strings.ReplaceAll(content, "DB_PATH ?= "+current.DatabasePath, "DB_PATH ?= "+target.DatabasePath), nil
			},
		},
		{
			path: "internal/server/server.go",
			transform: func(content string, current, target state) (string, error) {
				return strings.ReplaceAll(content, fmt.Sprintf("%q", current.ProjectName), fmt.Sprintf("%q", target.ProjectName)), nil
			},
		},
		{
			path: "internal/config/config.go",
			transform: func(content string, current, target state) (string, error) {
				return strings.ReplaceAll(content, formatAddress(current.EmailFromName, current.EmailFromAddress), formatAddress(target.EmailFromName, target.EmailFromAddress)), nil
			},
		},
		{
			path: "internal/app/build.go",
			transform: func(content string, current, target state) (string, error) {
				return strings.ReplaceAll(content, formatAddress(current.EmailFromName, current.EmailFromAddress), formatAddress(target.EmailFromName, target.EmailFromAddress)), nil
			},
		},
		{
			path: "internal/email/smtp.go",
			transform: func(content string, current, target state) (string, error) {
				content = strings.ReplaceAll(content, current.BinaryName+"-boundary", target.BinaryName+"-boundary")
				content = strings.ReplaceAll(content, "go-spark-boundary", target.BinaryName+"-boundary")
				return content, nil
			},
		},
		{
			path: "internal/projectinit/init_test.go",
			transform: func(content string, current, target state) (string, error) {
				content = strings.ReplaceAll(content, current.ProjectName, target.ProjectName)
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" all", "./"+target.BinaryName+" all")
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" serve", "./"+target.BinaryName+" serve")
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" worker", "./"+target.BinaryName+" worker")
				content = strings.ReplaceAll(content, "./"+current.BinaryName+" migrate status", "./"+target.BinaryName+" migrate status")
				content = strings.ReplaceAll(content, current.BinaryName+"-contrib.db", target.BinaryName+"-contrib.db")
				content = strings.ReplaceAll(content, current.BinaryName+"-boundary", target.BinaryName+"-boundary")
				content = strings.ReplaceAll(content, "go-spark-boundary", target.BinaryName+"-boundary")
				return content, nil
			},
		},
		{
			path: "internal/config/config_test.go",
			transform: func(content string, current, target state) (string, error) {
				content = strings.ReplaceAll(content, formatAddress(current.EmailFromName, current.EmailFromAddress), formatAddress(target.EmailFromName, target.EmailFromAddress))
				content = strings.ReplaceAll(content, quotedAddress(current.EmailFromName, current.EmailFromAddress), quotedAddress(target.EmailFromName, target.EmailFromAddress))
				content = strings.ReplaceAll(
					content,
					strings.ReplaceAll(quotedAddress(current.EmailFromName, current.EmailFromAddress), "\"", "\\\""),
					strings.ReplaceAll(quotedAddress(target.EmailFromName, target.EmailFromAddress), "\"", "\\\""),
				)
				return content, nil
			},
		},
		{
			path: "internal/server/server_test.go",
			transform: func(content string, current, target state) (string, error) {
				content = strings.ReplaceAll(content, current.ProjectName, target.ProjectName)
				return content, nil
			},
		},
	}

	applied := make([]string, 0, len(ops)+1)
	for _, op := range ops {
		changed, err := applyOperation(repoRoot, op, current, target)
		if err != nil {
			return nil, err
		}
		if changed {
			applied = append(applied, op.path)
		}
	}

	goFiles, err := replaceModuleImports(repoRoot, current.ModulePath, target.ModulePath)
	if err != nil {
		return nil, err
	}
	applied = append(applied, goFiles...)

	return uniqueStrings(applied), nil
}

func applyOperation(repoRoot string, op operation, current, target state) (bool, error) {
	fullPath := filepath.Join(repoRoot, op.path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", op.path, err)
	}

	updated, err := op.transform(string(content), current, target)
	if err != nil {
		return false, fmt.Errorf("update %s: %w", op.path, err)
	}

	if updated == string(content) {
		return false, nil
	}

	if err := os.WriteFile(fullPath, []byte(updated), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", op.path, err)
	}

	return true, nil
}

func replaceModuleImports(repoRoot, oldModulePath, newModulePath string) ([]string, error) {
	if oldModulePath == newModulePath {
		return nil, nil
	}

	changed := make([]string, 0)
	err := filepath.WalkDir(repoRoot, func(currentPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(repoRoot, currentPath, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(currentPath) != ".go" {
			return nil
		}

		content, err := os.ReadFile(currentPath)
		if err != nil {
			return err
		}

		updated := strings.ReplaceAll(string(content), oldModulePath, newModulePath)
		if updated == string(content) {
			return nil
		}

		if err := os.WriteFile(currentPath, []byte(updated), 0o644); err != nil {
			return err
		}

		relativePath, err := filepath.Rel(repoRoot, currentPath)
		if err != nil {
			return err
		}
		changed = append(changed, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("replace module imports: %w", err)
	}

	return changed, nil
}

func shouldSkipDir(repoRoot, currentPath, name string) bool {
	if currentPath == repoRoot {
		return false
	}
	if name == ".git" || name == "data" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func replaceModuleDeclaration(content, modulePath string) (string, error) {
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "module ") {
			lines[index] = "module " + modulePath
			return strings.Join(lines, "\n"), nil
		}
	}
	return "", fmt.Errorf("missing module declaration")
}

func replaceEnvValue(content, key, value string) string {
	lines := strings.Split(content, "\n")
	prefix := key + "="
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[index] = prefix + value
			return strings.Join(lines, "\n")
		}
	}
	return content
}

func replaceHeading(content, currentHeading, targetHeading string) string {
	current := "# " + currentHeading
	target := "# " + targetHeading
	return strings.Replace(content, current, target, 1)
}

func formatAddress(name, address string) string {
	return fmt.Sprintf("%s <%s>", name, address)
}

func quotedAddress(name, address string) string {
	return (&mail.Address{Name: name, Address: address}).String()
}

func homeTemplate(appName string) string {
	return fmt.Sprintf(`{{ define "content" }}
<section>
  <h1>Welcome to %s.</h1>
  <p>
    The starter setup is ready. Replace this page with your product's first real screen when you're ready to start building.
  </p>
</section>
{{ end }}
`, appName)
}

func writeStateFile(repoRoot string, target state) error {
	content := strings.Join([]string{
		"PROJECT_NAME=" + target.ProjectName,
		"MODULE_PATH=" + target.ModulePath,
		"EMAIL_FROM_NAME=" + target.EmailFromName,
		"EMAIL_FROM_ADDRESS=" + target.EmailFromAddress,
		"DATABASE_PATH=" + target.DatabasePath,
		"AUTH_EMAIL_VERIFICATION_REQUIRED=" + strconv.FormatBool(target.EmailVerificationRequired),
		"",
	}, "\n")

	if err := os.WriteFile(filepath.Join(repoRoot, stateFileName), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", stateFileName, err)
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
