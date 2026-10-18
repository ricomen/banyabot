package main

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"instabot/internal/banya"
	"instabot/internal/config"
	"instabot/internal/db"
	"instabot/internal/gemini"
	"instabot/internal/scheduler"
	"instabot/internal/telegram"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	store, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("db", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	facts, err := banya.LoadFacts(cfg.FactsPath, banya.DefaultFactsJSON)
	if err != nil {
		slog.Error("facts", "err", err)
		os.Exit(1)
	}

	ritual := &banya.Ritual{
		Store:  store,
		ChatID: cfg.BanyaChatID,
		Loc:    cfg.Location,
		Facts:  facts,
		RNG:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if cfg.GeminiAPIKey != "" {
		ritual.Source = &gemini.Client{APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel}
		slog.Info("факты из gemini", "model", cfg.GeminiModel)
	}

	tg, err := telegram.New(cfg, store, ritual)
	if err != nil {
		slog.Error("telegram", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cr, err := scheduler.Start(ctx, cfg, ritual)
	if err != nil {
		slog.Error("cron", "err", err)
		os.Exit(1)
	}
	defer cr.Stop()

	slog.Info("баньщик на посту", "tz", cfg.Location.String(), "cron", cfg.CronSpec, "chat", cfg.BanyaChatID)
	tg.Start(ctx)
	slog.Info("stopped")
}
