package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/datey/datey/handlers"
	"github.com/datey/datey/internal/config"
	"github.com/datey/datey/internal/db"
	"github.com/datey/datey/internal/logstore"
	"github.com/datey/datey/internal/notifier"
	"github.com/datey/datey/internal/repository"
	"github.com/datey/datey/internal/scheduler"
	"github.com/datey/datey/internal/settings"
	"github.com/datey/datey/internal/web"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

const Version = "1.20.0"

func main() {
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
	defer func() { _ = client.Close() }()

	// Overlay DB-stored settings onto the env-derived config. Non-null columns
	// from the singleton app_config row win over env values; null columns keep
	// the env value. DataDir is never overlaid (the DB is already open at the
	// env path). This happens before the log store, OTel helper, notifiers,
	// scheduler, and server are constructed so that DB-stored LogBufferSize,
	// OTLPEndpoint, Port, SchedulerHour, etc. take effect on this boot.
	settingsStore := settings.New(client)
	if err := settingsStore.EnsureSeeded(context.Background()); err != nil {
		slog.Error("failed to seed app_config", "error", err)
		os.Exit(1)
	}
	if err := settingsStore.Overlay(context.Background(), cfg); err != nil {
		slog.Warn("failed to overlay settings from DB, continuing with env config", "error", err)
	}

	// Run data migration from contact→person for existing deployments
	if err := db.MigrateContactsToPeople(context.Background(), client); err != nil {
		slog.Error("failed to migrate contacts to people", "error", err)
		os.Exit(1)
	}

	// Initialise the log store with a ring buffer. Done after the settings
	// overlay so a DB-stored LogBufferSize/OTLPEndpoint takes effect this boot.
	store := logstore.NewStore(cfg.LogBufferSize)
	initialLevel, ok := logstore.ParseLogLevel(cfg.LogLevel)
	if !ok {
		initialLevel = slog.LevelWarn
	}
	store.InitLevel(initialLevel)

	textHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: store.LevelVar()})
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

	reg := notifier.NewRegistry()
	reg.Register(notifier.NewEmailNotifier(cfg))
	reg.Register(notifier.NewGotifyNotifier(cfg))
	reg.Register(notifier.NewTelegramNotifier(cfg))

	r := chi.NewRouter()

	// Middleware for logging, recovery, and request ID
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)

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

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("starting server", "addr", addr, "version", Version)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}


