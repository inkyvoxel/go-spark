package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	requestIDHeaderName = "X-Request-Id"
	maxRequestIDLength  = 64
)

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := requestIDFromHeader(r.Header.Get(requestIDHeaderName))
		if id == "" {
			generatedID, err := newRequestID()
			if err != nil {
				s.logger.Error("generate request id", "err", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			id = generatedID
		}

		ctx := contextWithRequestID(r.Context(), id)
		r = r.WithContext(ctx)
		w.Header().Set(requestIDHeaderName, id)
		next.ServeHTTP(w, r)
	})
}

func requestIDFromHeader(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxRequestIDLength {
		return ""
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return ""
		}
	}
	return value
}

func newRequestID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Server) loggerForRequest(r *http.Request) *slog.Logger {
	return s.loggerForRequestID(requestID(r.Context()))
}

func (s *Server) loggerForRequestID(id string) *slog.Logger {
	if id == "" {
		return s.logger
	}
	return s.logger.With("request_id", id)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		_, hasUser := currentUser(r.Context())
		authState := &requestAuthState{authenticated: hasUser}
		r = r.WithContext(contextWithRequestAuthState(r.Context(), authState))

		rec := &statusCapturingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		path := r.URL.Path
		route := strings.TrimSpace(r.Pattern)
		if route == "" || (route == "/" && path != "/") {
			route = path
		}

		authenticated := authState.authenticated
		attrs := []any{
			"method", r.Method,
			"route", route,
			"path", path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"response_bytes", rec.written,
			"remote_ip", requestIP(r),
			"authenticated", authenticated,
		}

		logger := s.loggerForRequest(r)
		if rec.status >= http.StatusInternalServerError || rec.status == http.StatusTooManyRequests {
			logger.Warn("http request", attrs...)
			return
		}
		logger.Info("http request", attrs...)
	})
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (w *statusCapturingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapturingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}
