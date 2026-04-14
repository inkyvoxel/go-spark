package server

import (
	"database/sql"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"

	db "github.com/inkyvoxel/go-spark/internal/db/generated"
	"github.com/inkyvoxel/go-spark/internal/services"
)

type Server struct {
	db        *sql.DB
	auth      authService
	logger    *slog.Logger
	templates map[string]*template.Template
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
		templates: mustParseTemplates(),
	}
}

func mustParseTemplates() map[string]*template.Template {
	templates, err := parseTemplates()
	if err != nil {
		panic(err)
	}

	return templates
}

func parseTemplates() (map[string]*template.Template, error) {
	pages := []string{
		"account.html",
		"home.html",
		"login.html",
		"register.html",
	}
	templates := make(map[string]*template.Template, len(pages))
	layout := filepath.Join("templates", "layout.html")

	for _, page := range pages {
		parsed, err := template.ParseFiles(layout, filepath.Join("templates", page))
		if err != nil {
			return nil, err
		}
		templates[page] = parsed
	}

	return templates, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	dynamic := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /healthz", s.health)

	dynamic.HandleFunc("GET /register", s.registerForm)
	dynamic.HandleFunc("POST /register", s.register)
	dynamic.HandleFunc("GET /login", s.loginForm)
	dynamic.HandleFunc("POST /login", s.login)
	dynamic.HandleFunc("POST /logout", s.logout)
	dynamic.Handle("GET /account", s.requireAuth(http.HandlerFunc(s.account)))
	dynamic.HandleFunc("GET /", s.home)

	mux.Handle("/", s.csrf(s.loadSession(dynamic)))

	return s.logRequests(mux)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, "home.html", newTemplateData(r, "Go Spark"))
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
