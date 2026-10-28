package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"banyabot/internal/banya"
	"banyabot/internal/config"
	"banyabot/internal/db"
)

type Runner struct {
	cron   *cron.Cron
	job    func()
	mu     sync.Mutex
	id     cron.EntryID
	timer  *time.Timer
	when   time.Time
	store  *db.Store
	remind func() error
}

func Start(ctx context.Context, cfg config.Config, ritual *banya.Ritual, spec string) (*Runner, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	c := cron.New(
		cron.WithLocation(cfg.Location),
		cron.WithParser(parser),
	)
	r := &Runner{
		cron:  c,
		store: ritual.Store,
		remind: func() error {
			return ritual.Remind(ctx)
		},
		job: func() {
			if err := ritual.Run(ctx); err != nil {
				slog.Error("weekly ritual failed", "err", err)
			}
		},
	}
	ritual.OnPosted = func(time.Time) { r.scheduleNext() }
	if spec != "" {
		if err := r.Set(spec); err != nil {
			return nil, err
		}
	}
	r.restore()
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

func (r *Runner) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.id != 0 {
		r.cron.Remove(r.id)
		r.id = 0
	}
}

func (r *Runner) Stop() {
	r.mu.Lock()
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.mu.Unlock()
	r.cron.Stop()
}

func (r *Runner) restore() {
	weeks, err := r.store.WeeksUnnagged()
	if err != nil {
		slog.Error("restore reminder", "err", err)
		return
	}
	now := time.Now()
	due := false
	for _, w := range weeks {
		at, ok := remindAt(w.CreatedAt)
		if !ok {
			continue
		}
		if !at.After(now) {
			due = true
			break
		}
	}
	if due {
		r.fire()
		return
	}
	r.scheduleNext()
}

func (r *Runner) fire() {
	if err := r.remind(); err != nil {
		slog.Error("vote reminder failed", "err", err)
	}
	r.scheduleNext()
}

func (r *Runner) scheduleNext() {
	weeks, err := r.store.WeeksUnnagged()
	if err != nil {
		slog.Error("schedule reminder", "err", err)
		return
	}
	var next time.Time
	now := time.Now()
	for _, w := range weeks {
		at, ok := remindAt(w.CreatedAt)
		if !ok || !at.After(now) {
			continue
		}
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	if next.IsZero() {
		return
	}
	r.arm(next)
}

func (r *Runner) arm(due time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timer != nil && !r.when.IsZero() && !due.Before(r.when) {
		return
	}
	if r.timer != nil {
		r.timer.Stop()
	}
	r.when = due
	delay := time.Until(due)
	if delay < 0 {
		delay = 0
	}
	r.timer = time.AfterFunc(delay, func() {
		r.mu.Lock()
		r.timer = nil
		r.when = time.Time{}
		r.mu.Unlock()
		r.fire()
	})
}

func remindAt(created string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		slog.Error("week created_at", "err", err, "value", created)
		return time.Time{}, false
	}
	return t.Add(24 * time.Hour), true
}
