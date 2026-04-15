package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/inkyvoxel/go-spark/internal/config"
	"github.com/inkyvoxel/go-spark/internal/database"
	"github.com/inkyvoxel/go-spark/internal/server"
	"github.com/inkyvoxel/go-spark/internal/services"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.FromEnv(services.DefaultPasswordMinLength)
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	auth := services.NewAuthService(database.NewAuthStore(db), services.AuthOptions{
		PasswordMinLen: cfg.PasswordMinLength,
	})

	app := server.New(server.Options{
		Logger:            logger,
		DB:                db,
		Auth:              auth,
		CookieSecure:      cfg.CookieSecure,
		PasswordMinLength: cfg.PasswordMinLength,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "env", cfg.Env)
		errs <- httpServer.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown server", "err", err)
			os.Exit(1)
		}
		logger.Info("server stopped")
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}
}
