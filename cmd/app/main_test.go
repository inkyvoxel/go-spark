package main

import (
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/config"
)

func TestParseCLIArgsReturnsEmptyWhenNoArg(t *testing.T) {
	command, err := parseCLIArgs(nil)
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.processOverride != "" {
		t.Fatalf("parseCLIArgs() processOverride = %q, want empty", command.processOverride)
	}
}

func TestParseCLIArgsSupportsStart(t *testing.T) {
	command, err := parseCLIArgs([]string{"start"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.name != "start" {
		t.Fatalf("parseCLIArgs() name = %q, want start", command.name)
	}
	if command.processOverride != config.ProcessAll {
		t.Fatalf("parseCLIArgs() processOverride = %q, want %q", command.processOverride, config.ProcessAll)
	}
}

func TestParseCLIArgsSupportsStartWeb(t *testing.T) {
	command, err := parseCLIArgs([]string{"start", "web"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.processOverride != config.ProcessWeb {
		t.Fatalf("parseCLIArgs() processOverride = %q, want %q", command.processOverride, config.ProcessWeb)
	}
}

func TestParseCLIArgsSupportsStartWorker(t *testing.T) {
	command, err := parseCLIArgs([]string{"start", "worker"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.processOverride != config.ProcessWorker {
		t.Fatalf("parseCLIArgs() processOverride = %q, want %q", command.processOverride, config.ProcessWorker)
	}
}

func TestParseCLIArgsSupportsInit(t *testing.T) {
	command, err := parseCLIArgs([]string{"init", "-project-name", "Acme", "-module-path", "github.com/acme/app", "-database-path", "./data/acme.db", "-email-verification", "false"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.name != "init" {
		t.Fatalf("parseCLIArgs() name = %q, want init", command.name)
	}
	if command.initOptions == nil {
		t.Fatal("parseCLIArgs() initOptions = nil, want options")
	}
	if command.initOptions.ProjectName != "Acme" {
		t.Fatalf("parseCLIArgs() init project name = %q, want Acme", command.initOptions.ProjectName)
	}
	if command.initOptions.ModulePath != "github.com/acme/app" {
		t.Fatalf("parseCLIArgs() init module path = %q, want github.com/acme/app", command.initOptions.ModulePath)
	}
	if command.initOptions.DatabasePath != "./data/acme.db" {
		t.Fatalf("parseCLIArgs() database path = %q, want ./data/acme.db", command.initOptions.DatabasePath)
	}
	if command.initOptions.EmailVerificationRequired == nil || *command.initOptions.EmailVerificationRequired {
		t.Fatalf("parseCLIArgs() email verification = %v, want false", command.initOptions.EmailVerificationRequired)
	}
}

func TestParseCLIArgsRejectsRemovedTrimStarterFlag(t *testing.T) {
	_, err := parseCLIArgs([]string{"init", "-trim-starter", "true"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown flag context", err)
	}
}

func TestParseCLIArgsRejectsInvalidCommand(t *testing.T) {
	_, err := parseCLIArgs([]string{"jobs"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown command context", err)
	}
}

func TestParseCLIArgsRejectsLegacyServeCommand(t *testing.T) {
	_, err := parseCLIArgs([]string{"serve"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown command context", err)
	}
}

func TestParseCLIArgsRejectsLegacyWorkerCommand(t *testing.T) {
	_, err := parseCLIArgs([]string{"worker"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown command context", err)
	}
}

func TestParseCLIArgsRejectsLegacyRunCommand(t *testing.T) {
	_, err := parseCLIArgs([]string{"run", "web"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown command context", err)
	}
}

func TestParseCLIArgsRejectsStartExtraArgs(t *testing.T) {
	_, err := parseCLIArgs([]string{"start", "web", "extra"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "start subcommand accepts at most one") {
		t.Fatalf("parseCLIArgs() error = %v, want start argument context", err)
	}
}

func TestParseCLIArgsRejectsStartInvalidMode(t *testing.T) {
	_, err := parseCLIArgs([]string{"start", "jobs"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "start mode must be") {
		t.Fatalf("parseCLIArgs() error = %v, want start mode context", err)
	}
}

func TestParseCLIArgsRejectsInitPositionalArgs(t *testing.T) {
	_, err := parseCLIArgs([]string{"init", "extra"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "init subcommand does not accept positional arguments") {
		t.Fatalf("parseCLIArgs() error = %v, want init positional argument context", err)
	}
}
