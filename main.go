package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/datey/datey/handlers"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/scheduler"
	"github.com/datey/datey/internal/web"
	"github.com/go-chi/chi/v5"
)

const Version = "0.1.0"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	client, err := db.Init(cfg)
	if err != nil {
		slog.Error("failed to init database", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	reg := notifier.NewRegistry()
	reg.Register(notifier.NewEmailNotifier(cfg))
	reg.Register(notifier.NewGotifyNotifier(cfg))
	reg.Register(notifier.NewTelegramNotifier(cfg))

	r := chi.NewRouter()

	handler := web.NewHandler(cfg, client, reg)
	r.Get("/health", handlers.HealthCheck)
	handler.RegisterRoutes(r)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched := scheduler.New(cfg, client, reg)
	go sched.Start(ctx)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		cancel()
		srv.Shutdown(context.Background())
	}()

	slog.Info("starting server", "port", cfg.Port, "version", Version)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
