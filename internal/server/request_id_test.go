package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithRequestIDGeneratesAndSetsHeader(t *testing.T) {
	srv := &Server{logger: discardLogger()}
	handler := srv.withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := requestID(r.Context())
		if id == "" {
			t.Fatal("request ID missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get(requestIDHeaderName)
	if got == "" {
		t.Fatal("response missing request ID header")
	}
	if len(got) != 32 {
		t.Fatalf("request ID length = %d, want 32", len(got))
	}
}

func TestWithRequestIDPreservesValidHeader(t *testing.T) {
	srv := &Server{logger: discardLogger()}
	handler := srv.withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	const want = "client_Request-123"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeaderName, want)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(requestIDHeaderName); got != want {
		t.Fatalf("response request ID = %q, want %q", got, want)
	}
}

func TestWithRequestIDReplacesInvalidHeader(t *testing.T) {
	srv := &Server{logger: discardLogger()}
	handler := srv.withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(requestIDHeaderName, "invalid request id!")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get(requestIDHeaderName)
	if got == "" {
		t.Fatal("response missing request ID header")
	}
	if got == "invalid request id!" {
		t.Fatal("invalid inbound request ID was not replaced")
	}
}

func TestLogRequestsIncludesRequestIDStatusMethodAndPath(t *testing.T) {
	var logBuf bytes.Buffer
	logger := jsonLogger(&logBuf)
	srv := &Server{logger: logger}

	handler := srv.withRequestID(srv.logRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	})))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	wantRequestID := rec.Header().Get(requestIDHeaderName)
	if wantRequestID == "" {
		t.Fatal("response missing request ID header")
	}

	entries := decodeJSONLogLines(t, logBuf.String())
	entry, ok := findLogEntry(entries, "http request")
	if !ok {
		t.Fatalf("missing access log entry in logs: %v", entries)
	}

	if got := asString(entry["request_id"]); got != wantRequestID {
		t.Fatalf("logged request_id = %q, want %q", got, wantRequestID)
	}
	if got := asString(entry["method"]); got != http.MethodGet {
		t.Fatalf("logged method = %q, want %q", got, http.MethodGet)
	}
	if got := asString(entry["path"]); got != "/healthz" {
		t.Fatalf("logged path = %q, want %q", got, "/healthz")
	}
	if got := asInt(entry["status"]); got != http.StatusCreated {
		t.Fatalf("logged status = %d, want %d", got, http.StatusCreated)
	}
	if got := asInt(entry["duration_ms"]); got < 0 {
		t.Fatalf("logged duration_ms = %d, want >= 0", got)
	}
}

func TestHandlerErrorLogIncludesResponseRequestID(t *testing.T) {
	var logBuf bytes.Buffer
	logger := jsonLogger(&logBuf)
	srv := &Server{logger: logger}

	handler := srv.withRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.loggerForRequest(r).Error("handler failed", "err", "boom")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	wantRequestID := rec.Header().Get(requestIDHeaderName)
	if wantRequestID == "" {
		t.Fatal("response missing request ID header")
	}

	entries := decodeJSONLogLines(t, logBuf.String())
	entry, ok := findLogEntry(entries, "handler failed")
	if !ok {
		t.Fatalf("missing handler error log entry in logs: %v", entries)
	}
	if got := asString(entry["request_id"]); got != wantRequestID {
		t.Fatalf("logged request_id = %q, want %q", got, wantRequestID)
	}
}

func TestRequestIDFromHeaderValidation(t *testing.T) {
	valid := []string{
		"abc",
		"ABC_123-xyz",
		strings.Repeat("a", maxRequestIDLength),
	}
	for _, input := range valid {
		if got := requestIDFromHeader(input); got != input {
			t.Fatalf("requestIDFromHeader(%q) = %q, want %q", input, got, input)
		}
	}

	invalid := []string{
		"",
		"   ",
		"contains space",
		"bad!",
		strings.Repeat("a", maxRequestIDLength+1),
	}
	for _, input := range invalid {
		if got := requestIDFromHeader(input); got != "" {
			t.Fatalf("requestIDFromHeader(%q) = %q, want empty", input, got)
		}
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func jsonLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, nil))
}

func decodeJSONLogLines(t *testing.T, output string) []map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry := map[string]any{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to parse log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func findLogEntry(entries []map[string]any, msg string) (map[string]any, bool) {
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
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
