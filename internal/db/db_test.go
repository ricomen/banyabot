package db

import (
	"testing"
	"time"
)

func TestWeekID(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		t.Fatal(err)
	}
	got := WeekID(time.Date(2026, 10, 13, 18, 0, 0, 0, loc)) // Tuesday
	if got != "2026-W42" {
		t.Fatalf("got %s", got)
	}
}

func TestVoteUpsert(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.InsertWeek(Week{
		WeekID: "2026-W42", ChatID: -100, PollID: "p1", MessageID: 1, CreatedAt: "now",
	}); err != nil {
		t.Fatal(err)
	}

	v := Vote{WeekID: "2026-W42", UserID: 7, Username: "vasya", OptionID: 0, OptionText: "Иду", UpdatedAt: "t1"}
	if err := s.UpsertVote(v); err != nil {
		t.Fatal(err)
	}
	v.OptionID = 1
	v.OptionText = "Не иду"
	v.UpdatedAt = "t2"
	if err := s.UpsertVote(v); err != nil {
		t.Fatal(err)
	}

	votes, err := s.VotesForWeek("2026-W42")
	if err != nil {
		t.Fatal(err)
	}
	if len(votes) != 1 {
		t.Fatalf("len=%d", len(votes))
	}
	if votes[0].OptionText != "Не иду" {
		t.Fatalf("got %s", votes[0].OptionText)
	}
}

func TestGetWeekByPollID(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.InsertWeek(Week{
		WeekID: "2026-W41", ChatID: -1, PollID: "abc", MessageID: 9, CreatedAt: "x",
	}); err != nil {
		t.Fatal(err)
	}
	w, err := s.GetWeekByPollID("abc")
	if err != nil {
		t.Fatal(err)
	}
	if w.WeekID != "2026-W41" {
		t.Fatalf("got %s", w.WeekID)
	}
}
