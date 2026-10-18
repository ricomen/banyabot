package banya

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"instabot/internal/db"
)

type PollSender interface {
	SendFact(ctx context.Context, chatID int64, text string) error
	SendPoll(ctx context.Context, chatID int64, question string, options []string) (pollID string, messageID int, err error)
}

type Ritual struct {
	Store  *db.Store
	ChatID int64
	Loc    *time.Location
	Facts  []Fact
	RNG    *rand.Rand
	Sender PollSender
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

	used, err := r.Store.UsedFactIDs()
	if err != nil {
		return err
	}
	fact := PickFact(r.Facts, used, r.RNG)
	if len(used) >= len(r.Facts) {
		if err := r.Store.ClearFactsUsed(); err != nil {
			return err
		}
	}

	if err := r.Sender.SendFact(ctx, r.ChatID, WrapFact(fact.Text)); err != nil {
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
	return r.Store.MarkFactUsed(fact.ID, now.Format(time.RFC3339))
}
