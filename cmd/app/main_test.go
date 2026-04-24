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

func TestParseCLIArgsSupportsServe(t *testing.T) {
	command, err := parseCLIArgs([]string{"serve"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.name != "serve" {
		t.Fatalf("parseCLIArgs() name = %q, want serve", command.name)
	}
	if command.processOverride != config.ProcessWeb {
		t.Fatalf("parseCLIArgs() processOverride = %q, want %q", command.processOverride, config.ProcessWeb)
	}
}

func TestParseCLIArgsSupportsWorkerSubcommand(t *testing.T) {
	command, err := parseCLIArgs([]string{"worker"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.name != "worker" {
		t.Fatalf("parseCLIArgs() name = %q, want worker", command.name)
	}
	if command.processOverride != config.ProcessWorker {
		t.Fatalf("parseCLIArgs() processOverride = %q, want %q", command.processOverride, config.ProcessWorker)
	}
}

func TestParseCLIArgsSupportsAll(t *testing.T) {
	command, err := parseCLIArgs([]string{"all"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.name != "all" {
		t.Fatalf("parseCLIArgs() name = %q, want all", command.name)
	}
	if command.processOverride != config.ProcessAll {
		t.Fatalf("parseCLIArgs() processOverride = %q, want %q", command.processOverride, config.ProcessAll)
	}
}

func TestParseCLIArgsSupportsMigrate(t *testing.T) {
	command, err := parseCLIArgs([]string{"migrate", "status"})
	if err != nil {
		t.Fatalf("parseCLIArgs() error = %v", err)
	}
	if command.name != "migrate" {
		t.Fatalf("parseCLIArgs() name = %q, want migrate", command.name)
	}
	if command.migrateAction != "status" {
		t.Fatalf("parseCLIArgs() migrateAction = %q, want status", command.migrateAction)
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

func TestParseCLIArgsRejectsLegacyRunCommand(t *testing.T) {
	_, err := parseCLIArgs([]string{"run", "web"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown command context", err)
	}
}

func TestParseCLIArgsRejectsInvalidMigrateArgs(t *testing.T) {
	_, err := parseCLIArgs([]string{"migrate"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "migrate subcommand requires exactly one action") {
		t.Fatalf("parseCLIArgs() error = %v, want migrate argument context", err)
	}
}

func TestParseCLIArgsRejectsInvalidMigrateAction(t *testing.T) {
	_, err := parseCLIArgs([]string{"migrate", "redo"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "migrate action must be") {
		t.Fatalf("parseCLIArgs() error = %v, want migrate action context", err)
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

func TestParseCLIArgsRejectsLegacyStartCommand(t *testing.T) {
	tests := [][]string{
		{"start"},
		{"start", "web"},
		{"start", "worker"},
	}

	for _, args := range tests {
		_, err := parseCLIArgs(args)
		if err == nil {
			t.Fatalf("parseCLIArgs(%v) error = nil, want error", args)
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("parseCLIArgs(%v) error = %v, want unknown command context", args, err)
		}
	}
}

func TestStartupURLUsesLocalBaseURLWhenPresent(t *testing.T) {
	cfg := config.Config{
		AppBaseURL: "http://localhost:8080/account?tab=settings",
		Addr:       ":9999",
	}

	got := startupURL(cfg)
	if got != "http://localhost:8080/account" {
		t.Fatalf("startupURL() = %q, want %q", got, "http://localhost:8080/account")
	}
}

func TestStartupURLFallsBackToListenAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "wildcard port", addr: ":8080", want: "http://localhost:8080/"},
		{name: "ipv4 wildcard", addr: "0.0.0.0:3000", want: "http://localhost:3000/"},
		{name: "ipv6 wildcard", addr: "[::]:4000", want: "http://localhost:4000/"},
		{name: "localhost", addr: "localhost:5000", want: "http://localhost:5000/"},
		{name: "loopback ipv4", addr: "127.0.0.1:6000", want: "http://127.0.0.1:6000/"},
		{name: "loopback ipv6", addr: "[::1]:7000", want: "http://[::1]:7000/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Config{Addr: tt.addr}
			if got := startupURL(cfg); got != tt.want {
				t.Fatalf("startupURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStartupURLIgnoresNonLocalBaseURL(t *testing.T) {
	cfg := config.Config{
		AppBaseURL: "https://app.example.com",
		Addr:       ":8080",
	}

	got := startupURL(cfg)
	if got != "http://localhost:8080/" {
		t.Fatalf("startupURL() = %q, want %q", got, "http://localhost:8080/")
	}
}

func TestStartupURLReturnsEmptyForNonLocalAddr(t *testing.T) {
	cfg := config.Config{Addr: "192.168.1.10:8080"}

	if got := startupURL(cfg); got != "" {
		t.Fatalf("startupURL() = %q, want empty", got)
	}
}
