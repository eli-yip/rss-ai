package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"

	"github.com/eli-yip/rss-ai/pkg/config"
	"github.com/eli-yip/rss-ai/pkg/gateway"
	"github.com/eli-yip/rss-ai/pkg/handler"
	"github.com/eli-yip/rss-ai/pkg/mlog"
	"github.com/eli-yip/rss-ai/pkg/proxy"
	"github.com/eli-yip/rss-ai/pkg/server"
	"github.com/eli-yip/rss-ai/pkg/store"
)

func main() {
	app := &cli.Command{
		Name:  "rss-ai",
		Usage: "AI title-rewriting gateway in front of RSSHub",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "config.toml",
				Usage:   "path to config file",
			},
		},
		Action: run,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, c *cli.Command) error {
	if err := config.Init(c.String("config")); err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	cfg := config.C

	level, err := mlog.LogLevelFromString(cfg.Log.Level)
	if err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}
	logger := mlog.New(cfg.App.Env, level)
	logger.Info("starting rss-ai", zap.String("addr", cfg.Server.Addr))

	st, err := store.New(cfg.DB.DSN)
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}

	px, err := proxy.New(cfg.Upstream.BaseURL, logger)
	if err != nil {
		return fmt.Errorf("init proxy: %w", err)
	}
	gw := gateway.New(handler.NewResolver(cfg.Handlers), px, logger)

	srv := server.New(cfg.Server.Addr, st, logger)
	// Catch-all for every non-health path; static routes (/healthz, /readyz)
	// take precedence over the wildcard.
	srv.Echo().Any("/*", gw.Handle)
	if err := srv.Start(); err != nil {
		return fmt.Errorf("start server: %w", err)
	}

	<-ctx.Done()
	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Stop(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
	logger.Info("bye")
	return nil
}
