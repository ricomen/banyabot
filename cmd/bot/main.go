package main

import (
	"context"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"banyabot/internal/banya"
	"banyabot/internal/config"
	"banyabot/internal/db"
	"banyabot/internal/gemini"
	"banyabot/internal/scheduler"
	"banyabot/internal/telegram"
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
		client := &gemini.Client{APIKey: cfg.GeminiAPIKey, Model: cfg.GeminiModel}
		ritual.Source = client
		ritual.Taunt = client
		slog.Info("факты из gemini", "model", cfg.GeminiModel)
	}
	cronSpec := cfg.CronSpec
	if spec, ok, err := store.Setting("cron_spec"); err != nil {
		slog.Error("settings", "err", err)
		os.Exit(1)
	} else if ok && spec != "" {
		cronSpec = spec
		cfg.CronSpec = spec
	}
	if raw, ok, err := store.Setting("chat_id"); err != nil {
		slog.Error("settings", "err", err)
		os.Exit(1)
	} else if ok && raw != "" {
		chatID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			slog.Error("settings chat_id", "err", err)
			os.Exit(1)
		}
		cfg.BanyaChatID = chatID
		ritual.ChatID = chatID
	}

	tg, err := telegram.New(cfg, store, ritual)
	if err != nil {
		slog.Error("telegram", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cr, err := scheduler.Start(ctx, cfg, ritual, cronSpec)
	if err != nil {
		slog.Error("cron", "err", err)
		os.Exit(1)
	}
	defer cr.Stop()
	tg.Sched = cr

	slog.Info("баньщик на посту", "tz", cfg.Location.String(), "cron", cronSpec, "chat", cfg.BanyaChatID)
	tg.Start(ctx)
	slog.Info("stopped")
}
