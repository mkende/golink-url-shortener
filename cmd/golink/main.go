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

	"github.com/mkende/golink-url-shortener/internal/auth"
	"github.com/mkende/golink-url-shortener/internal/config"
	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/internal/server"
)

func main() {
	configPath := flag.String("config", "", "path to configuration file (overrides $GOLINK_CONFIG)")
	flag.Parse()

	path := *configPath
	if path == "" {
		path = os.Getenv("GOLINK_CONFIG")
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "error: config file must be specified via -config flag or GOLINK_CONFIG environment variable")
		os.Exit(1)
	}

	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	// structured logger
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	ctx := context.Background()
	sqlDB, err := db.Open(ctx, cfg.DB.Driver, cfg.DB.DSN)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	// Initialise OIDC handler only when OIDC is enabled.
	var oidcHandler *auth.OIDCHandler
	if cfg.OIDC.Enabled {
		userRepo := db.NewUserRepo(sqlDB)
		oidcHandler, err = auth.NewOIDCHandler(ctx, cfg, userRepo)
		if err != nil {
			logger.Error("failed to initialise OIDC provider", "error", err)
			os.Exit(1)
		}
	}

	srv := server.New(cfg, sqlDB, logger, oidcHandler)

	httpSrv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx) //nolint:errcheck
		// Flush any buffered use-count increments before exiting.
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("use-counter shutdown error", "error", err)
		}
		close(done)
	}()

	logger.Info("starting server", "addr", cfg.ListenAddr)
	if cfg.CanonicalAddress != "" {
		logger.Info("ready at: " + cfg.CanonicalAddress)
	}
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	<-done
}
