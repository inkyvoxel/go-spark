package server

import (
	"database/sql"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"

	db "go-starter/internal/db/generated"
	"go-starter/internal/services"
)

type Server struct {
	db        *sql.DB
	auth      authSessionLookup
	logger    *slog.Logger
	templates *template.Template
}

type Options struct {
	DB     *sql.DB
	Logger *slog.Logger
}

func New(opts Options) *Server {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		db:        opts.DB,
		auth:      services.NewAuthService(db.New(opts.DB), services.AuthOptions{}),
		logger:    logger,
		templates: template.Must(template.ParseGlob(filepath.Join("templates", "*.html"))),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /", s.home)

	return s.logRequests(mux)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	data := map[string]string{
		"Title": "Go Starter",
	}

	if err := s.templates.ExecuteTemplate(w, "home.html", data); err != nil {
		s.logger.Error("render home", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		s.logger.Error("health check failed", "err", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.logger.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
