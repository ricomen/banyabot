CREATE TABLE IF NOT EXISTS weeks (
  week_id     TEXT PRIMARY KEY,
  chat_id     INTEGER NOT NULL,
  poll_id     TEXT NOT NULL UNIQUE,
  message_id  INTEGER NOT NULL,
  created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS votes (
  week_id     TEXT NOT NULL,
  user_id     INTEGER NOT NULL,
  username    TEXT,
  option_id   INTEGER NOT NULL,
  option_text TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  PRIMARY KEY (week_id, user_id),
  FOREIGN KEY (week_id) REFERENCES weeks(week_id)
);

CREATE TABLE IF NOT EXISTS facts_used (
  fact_id     INTEGER PRIMARY KEY,
  used_at     TEXT NOT NULL
);

DROP TABLE IF EXISTS sent_facts;

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
