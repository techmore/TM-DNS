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

	"github.com/techmore/tm-dns/internal/config"
	"github.com/techmore/tm-dns/internal/dnsserver"
	"github.com/techmore/tm-dns/internal/htt_server"
	"github.com/techmore/tm-dns/internal/store"
)

func main() {
	cfg := config.Load()
	logger := newLogger(cfg.LogLevel)
	logger.Info("starting tm-dns", "dns_addr", cfg.DNSAddr, "http_addr", cfg.HTTPAddr, "db_path", cfg.DBPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DBPath, logger)
	if err != nil {
		logger.Error("open store", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.SeedDefaults(ctx); err != nil {
		logger.Error("seed defaults", "error", err)
		os.Exit(1)
	}
	if _, err := db.PurgeOldEvents(ctx); err != nil {
		logger.Warn("retention purge failed", "error", err)
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if removed, err := db.PurgeOldEvents(context.Background()); err != nil {
					logger.Warn("retention purge failed", "error", err)
				} else if removed > 0 {
					logger.Info("retention purge complete", "removed", removed)
				}
			}
		}
	}()

	resolver := dnsserver.New(cfg, db, logger)
	api := htt_server.New(cfg, db, resolver, logger)

	errs := make(chan error, 3)
	go func() { errs <- resolver.Start(ctx) }()
	go func() { errs <- api.Start(ctx) }()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errs:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("service failed", "error", err)
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := api.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown failed", "error", err)
	}
	if err := resolver.Shutdown(); err != nil {
		logger.Warn("dns shutdown failed", "error", err)
	}
	logger.Info("tm-dns stopped")
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
}
