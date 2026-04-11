package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/penche/router/internal/adapters"
	localadapter "github.com/penche/router/internal/adapters/local"
	taigaadapter "github.com/penche/router/internal/adapters/taiga"
	webhookadapter "github.com/penche/router/internal/adapters/webhook"
	"github.com/penche/router/internal/api"
	"github.com/penche/router/internal/auth"
	"github.com/penche/router/internal/config"
	"github.com/penche/router/internal/storage"
	"github.com/penche/router/internal/worker"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	log := buildLogger(cfg.Log)

	store, err := storage.New(cfg.Storage.DSN)
	if err != nil {
		log.Error("storage init failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	adapterMap := buildAdapters(cfg, log)

	verifier := auth.NewVerifier(cfg.Auth.SharedSecret)
	handler := api.New(store, verifier, cfg.Routes, cfg.Worker, log)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	handler.Mount(r)

	srv := &http.Server{
		Addr:         cfg.Server.Addr(),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := worker.New(store, adapterMap, cfg.Worker, log)
	go w.Run(ctx)

	go func() {
		log.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutdown signal received")
	cancel() // stop worker

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	log.Info("server stopped")
}

func buildAdapters(cfg *config.Config, log *slog.Logger) map[string]adapters.DestinationAdapter {
	m := make(map[string]adapters.DestinationAdapter)

	if cfg.Adapters.Taiga.Enabled {
		ta := taigaadapter.New(cfg.Adapters.Taiga)
		if err := ta.ValidateConfig(); err != nil {
			log.Error("taiga adapter config invalid", "error", err)
			os.Exit(1)
		}
		// Resolve numeric project ID from slug at startup so it fails fast.
		startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer startupCancel()
		if err := ta.ResolveProjectID(startupCtx); err != nil {
			log.Error("taiga: could not resolve project", "error", err)
			os.Exit(1)
		}
		m[ta.Name()] = ta
		log.Info("adapter registered", "name", ta.Name())
	}

	if cfg.Adapters.Webhook.Enabled {
		wa := webhookadapter.New(cfg.Adapters.Webhook)
		if err := wa.ValidateConfig(); err != nil {
			log.Error("webhook adapter config invalid", "error", err)
			os.Exit(1)
		}
		m[wa.Name()] = wa
		log.Info("adapter registered", "name", wa.Name())
	}

	if cfg.Adapters.Local.Enabled {
		la := localadapter.New(cfg.Adapters.Local)
		if err := la.ValidateConfig(); err != nil {
			log.Error("local adapter config invalid", "error", err)
			os.Exit(1)
		}
		m[la.Name()] = la
		log.Info("adapter registered", "name", la.Name(), "output_dir", cfg.Adapters.Local.OutputDir)
	}

	if len(m) == 0 {
		// Default fallback: save to ./output so the tool works out of the box.
		log.Warn("no adapters enabled — enabling local adapter with default output dir './output'")
		la := localadapter.New(config.LocalConfig{Enabled: true, OutputDir: "output"})
		m[la.Name()] = la
	}

	return m
}

func buildLogger(cfg config.LogConfig) *slog.Logger {
	level := slog.LevelInfo
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
