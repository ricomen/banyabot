package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"instabot/migrations"

	_ "modernc.org/sqlite"
)

type Store struct {
	SQL *sql.DB
}

type Week struct {
	WeekID    string
	ChatID    int64
	PollID    string
	MessageID int
	CreatedAt string
}

type Vote struct {
	WeekID     string
	UserID     int64
	Username   string
	OptionID   int
	OptionText string
	UpdatedAt  string
}

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir data: %w", err)
		}
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	s := &Store{SQL: sqlDB}
	if err := s.Migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.SQL.Close()
}

func (s *Store) Migrate() error {
	sqlBytes, err := migrations.FS.ReadFile("001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if _, err := s.SQL.Exec(string(sqlBytes)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	return nil
}

func WeekID(t time.Time) string {
	year, week := t.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

func (s *Store) InsertWeek(w Week) error {
	_, err := s.SQL.Exec(
		`INSERT INTO weeks (week_id, chat_id, poll_id, message_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		w.WeekID, w.ChatID, w.PollID, w.MessageID, w.CreatedAt,
	)
	return err
}

func (s *Store) GetWeek(weekID string, chatID int64) (Week, error) {
	var w Week
	err := s.SQL.QueryRow(
		`SELECT week_id, chat_id, poll_id, message_id, created_at FROM weeks WHERE week_id = ? AND chat_id = ?`,
		weekID, chatID,
	).Scan(&w.WeekID, &w.ChatID, &w.PollID, &w.MessageID, &w.CreatedAt)
	return w, err
}

func (s *Store) GetWeekByPollID(pollID string) (Week, error) {
	var w Week
	err := s.SQL.QueryRow(
		`SELECT week_id, chat_id, poll_id, message_id, created_at FROM weeks WHERE poll_id = ?`,
		pollID,
	).Scan(&w.WeekID, &w.ChatID, &w.PollID, &w.MessageID, &w.CreatedAt)
	return w, err
}

func (s *Store) LatestWeekBefore(chatID int64, weekID string) (Week, error) {
	var w Week
	err := s.SQL.QueryRow(
		`SELECT week_id, chat_id, poll_id, message_id, created_at FROM weeks
		 WHERE chat_id = ? AND week_id < ? ORDER BY week_id DESC LIMIT 1`,
		chatID, weekID,
	).Scan(&w.WeekID, &w.ChatID, &w.PollID, &w.MessageID, &w.CreatedAt)
	return w, err
}

func (s *Store) UpsertVote(v Vote) error {
	_, err := s.SQL.Exec(
		`INSERT INTO votes (week_id, user_id, username, option_id, option_text, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(week_id, user_id) DO UPDATE SET
		   username = excluded.username,
		   option_id = excluded.option_id,
		   option_text = excluded.option_text,
		   updated_at = excluded.updated_at`,
		v.WeekID, v.UserID, v.Username, v.OptionID, v.OptionText, v.UpdatedAt,
	)
	return err
}

func (s *Store) DeleteVote(weekID string, userID int64) error {
	_, err := s.SQL.Exec(`DELETE FROM votes WHERE week_id = ? AND user_id = ?`, weekID, userID)
	return err
}

func (s *Store) VotesForWeek(weekID string) ([]Vote, error) {
	rows, err := s.SQL.Query(
		`SELECT week_id, user_id, username, option_id, option_text, updated_at
		 FROM votes WHERE week_id = ? ORDER BY option_id, username`,
		weekID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Vote
	for rows.Next() {
		var v Vote
		if err := rows.Scan(&v.WeekID, &v.UserID, &v.Username, &v.OptionID, &v.OptionText, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) UsedFactIDs() (map[int]struct{}, error) {
	rows, err := s.SQL.Query(`SELECT fact_id FROM facts_used`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]struct{})
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (s *Store) MarkFactUsed(factID int, usedAt string) error {
	_, err := s.SQL.Exec(
		`INSERT INTO facts_used (fact_id, used_at) VALUES (?, ?)
		 ON CONFLICT(fact_id) DO UPDATE SET used_at = excluded.used_at`,
		factID, usedAt,
	)
	return err
}

func (s *Store) ClearFactsUsed() error {
	_, err := s.SQL.Exec(`DELETE FROM facts_used`)
	return err
}

func (s *Store) RecentFactTexts(limit int) ([]string, error) {
	rows, err := s.SQL.Query(
		`SELECT text FROM sent_facts ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		out = append(out, text)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *Store) SaveSentFact(text, usedAt string) error {
	_, err := s.SQL.Exec(`INSERT INTO sent_facts (text, used_at) VALUES (?, ?)`, text, usedAt)
	return err
}
