package server

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/services"
)

func TestServerPackageDoesNotImportGeneratedDB(t *testing.T) {
	t.Helper()

	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, importSpec := range file.Imports {
			if strings.Trim(importSpec.Path.Value, `"`) == "github.com/inkyvoxel/go-spark/internal/db/generated" {
				t.Fatalf("%s imports internal/db/generated; server should depend on service-owned types instead", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk server package: %v", err)
	}
}

func TestTemplateDataUsesSafeAuthUser(t *testing.T) {
	userType := reflect.TypeOf(templateData{}.User)
	if userType != reflect.TypeOf(services.User{}) {
		t.Fatalf("templateData.User type = %s, want services.User", userType)
	}

	for _, fieldName := range []string{"PasswordHash", "TokenHash", "SessionToken"} {
		if _, ok := userType.FieldByName(fieldName); ok {
			t.Fatalf("templateData.User exposes sensitive field %q", fieldName)
		}
	}
}

func TestDocsDoNotReferenceRemovedAuthStorageTypes(t *testing.T) {
	t.Helper()

	repoRoot := filepath.Join("..", "..")
	for _, path := range []string{
		filepath.Join(repoRoot, "README.md"),
		filepath.Join(repoRoot, "SECURITY.md"),
		filepath.Join(repoRoot, "docs"),
		filepath.Join(repoRoot, ".env.example"),
	} {
		assertNoRemovedAuthStorageReferences(t, path)
	}
}

func assertNoRemovedAuthStorageReferences(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatalf("read dir %s: %v", path, err)
		}
		for _, entry := range entries {
			assertNoRemovedAuthStorageReferences(t, filepath.Join(path, entry.Name()))
		}
		return
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(body)
	for _, forbidden := range []string{"db.User", "db.Session", "PasswordHash", "TokenHash"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("%s references removed auth storage detail %q", path, forbidden)
		}
	}
}
