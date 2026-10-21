package scheduler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"

	"banyabot/internal/banya"
	"banyabot/internal/config"
)

type Runner struct {
	cron *cron.Cron
	job  func()
	mu   sync.Mutex
	id   cron.EntryID
}

func Start(ctx context.Context, cfg config.Config, ritual *banya.Ritual, spec string) (*Runner, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	c := cron.New(
		cron.WithLocation(cfg.Location),
		cron.WithParser(parser),
	)
	r := &Runner{
		cron: c,
		job: func() {
			if err := ritual.Run(ctx); err != nil {
				slog.Error("weekly ritual failed", "err", err)
			}
		},
	}
	if err := r.Set(spec); err != nil {
		return nil, err
	}
	if _, err := c.AddFunc("0 * * * *", func() {
		if err := ritual.Remind(ctx); err != nil {
			slog.Error("vote reminder failed", "err", err)
		}
	}); err != nil {
		return nil, err
	}
	c.Start()
	return r, nil
}

func (r *Runner) Set(spec string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.id != 0 {
		r.cron.Remove(r.id)
		r.id = 0
	}
	id, err := r.cron.AddFunc(spec, r.job)
	if err != nil {
		return err
	}
	r.id = id
	return nil
}

func (r *Runner) Stop() {
	r.cron.Stop()
}
