package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestRunJobOnceLogsLifecycleWithStableFields(t *testing.T) {
	var output bytes.Buffer
	runner := &Runner{
		logger: slog.New(slog.NewJSONHandler(&output, nil)),
	}

	ok := runner.runJobOnce(context.Background(), Job{
		Name:     "example",
		Interval: 2 * time.Second,
		Run: func(ctx context.Context) error {
			return nil
		},
	})
	if !ok {
		t.Fatal("runJobOnce() = false, want true")
	}

	entries := decodeLogEntries(t, output.String())
	start, found := findEntry(entries, "background job starting")
	if !found {
		t.Fatalf("missing start log entry in %v", entries)
	}
	if got := asString(start["job"]); got != "example" {
		t.Fatalf("start job = %q, want %q", got, "example")
	}
	if got := asInt(start["interval_ms"]); got != 2000 {
		t.Fatalf("start interval_ms = %d, want %d", got, 2000)
	}

	done, found := findEntry(entries, "background job finished")
	if !found {
		t.Fatalf("missing finish log entry in %v", entries)
	}
	if got := asString(done["job"]); got != "example" {
		t.Fatalf("finish job = %q, want %q", got, "example")
	}
	if got := asInt(done["duration_ms"]); got < 0 {
		t.Fatalf("finish duration_ms = %d, want >= 0", got)
	}
}

func TestRunJobOnceLogsFailureWithDurationMs(t *testing.T) {
	var output bytes.Buffer
	runner := &Runner{
		logger: slog.New(slog.NewJSONHandler(&output, nil)),
	}

	ok := runner.runJobOnce(context.Background(), Job{
		Name:     "failing",
		Interval: time.Second,
		Run: func(ctx context.Context) error {
			return errors.New("boom")
		},
	})
	if !ok {
		t.Fatal("runJobOnce() = false, want true")
	}

	entries := decodeLogEntries(t, output.String())
	failed, found := findEntry(entries, "background job failed")
	if !found {
		t.Fatalf("missing failure log entry in %v", entries)
	}
	if got := asString(failed["job"]); got != "failing" {
		t.Fatalf("failed job = %q, want %q", got, "failing")
	}
	if got := asInt(failed["duration_ms"]); got < 0 {
		t.Fatalf("failed duration_ms = %d, want >= 0", got)
	}
}

func decodeLogEntries(t *testing.T, raw string) []map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := map[string]any{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("json.Unmarshal(%q) error = %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func findEntry(entries []map[string]any, msg string) (map[string]any, bool) {
	for _, entry := range entries {
		if asString(entry["msg"]) == msg {
			return entry, true
		}
	}
	return nil, false
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt(v any) int {
	switch value := v.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}
