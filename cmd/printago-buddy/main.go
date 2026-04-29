// Package main is the entry point for the printago-buddy daemon.
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/robfig/cron/v3"

	"github.com/bandesz/printago-buddy/internal/config"
	"github.com/bandesz/printago-buddy/internal/jobs"
	"github.com/bandesz/printago-buddy/internal/printago"
	"github.com/bandesz/printago-buddy/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	client := printago.NewClient(cfg.APIKey, cfg.StoreID)
	tagger := jobs.NewFilamentTaggerJob(client)

	c := cron.New()
	// Run every minute.
	_, err = c.AddJob("* * * * *", tagger)
	if err != nil {
		slog.Error("failed to register cron job", "error", err)
		os.Exit(1)
	}
	c.Start()

	webSrv, err := web.NewServer(client, cfg.WebPort)
	if err != nil {
		slog.Error("failed to create web server", "error", err)
		os.Exit(1)
	}
	go func() {
		if err := webSrv.Start(); err != nil {
			slog.Error("web server error", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("printago-buddy started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	// Stop blocks until all running jobs finish.
	<-c.Stop().Done()
	slog.Info("stopped")
}
