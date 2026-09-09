package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rosemary-virsree/internal/buildinfo"
	"rosemary-virsree/internal/config"
	"rosemary-virsree/internal/httpapi"
	"rosemary-virsree/internal/provider"
	"rosemary-virsree/internal/secretbox"
	"rosemary-virsree/internal/service"
	"rosemary-virsree/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("database failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	box, err := secretbox.New(cfg.MasterKey)
	if err != nil {
		slog.Error("secret encryption failed", "error", err)
		os.Exit(1)
	}
	svc, err := service.New(db, provider.New(cfg.Backend), box, cfg)
	if err != nil {
		slog.Error("storage sources failed", "error", err)
		os.Exit(1)
	}
	server := &http.Server{Addr: cfg.Listen, Handler: httpapi.New(svc).Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	go func() {
		slog.Info("VirSree gateway ready", "listen", cfg.Listen, "public_url", cfg.PublicURL, "backend_ready", cfg.BackendReady(), "version", buildinfo.NormalizedVersion(), "commit", buildinfo.Commit)
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			slog.Error("server stopped", "error", e)
			os.Exit(1)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
