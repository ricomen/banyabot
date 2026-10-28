package banya

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"banyabot/internal/db"
)

type PollSender interface {
	SendFact(ctx context.Context, chatID int64, text string) error
	SendPoll(ctx context.Context, chatID int64, question string, options []string) (pollID string, messageID int, err error)
	SendText(ctx context.Context, chatID int64, text string) error
	SendShame(ctx context.Context, chatID int64, fileID, text string) error
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
	// OnPosted ставит напоминание через сутки после опроса.
	OnPosted func(at time.Time)
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

	pollID, msgID, err := r.Sender.SendPoll(ctx, r.ChatID, r.Question(), Options())
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
	if r.OnPosted != nil {
		r.OnPosted(now)
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
	cutoff := time.Now().In(r.Loc).Add(-24*time.Hour + time.Second).Format(time.RFC3339)
	weeks, err := r.Store.WeeksUnnaggedSince(cutoff)
	if err != nil {
		return err
	}
	for _, week := range weeks {
		if err := r.shame(ctx, week, cutoff); err != nil {
			return err
		}
	}
	return nil
}

func (r *Ritual) shame(ctx context.Context, week db.Week, cutoff string) error {
	dirty, votes, err := r.absent(week.ChatID, week.WeekID)
	if err != nil {
		return err
	}
	switch {
	case len(dirty) > 0:
		fileID, caption := r.PickOffense()
		if err := r.Sender.SendShame(ctx, week.ChatID, fileID, ShameText(caption, dirty)); err != nil {
			return err
		}
	case len(votes) == 0:
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
	}
	return r.Store.SetSetting("nagged:"+week.WeekID, cutoff)
}

func (r *Ritual) Question() string {
	if r.Store != nil {
		if q, ok, err := r.Store.Setting("poll_question"); err == nil && ok && strings.TrimSpace(q) != "" {
			return strings.TrimSpace(q)
		}
	}
	return PollQuestion()
}

func (r *Ritual) PickOffense() (fileID, caption string) {
	if r.Store == nil {
		return "", ""
	}
	list, err := r.Store.Offenses()
	if err != nil || len(list) == 0 {
		return "", ""
	}
	n := 0
	if r.RNG != nil {
		n = r.RNG.Intn(len(list))
	}
	return list[n].FileID, list[n].Caption
}

func (r *Ritual) Absent(chatID int64, weekID string) ([]string, error) {
	names, _, err := r.absent(chatID, weekID)
	return names, err
}

func (r *Ritual) absent(chatID int64, weekID string) ([]string, []db.Vote, error) {
	members, err := r.Store.Members(chatID)
	if err != nil {
		return nil, nil, err
	}
	votes, err := r.Store.VotesForWeek(weekID)
	if err != nil {
		return nil, nil, err
	}
	voted := make(map[int64]struct{}, len(votes))
	for _, v := range votes {
		voted[v.UserID] = struct{}{}
	}
	var dirty []string
	for _, m := range members {
		if _, ok := voted[m.UserID]; ok {
			continue
		}
		dirty = append(dirty, m.Name)
	}
	return dirty, votes, nil
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
