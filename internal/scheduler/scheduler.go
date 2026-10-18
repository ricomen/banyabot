package scheduler

import (
	"context"
	"log/slog"

	"github.com/robfig/cron/v3"

	"instabot/internal/banya"
	"instabot/internal/config"
)

func Start(ctx context.Context, cfg config.Config, ritual *banya.Ritual) (*cron.Cron, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	c := cron.New(
		cron.WithLocation(cfg.Location),
		cron.WithParser(parser),
	)
	_, err := c.AddFunc(cfg.CronSpec, func() {
		if err := ritual.Run(ctx); err != nil {
			slog.Error("weekly ritual failed", "err", err)
		}
	})
	if err != nil {
		return nil, err
	}
	c.Start()
	return c, nil
}
