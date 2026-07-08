// Package store is Chronos's SQLite-backed storage layer. It follows the
// same shape as the CloudOlympus oracle's store: a pure-Go modernc.org/sqlite
// driver (so CGO_ENABLED=0 builds keep working), WAL journalling, a single
// writer connection, and an idempotent CREATE-IF-NOT-EXISTS schema.
//
// Every domain row carries an owner_id. Today there is exactly one seeded
// user (SeedOwner) and the API hands that id to every call, so the app is
// effectively single-user. Turning on real accounts later means adding a
// login flow and resolving a real owner_id — no schema change.
package store

import (
	"database/sql"
	"errors"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

// SeedOwner is the id of the single user created on first run. The API's
// currentOwner shim returns this until real authentication exists.
const SeedOwner int64 = 1

// ErrTimerRunning is returned by StartTimer when a timer is already running
// for the owner (enforced by a unique partial index, so it is race-free).
var ErrTimerRunning = errors.New("a timer is already running")

// ErrNotFound is returned when a lookup by id matches no row for the owner.
var ErrNotFound = errors.New("not found")

// ErrInUse is returned when deleting something that other rows still
// reference (e.g. a work type used by time entries).
var ErrInUse = errors.New("in use")

// Store owns the database handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path, applies the schema,
// and seeds first-run defaults.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// Single writer avoids lock churn on SQLite, exactly as the oracle does.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if _, err := db.Exec(schema); err != nil {
		if cerr := db.Close(); cerr != nil {
			log.Printf("sqlite close after schema failure failed: %v", cerr)
		}
		return nil, err
	}
	if err := s.seed(); err != nil {
		if cerr := db.Close(); cerr != nil {
			log.Printf("sqlite close after seed failure failed: %v", cerr)
		}
		return nil, err
	}
	// Rebuild the FTS indexes from content tables — idempotent, cheap at this
	// scale, and it backfills rows written before FTS existed.
	for _, t := range []string{"entries_fts", "templates_fts"} {
		if _, err := db.Exec(`INSERT INTO ` + t + `(` + t + `) VALUES ('rebuild')`); err != nil {
			log.Printf("fts rebuild %s failed: %v", t, err)
		}
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  email      TEXT    NOT NULL DEFAULT '',
  name       TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS clients (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id    INTEGER NOT NULL,
  name        TEXT    NOT NULL,
  code        TEXT    NOT NULL DEFAULT '',
  notes       TEXT    NOT NULL DEFAULT '',
  status      TEXT    NOT NULL DEFAULT 'active', -- active | archived
  created_at  INTEGER NOT NULL,
  archived_at INTEGER
);
CREATE INDEX IF NOT EXISTS clients_owner ON clients (owner_id, status);

-- Optional grouping: a reorg, a T2 engagement, a CRA dispute. Entries and
-- todos may point at a matter or just a client. Schema-ready for later UI.
CREATE TABLE IF NOT EXISTS matters (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id  INTEGER NOT NULL,
  client_id INTEGER NOT NULL,
  name      TEXT    NOT NULL,
  kind      TEXT    NOT NULL DEFAULT '',
  status    TEXT    NOT NULL DEFAULT 'open', -- open | closed
  opened_at INTEGER NOT NULL,
  closed_at INTEGER
);
CREATE INDEX IF NOT EXISTS matters_owner ON matters (owner_id, client_id);

CREATE TABLE IF NOT EXISTS work_types (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id         INTEGER NOT NULL,
  name             TEXT    NOT NULL,
  category         TEXT    NOT NULL DEFAULT '',
  billable_default INTEGER NOT NULL DEFAULT 1,
  sort             INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS work_types_owner ON work_types (owner_id);

CREATE TABLE IF NOT EXISTS time_entries (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id     INTEGER NOT NULL,
  client_id    INTEGER,
  matter_id    INTEGER,
  work_type_id INTEGER,
  description  TEXT    NOT NULL DEFAULT '',
  started_at   INTEGER NOT NULL,       -- unix seconds
  ended_at     INTEGER,                -- NULL while a timer runs
  duration_min INTEGER,                -- NULL while a timer runs
  billable     INTEGER NOT NULL DEFAULT 1,
  source       TEXT    NOT NULL DEFAULT 'manual', -- timer | manual | import
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS entries_owner_started ON time_entries (owner_id, started_at);
-- At most one running timer per owner, enforced at the storage layer.
CREATE UNIQUE INDEX IF NOT EXISTS one_running_timer
  ON time_entries (owner_id) WHERE ended_at IS NULL;

CREATE TABLE IF NOT EXISTS todos (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id     INTEGER NOT NULL,
  client_id    INTEGER,
  matter_id    INTEGER,
  title        TEXT    NOT NULL,
  detail       TEXT    NOT NULL DEFAULT '',
  due_date     TEXT,                   -- YYYY-MM-DD
  priority     INTEGER NOT NULL DEFAULT 0,
  status       TEXT    NOT NULL DEFAULT 'open', -- open | done
  created_at   INTEGER NOT NULL,
  completed_at INTEGER
);
CREATE INDEX IF NOT EXISTS todos_owner ON todos (owner_id, status);

CREATE TABLE IF NOT EXISTS templates (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id        INTEGER NOT NULL,
  work_type_id    INTEGER,
  title           TEXT    NOT NULL,
  body            TEXT    NOT NULL DEFAULT '',
  tags            TEXT    NOT NULL DEFAULT '',
  source_entry_id INTEGER,
  created_at      INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS templates_owner ON templates (owner_id);

CREATE TABLE IF NOT EXISTS settings (
  owner_id INTEGER NOT NULL,
  k        TEXT    NOT NULL,
  v        TEXT    NOT NULL,
  PRIMARY KEY (owner_id, k)
);

-- Full-text search (external-content FTS5, kept in sync by triggers) over
-- entry descriptions and templates. This is what makes past work findable.
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
  description, content='time_entries', content_rowid='id');
CREATE TRIGGER IF NOT EXISTS entries_fts_i AFTER INSERT ON time_entries BEGIN
  INSERT INTO entries_fts(rowid, description) VALUES (new.id, new.description);
END;
CREATE TRIGGER IF NOT EXISTS entries_fts_d AFTER DELETE ON time_entries BEGIN
  INSERT INTO entries_fts(entries_fts, rowid, description) VALUES ('delete', old.id, old.description);
END;
CREATE TRIGGER IF NOT EXISTS entries_fts_u AFTER UPDATE ON time_entries BEGIN
  INSERT INTO entries_fts(entries_fts, rowid, description) VALUES ('delete', old.id, old.description);
  INSERT INTO entries_fts(rowid, description) VALUES (new.id, new.description);
END;

CREATE VIRTUAL TABLE IF NOT EXISTS templates_fts USING fts5(
  title, body, tags, content='templates', content_rowid='id');
CREATE TRIGGER IF NOT EXISTS templates_fts_i AFTER INSERT ON templates BEGIN
  INSERT INTO templates_fts(rowid, title, body, tags) VALUES (new.id, new.title, new.body, new.tags);
END;
CREATE TRIGGER IF NOT EXISTS templates_fts_d AFTER DELETE ON templates BEGIN
  INSERT INTO templates_fts(templates_fts, rowid, title, body, tags) VALUES ('delete', old.id, old.title, old.body, old.tags);
END;
CREATE TRIGGER IF NOT EXISTS templates_fts_u AFTER UPDATE ON templates BEGIN
  INSERT INTO templates_fts(templates_fts, rowid, title, body, tags) VALUES ('delete', old.id, old.title, old.body, old.tags);
  INSERT INTO templates_fts(rowid, title, body, tags) VALUES (new.id, new.title, new.body, new.tags);
END;`

// seed creates the single user, a default tax work-type catalog, and the
// default settings the first time the database is empty. It is idempotent.
func (s *Store) seed() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	now := time.Now().Unix()
	if _, err := s.db.Exec(
		`INSERT INTO users (id, email, name, created_at) VALUES (?, '', 'me', ?)`,
		SeedOwner, now); err != nil {
		return err
	}

	// A starter catalog aimed at a tax practitioner's day. billable=0 for the
	// two admin categories. Users can add/rename these later.
	type wt struct {
		name     string
		category string
		billable bool
	}
	catalog := []wt{
		{"CRA correspondence", "compliance", true},
		{"T1 personal", "compliance", true},
		{"T2 corporate", "compliance", true},
		{"Reorganization", "planning", true},
		{"Reorg step letter", "planning", true},
		{"Form filing", "compliance", true},
		{"Bookkeeping", "compliance", true},
		{"Tax research", "planning", true},
		{"Client meeting", "advisory", true},
		{"Emails & calls", "advisory", true},
		{"General", "internal", false},
		{"Admin", "internal", false},
		{"Professional development", "internal", false},
	}
	for i, w := range catalog {
		b := 0
		if w.billable {
			b = 1
		}
		if _, err := s.db.Exec(
			`INSERT INTO work_types (owner_id, name, category, billable_default, sort)
			 VALUES (?, ?, ?, ?, ?)`,
			SeedOwner, w.name, w.category, b, i); err != nil {
			return err
		}
	}

	defaults := map[string]string{
		"busy_season_target_min": "480", // Jan–Apr: 8h coded per day
		"off_season_target_min":  "450", // May–Dec: 7.5h coded per day
		"rounding_min":           "6",   // firms bill in 0.1h (6 min) increments
		"stale_days":             "14",  // todo age before it counts as stale
		"due_soon_days":          "2",   // due within this window = "due soon"
		"timezone":               "",    // "" = server local time
	}
	for k, v := range defaults {
		if _, err := s.db.Exec(
			`INSERT INTO settings (owner_id, k, v) VALUES (?, ?, ?)`,
			SeedOwner, k, v); err != nil {
			return err
		}
	}
	return nil
}
