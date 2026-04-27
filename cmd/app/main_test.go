package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
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

func TestConfigureAppLoggerUsesJSONFormat(t *testing.T) {
	var output bytes.Buffer
	logger := configureAppLogger("json", &output)

	logger.Info("test message", "key", "value")

	entry := map[string]any{}
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := entry[slog.MessageKey]; got != "test message" {
		t.Fatalf("message = %v, want %q", got, "test message")
	}
	if got := entry["key"]; got != "value" {
		t.Fatalf("key = %v, want %q", got, "value")
	}
}

func TestConfigureAppLoggerFallsBackToTextFormat(t *testing.T) {
	var output bytes.Buffer
	logger := configureAppLogger("yaml", &output)

	logger.Info("text message", "key", "value")
	got := output.String()
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Fatalf("logger output appears to be json: %q", got)
	}
	if !strings.Contains(got, "msg=\"text message\"") {
		t.Fatalf("logger output = %q, want text handler output", got)
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

func TestParseCLIArgsRejectsInitCommand(t *testing.T) {
	_, err := parseCLIArgs([]string{"init"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown command context", err)
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

func TestParseCLIArgsRejectsInitWithPositionalArgs(t *testing.T) {
	_, err := parseCLIArgs([]string{"init", "extra"})
	if err == nil {
		t.Fatal("parseCLIArgs() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("parseCLIArgs() error = %v, want unknown command context", err)
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
