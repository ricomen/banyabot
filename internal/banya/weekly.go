package banya

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"banyabot/internal/db"
)

type PollSender interface {
	SendFact(ctx context.Context, chatID int64, text string) error
	SendPoll(ctx context.Context, chatID int64, question string, options []string) (pollID string, messageID int, err error)
	SendText(ctx context.Context, chatID int64, text string) error
}

type FactSource interface {
	Fact(ctx context.Context) (string, error)
}

type Taunter interface {
	Taunt(ctx context.Context) (string, error)
}

type Ritual struct {
	Store  *db.Store
	ChatID int64
	Loc    *time.Location
	Facts  []Fact
	RNG    *rand.Rand
	Sender PollSender
	Source FactSource
	Taunt  Taunter
}

var ErrAlreadyPosted = errors.New("poll already posted for this week")

func (r *Ritual) Run(ctx context.Context) error {
	now := time.Now().In(r.Loc)
	weekID := db.WeekID(now)

	_, err := r.Store.GetWeek(weekID, r.ChatID)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrAlreadyPosted, weekID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	text, factID, err := r.nextFact(ctx)
	if err != nil {
		return err
	}

	if err := r.Sender.SendFact(ctx, r.ChatID, text); err != nil {
		return fmt.Errorf("send fact: %w", err)
	}

	pollID, msgID, err := r.Sender.SendPoll(ctx, r.ChatID, PollQuestion(), Options())
	if err != nil {
		return fmt.Errorf("send poll: %w", err)
	}

	if err := r.Store.InsertWeek(db.Week{
		WeekID:    weekID,
		ChatID:    r.ChatID,
		PollID:    pollID,
		MessageID: msgID,
		CreatedAt: now.Format(time.RFC3339),
	}); err != nil {
		return err
	}
	if factID == 0 {
		return nil
	}
	return r.Store.MarkFactUsed(factID, now.Format(time.RFC3339))
}

func (r *Ritual) Remind(ctx context.Context) error {
	if r.Loc == nil {
		r.Loc = time.UTC
	}
	cutoff := time.Now().In(r.Loc).Add(-24 * time.Hour).Format(time.RFC3339)
	weeks, err := r.Store.WeeksQuietSince(cutoff)
	if err != nil {
		return err
	}
	for _, week := range weeks {
		text := NobodyVoted()
		if r.Taunt != nil {
			if line, err := r.Taunt.Taunt(ctx); err == nil && line != "" {
				text = line
			} else if err != nil {
				slog.Warn("gemini taunt failed", "err", err)
			}
		}
		if err := r.Sender.SendText(ctx, week.ChatID, text); err != nil {
			return err
		}
		if err := r.Store.SetSetting("nagged:"+week.WeekID, cutoff); err != nil {
			return err
		}
	}
	return nil
}

func (r *Ritual) PostFact(ctx context.Context) error {
	text, factID, err := r.nextFact(ctx)
	if err != nil {
		return err
	}
	if err := r.Sender.SendFact(ctx, r.ChatID, text); err != nil {
		return fmt.Errorf("send fact: %w", err)
	}
	if factID == 0 {
		return nil
	}
	return r.Store.MarkFactUsed(factID, time.Now().In(r.Loc).Format(time.RFC3339))
}

func (r *Ritual) nextFact(ctx context.Context) (string, int, error) {
	if r.Source != nil {
		text, err := r.Source.Fact(ctx)
		if err == nil && text != "" {
			return text, 0, nil
		}
		slog.Warn("gemini fact failed, using facts.json", "err", err)
	}

	used, err := r.Store.UsedFactIDs()
	if err != nil {
		return "", 0, err
	}
	fact := PickFact(r.Facts, used, r.RNG)
	if len(used) >= len(r.Facts) {
		if err := r.Store.ClearFactsUsed(); err != nil {
			return "", 0, err
		}
	}
	return fact.Text, fact.ID, nil
}
