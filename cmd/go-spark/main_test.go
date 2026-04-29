package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNewCreatesProject(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app")
	var stdout bytes.Buffer

	err := run([]string{
		"new",
		target,
		"-project-name", "Acme",
		"-module-path", "github.com/acme/app",
		"-database-path", "./data/acme.db",
		"-email-from", "Acme <team@acme.test>",
		"-features", "auth",
		"-yes",
	}, strings.NewReader(""), &stdout)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Created Acme") {
		t.Fatalf("stdout = %q, want creation summary", stdout.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"init"}, strings.NewReader(""), ioDiscard{})
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v, want unknown command", err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
