package generator

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewProjectGeneratesStarterApp(t *testing.T) {
	target := filepath.Join(t.TempDir(), "acme")
	var stdout bytes.Buffer

	gen := New()
	gen.Stdout = &stdout
	result, err := gen.NewProject(ProjectOptions{
		TargetPath:   target,
		ProjectName:  "Acme Portal",
		ModulePath:   "github.com/acme/portal",
		DatabasePath: "./data/acme.db",
		EmailFrom:    "Acme Portal <team@acme.test>",
		Features:     []string{FeatureEmailVerification},
		Yes:          true,
	})
	if err != nil {
		t.Fatalf("NewProject() error = %v", err)
	}

	assertGeneratedFileContains(t, target, "go.mod", "module github.com/acme/portal")
	assertGeneratedFileContains(t, target, ".env.example", "DATABASE_PATH=./data/acme.db")
	assertGeneratedFileContains(t, target, ".env.example", "EMAIL_FROM=\"Acme Portal <team@acme.test>\"")
	assertGeneratedFileContains(t, target, "cmd/app/main.go", "github.com/acme/portal/internal/app")
	assertGeneratedFileContains(t, target, "templates/layout.html", "Acme Portal")
	assertGeneratedFileContains(t, target, "templates/home.html", "Welcome to Acme Portal.")
	assertGeneratedFileContains(t, target, "docs/todo.md", "No open TODOs.")
	assertGeneratedFileNotContains(t, target, "Makefile", "build-generator")
	assertGeneratedFileNotContains(t, target, "docs/development.md", "cmd/go-spark")
	assertGeneratedFileNotContains(t, target, "docs/architecture.md", "internal/generator")
	assertGeneratedFileNotExists(t, target, "internal/projectinit/init.go")

	if len(result.Files) == 0 {
		t.Fatal("NewProject() generated no files")
	}
	if !strings.Contains(stdout.String(), "Selected components:") {
		t.Fatalf("stdout = %q, want selected component summary", stdout.String())
	}
}

func TestNewProjectRejectsNonEmptyTargetWithoutForce(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "existing.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	gen := New()
	gen.Stdout = nil
	_, err := gen.NewProject(ProjectOptions{
		TargetPath:  target,
		ModulePath:  "github.com/acme/portal",
		ProjectName: "Acme Portal",
		EmailFrom:   "Acme Portal <team@acme.test>",
		Yes:         true,
	})
	if err == nil {
		t.Fatal("NewProject() error = nil, want non-empty target error")
	}
}

func TestNewProjectRejectsInvalidModulePath(t *testing.T) {
	gen := New()
	gen.Stdout = nil
	_, err := gen.NewProject(ProjectOptions{
		TargetPath:  filepath.Join(t.TempDir(), "app"),
		ModulePath:  "app",
		ProjectName: "App",
		EmailFrom:   "App <hello@example.com>",
		Yes:         true,
	})
	if err == nil {
		t.Fatal("NewProject() error = nil, want invalid module path error")
	}
}

func assertGeneratedFileContains(t *testing.T, root, relativePath, fragment string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
	}
	if !strings.Contains(string(body), fragment) {
		t.Fatalf("%s = %q, want fragment %q", relativePath, string(body), fragment)
	}
}

func assertGeneratedFileNotExists(t *testing.T, root, relativePath string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, relativePath))
	if err == nil {
		t.Fatalf("%s exists, want absent", relativePath)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("Stat(%q) error = %v", relativePath, err)
	}
}

func assertGeneratedFileNotContains(t *testing.T, root, relativePath, fragment string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", relativePath, err)
	}
	if strings.Contains(string(body), fragment) {
		t.Fatalf("%s = %q, do not want fragment %q", relativePath, string(body), fragment)
	}
}
