package main

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"math/big"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	minIDLen = 8
	maxIDLen = 16
	// A link is touched at most once a day on read, so a hot link costs no
	// writes and an unused one still ages out.
	touchEvery = 24 * time.Hour
)

// ErrFull is returned when a new share would push stored tokens past the cap.
var ErrFull = errors.New("share storage is full")

// Store keeps share tokens (the client's compressed scenario, opaque here)
// keyed by a content-derived ID.
type Store struct {
	db  *sql.DB
	now func() time.Time

	mu       sync.Mutex // guards bytes and serializes the size check with the insert
	bytes    int64      // total length of stored tokens
	maxBytes int64      // 0 = unlimited
}

func OpenStore(path string, maxBytes int64) (*Store, error) {
	// WAL + a busy timeout: single writer, many readers, no "database is locked".
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS shares (
		id         TEXT PRIMARY KEY,
		token      TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		last_hit   INTEGER NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	st := &Store{db: db, now: time.Now, maxBytes: maxBytes}
	if err := st.recount(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) recount() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.QueryRow(`SELECT COALESCE(SUM(length(token)), 0) FROM shares`).Scan(&s.bytes)
}

func (s *Store) Close() error { return s.db.Close() }

// idFor derives the base62 ID of a token, n characters long. The same token
// always maps to the same ID, so re-sharing a scenario returns the same link
// and an existing link's content can never change.
func idFor(token string, n int) string {
	sum := sha256.Sum256([]byte(token))
	return new(big.Int).SetBytes(sum[:]).Text(62)[:n]
}

// Put stores token and returns its ID, extending the ID on the (astronomically
// unlikely) event of a prefix collision with different content.
func (s *Store) Put(token string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	for n := minIDLen; n <= maxIDLen; n++ {
		id := idFor(token, n)
		var existing string
		err := s.db.QueryRow(`SELECT token FROM shares WHERE id = ?`, id).Scan(&existing)
		switch {
		case err == nil && existing == token:
			_, err = s.db.Exec(`UPDATE shares SET last_hit = ? WHERE id = ?`, now, id)
			return id, err
		case err == nil:
			continue // collision: try a longer ID
		case errors.Is(err, sql.ErrNoRows):
			if s.maxBytes > 0 && s.bytes+int64(len(token)) > s.maxBytes {
				return "", ErrFull
			}
			_, err = s.db.Exec(`INSERT INTO shares (id, token, created_at, last_hit) VALUES (?, ?, ?, ?)`, id, token, now, now)
			if err == nil {
				s.bytes += int64(len(token))
			}
			return id, err
		default:
			return "", err
		}
	}
	return "", errors.New("share id space exhausted")
}

// Get returns the token for id, or ("", false) when unknown.
func (s *Store) Get(id string) (string, bool, error) {
	var token string
	var last int64
	err := s.db.QueryRow(`SELECT token, last_hit FROM shares WHERE id = ?`, id).Scan(&token, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if now := s.now(); now.Sub(time.Unix(last, 0)) > touchEvery {
		if _, err := s.db.Exec(`UPDATE shares SET last_hit = ? WHERE id = ?`, now.Unix(), id); err != nil {
			return "", false, err
		}
	}
	return token, true, nil
}

// Backup writes a consistent snapshot of the database to path (which must not
// exist) using VACUUM INTO — safe while the server is running, unlike copying
// a WAL-mode file.
func (s *Store) Backup(path string) error {
	_, err := s.db.Exec(`VACUUM INTO ?`, path)
	return err
}

// Purge deletes shares not read (or re-shared) within maxAge.
func (s *Store) Purge(maxAge time.Duration) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM shares WHERE last_hit < ?`, s.now().Add(-maxAge).Unix())
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if n > 0 {
		err = s.recount()
	}
	return n, err
}
