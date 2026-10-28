package banya

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"testing"
	"time"

	"banyabot/internal/db"
)

type fakeSender struct {
	facts []string
	polls int
	texts []string
	shames []string
}

func (f *fakeSender) SendFact(ctx context.Context, chatID int64, text string) error {
	f.facts = append(f.facts, text)
	return nil
}

func (f *fakeSender) SendText(ctx context.Context, chatID int64, text string) error {
	f.texts = append(f.texts, text)
	return nil
}

func (f *fakeSender) SendShame(ctx context.Context, chatID int64, text string) error {
	f.shames = append(f.shames, text)
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
	if len(sender.facts) != 1 || sender.polls != 1 {
		t.Fatalf("sends fact=%v poll=%d", sender.facts, sender.polls)
	}
	err = r.Run(context.Background())
	if !errors.Is(err, ErrAlreadyPosted) {
		t.Fatalf("want ErrAlreadyPosted, got %v", err)
	}
}

type stubSource struct {
	text string
	err  error
}

func (s stubSource) Fact(ctx context.Context) (string, error) {
	return s.text, s.err
}

func TestRitualPrefersGemini(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loc, _ := time.LoadLocation("Asia/Almaty")
	sender := &fakeSender{}
	r := &Ritual{
		Store:  store,
		ChatID: -100,
		Loc:    loc,
		Facts:  []Fact{{ID: 1, Text: "из файла"}},
		RNG:    rand.New(rand.NewSource(1)),
		Sender: sender,
		Source: stubSource{text: "из поиска"},
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.facts) != 1 || sender.facts[0] != "из поиска" {
		t.Fatalf("sent %v", sender.facts)
	}
}

func TestPostFactDoesNotOpenPoll(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	loc, _ := time.LoadLocation("Asia/Almaty")
	sender := &fakeSender{}
	r := &Ritual{
		Store:  store,
		ChatID: -100,
		Loc:    loc,
		Facts:  []Fact{{ID: 1, Text: "из файла"}},
		RNG:    rand.New(rand.NewSource(1)),
		Sender: sender,
		Source: stubSource{text: "просто факт"},
	}
	if err := r.PostFact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.facts) != 1 || sender.polls != 0 {
		t.Fatalf("fact=%v poll=%d", sender.facts, sender.polls)
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRemindShamesEmptyPoll(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	old := time.Now().UTC().Add(-25 * time.Hour).Format(time.RFC3339)
	if err := store.InsertWeek(db.Week{
		WeekID: "2026-W01", ChatID: -100, PollID: "p", MessageID: 1, CreatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{}
	r := &Ritual{Store: store, Loc: time.UTC, Sender: sender, Taunt: stubTaunt{text: "молчуны"}}
	if err := r.Remind(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.texts) != 1 || sender.texts[0] != "молчуны" {
		t.Fatalf("%v", sender.texts)
	}
	if err := r.Remind(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.texts) != 1 {
		t.Fatalf("reminded twice: %v", sender.texts)
	}
}

func TestRemindShamesKnownNonVoters(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	old := time.Now().UTC().Add(-25 * time.Hour).Format(time.RFC3339)
	if err := store.InsertWeek(db.Week{
		WeekID: "2026-W02", ChatID: -100, PollID: "p", MessageID: 1, CreatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertVote(db.Vote{
		WeekID: "2026-W02", UserID: 1, Username: "@clean", OptionID: 0, OptionText: "Иду", UpdatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []db.Member{
		{ChatID: -100, UserID: 1, Name: "@clean"},
		{ChatID: -100, UserID: 2, Name: "@dirty"},
	} {
		if err := store.UpsertMember(m); err != nil {
			t.Fatal(err)
		}
	}
	sender := &fakeSender{}
	r := &Ritual{Store: store, Loc: time.UTC, Sender: sender}
	if err := r.Remind(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.shames) != 1 || len(sender.texts) != 0 {
		t.Fatalf("shames=%v texts=%v", sender.shames, sender.texts)
	}
	if !strings.Contains(sender.shames[0], "@dirty") || strings.Contains(sender.shames[0], "@clean") {
		t.Fatalf("%s", sender.shames[0])
	}
}

type stubTaunt struct{ text string }

func (s stubTaunt) Taunt(ctx context.Context) (string, error) { return s.text, nil }
