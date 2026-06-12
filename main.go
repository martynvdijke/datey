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
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/scheduler"
	"github.com/datey/datey/internal/web"
	"github.com/go-chi/chi/v5"
)

const Version = "1.9.1"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialise the log store with a ring buffer.
	store := logstore.NewStore(cfg.LogBufferSize)
	initialLevel, ok := logstore.ParseLogLevel(cfg.LogLevel)
	if !ok {
		initialLevel = slog.LevelWarn
	}
	store.InitLevel(initialLevel)

	// Create the custom handler wrapping the default stderr text handler.
	textHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: store.LevelVar()})
	var otelFn func(context.Context, slog.Record)

	if cfg.OTLPEndpoint != "" {
		otelHelper, otelErr := logstore.NewOTelHelper(cfg.OTLPEndpoint)
		if otelErr != nil {
			slog.Warn("failed to initialise OTel logger, continuing without OTEL", "error", otelErr)
		} else if otelHelper != nil {
			otelFn = func(ctx context.Context, r slog.Record) {
				otelHelper.Emit(ctx, r)
			}
			slog.Info("OTel logging enabled", "endpoint", cfg.OTLPEndpoint)
		}
	}

	customHandler := logstore.NewHandler(textHandler, store, otelFn)
	slog.SetDefault(slog.New(customHandler))

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

	handler := web.NewHandler(cfg, client, reg, store)
	handlers.Version = Version
	handler.RegisterRoutes(r)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched := scheduler.New(cfg, client, reg)
	go sched.Start(ctx)

	onRepo := repository.NewOneTimeNotificationRepository(client)
	onDeliveryRepo := repository.NewNotificationDeliveryRepository(client)
	onSched := scheduler.NewOneTimeNotificationScheduler(onRepo, onDeliveryRepo, reg)
	go onSched.Start(ctx)

	// Run an initial backup on startup (non-blocking).
	go func() {
		dbPath := cfg.DataDir + "/datey.db"
		slog.Info("running initial database backup", "path", dbPath)
		if err := db.Backup(dbPath, cfg.BackupDir, cfg.BackupRetentionDays); err != nil {
			slog.Warn("initial backup failed", "error", err)
		} else {
			slog.Info("initial database backup completed", "dir", cfg.BackupDir)
		}
	}()

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


