package banya

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"instabot/internal/db"
)

type fakeSender struct {
	facts int
	polls int
}

func (f *fakeSender) SendFact(ctx context.Context, chatID int64, text string) error {
	f.facts++
	return nil
}

func (f *fakeSender) SendPoll(ctx context.Context, chatID int64, question string, options []string) (string, int, error) {
	f.polls++
	return "poll-1", 42, nil
}

func TestRitualOncePerWeek(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{}
	r := &Ritual{
		Store:  store,
		ChatID: -100,
		Loc:    loc,
		Facts:  []Fact{{ID: 1, Text: "пар"}},
		RNG:    rand.New(rand.NewSource(1)),
		Sender: sender,
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sender.facts != 1 || sender.polls != 1 {
		t.Fatalf("sends fact=%d poll=%d", sender.facts, sender.polls)
	}
	err = r.Run(context.Background())
	if !errors.Is(err, ErrAlreadyPosted) {
		t.Fatalf("want ErrAlreadyPosted, got %v", err)
	}
}
