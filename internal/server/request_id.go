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
		rec := &statusCapturingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		s.loggerForRequest(r).Info(
			"http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_ip", requestIP(r),
		)
	})
}

type statusCapturingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapturingResponseWriter) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}
