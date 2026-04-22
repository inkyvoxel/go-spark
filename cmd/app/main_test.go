package main

import (
	"strings"
	"testing"

	"github.com/inkyvoxel/go-spark/internal/config"
)

func TestProcessArgReturnsEmptyWhenNoArg(t *testing.T) {
	process, err := processArg(nil)
	if err != nil {
		t.Fatalf("processArg() error = %v", err)
	}
	if process != "" {
		t.Fatalf("processArg() = %q, want empty", process)
	}
}

func TestProcessArgReturnsValidMode(t *testing.T) {
	process, err := processArg([]string{"web"})
	if err != nil {
		t.Fatalf("processArg() error = %v", err)
	}
	if process != config.ProcessWeb {
		t.Fatalf("processArg() = %q, want %q", process, config.ProcessWeb)
	}
}

func TestProcessArgRejectsInvalidMode(t *testing.T) {
	_, err := processArg([]string{"jobs"})
	if err == nil {
		t.Fatal("processArg() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "process mode") {
		t.Fatalf("processArg() error = %v, want process mode context", err)
	}
}

func TestProcessArgRejectsMultipleArgs(t *testing.T) {
	_, err := processArg([]string{"web", "worker"})
	if err == nil {
		t.Fatal("processArg() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("processArg() error = %v, want argument count context", err)
	}
}
