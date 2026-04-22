package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	bootstrap "github.com/inkyvoxel/go-spark/internal/app"
	"github.com/inkyvoxel/go-spark/internal/config"
	"github.com/inkyvoxel/go-spark/internal/jobs"
	"github.com/inkyvoxel/go-spark/internal/services"
)

type cliCommand struct {
	processOverride string
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := run(os.Args[1:], logger); err != nil {
		logger.Error("application failed", "err", err)
		os.Exit(1)
	}
}

func run(args []string, logger *slog.Logger) error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}

	command, err := parseCLIArgs(args)
	if err != nil {
		return err
	}

	cfg, err := config.FromEnvWithProcess(services.DefaultPasswordMinLength, command.processOverride)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	runtime, err := bootstrap.Build(cfg, logger)
	if err != nil {
		return err
	}
	defer runtime.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.Process {
	case config.ProcessAll:
		return runAll(ctx, logger, cfg, runtime.HTTPServer, runtime.JobsRunner)
	case config.ProcessWeb:
		return runWeb(ctx, logger, cfg, runtime.HTTPServer)
	case config.ProcessWorker:
		return runWorker(ctx, logger, cfg, runtime.JobsRunner)
	default:
		return fmt.Errorf("APP_PROCESS must be %q, %q, or %q", config.ProcessAll, config.ProcessWeb, config.ProcessWorker)
	}
}

func parseCLIArgs(args []string) (cliCommand, error) {
	if len(args) == 0 {
		return cliCommand{}, nil
	}

	switch command := strings.ToLower(strings.TrimSpace(args[0])); command {
	case "start":
		return parseStartArgs(args[1:])
	default:
		return cliCommand{}, fmt.Errorf("unknown command %q; use %q", command, "start")
	}
}

func parseStartArgs(args []string) (cliCommand, error) {
	if len(args) == 0 {
		return cliCommand{processOverride: config.ProcessAll}, nil
	}
	if len(args) > 1 {
		return cliCommand{}, fmt.Errorf("start subcommand accepts at most one mode argument (%q, %q, or %q)", config.ProcessAll, config.ProcessWeb, config.ProcessWorker)
	}

	process := strings.ToLower(strings.TrimSpace(args[0]))
	switch process {
	case config.ProcessAll, config.ProcessWeb, config.ProcessWorker:
		return cliCommand{processOverride: process}, nil
	default:
		return cliCommand{}, fmt.Errorf("start mode must be %q, %q, or %q", config.ProcessAll, config.ProcessWeb, config.ProcessWorker)
	}
}

func runAll(ctx context.Context, logger *slog.Logger, cfg config.Config, httpServer *http.Server, jobsRunner *jobs.Runner) error {
	errs := make(chan error, 2)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
		errs <- httpServer.ListenAndServe()
	}()

	go func() {
		if err := jobsRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errs <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		logger.Info("server stopped")
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}

func runWeb(ctx context.Context, logger *slog.Logger, cfg config.Config, httpServer *http.Server) error {
	errs := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.Addr, "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
		errs <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		logger.Info("server stopped")
		return nil
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runWorker(ctx context.Context, logger *slog.Logger, cfg config.Config, jobsRunner *jobs.Runner) error {
	logger.Info("background jobs worker starting", "env", cfg.Env, "email_provider", cfg.EmailProvider, "process", cfg.Process)
	if err := jobsRunner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("background jobs worker stopped")
	return nil
}
