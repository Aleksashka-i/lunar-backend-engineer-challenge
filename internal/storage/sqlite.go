// Package storage implements rocket.Store.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
)

const schema = `
CREATE TABLE IF NOT EXISTS rockets (
	channel             TEXT PRIMARY KEY,
	status              TEXT    NOT NULL,
	type                TEXT    NOT NULL,
	mission             TEXT    NOT NULL,
	speed               INTEGER NOT NULL,
	explosion_reason    TEXT    NOT NULL,
	last_message_number INTEGER NOT NULL,
	last_message_time   TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS pending_messages (
	channel        TEXT    NOT NULL,
	message_number INTEGER NOT NULL,
	message        TEXT    NOT NULL,
	PRIMARY KEY (channel, message_number)
);`

const rocketColumns = `channel, status, type, mission, speed, explosion_reason, last_message_number, last_message_time`

// SQLite is a rocket.Store stored in a SQLite database file, so state survives restarts.
// SQLite allows one writer at a time, so transactions share a single connection and never
// conflict; reads use a separate read-only pool and, in WAL mode, run in parallel.
type SQLite struct {
	write *sql.DB
	read  *sql.DB
}

var _ rocket.Store = (*SQLite)(nil)

// OpenSQLite opens the database at path, creating it and its tables if needed.
func OpenSQLite(path string) (*SQLite, error) {
	write, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	// BEGIN IMMEDIATE takes the write lock up front, so a write waits for busy_timeout
	// instead of failing with SQLITE_BUSY when upgrading from a read.
	write.SetMaxOpenConns(1)
	if _, err := write.Exec(schema); err != nil {
		write.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	read, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=query_only(1)")
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(runtime.NumCPU())

	return &SQLite{write: write, read: read}, nil
}

// Reset deletes all rockets and pending messages.
func (s *SQLite) Reset(ctx context.Context) error {
	_, err := s.write.ExecContext(ctx, `DELETE FROM rockets; DELETE FROM pending_messages;`)
	return err
}

// Close closes the database.
func (s *SQLite) Close() error {
	return errors.Join(s.read.Close(), s.write.Close())
}

// WithTx runs fn in one transaction, committing if fn returns nil and rolling back otherwise.
func (s *SQLite) WithTx(ctx context.Context, fn func(tx rocket.TxStore) error) error {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit; also rolls back if fn panics

	if err := fn(sqliteTx{tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

// GetRocket returns one rocket, or rocket.ErrNotFound.
func (s *SQLite) GetRocket(ctx context.Context, channel string) (rocket.Rocket, error) {
	return getRocket(ctx, s.read, channel)
}

// ListRockets returns all rockets in no particular order.
func (s *SQLite) ListRockets(ctx context.Context) ([]rocket.Rocket, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+rocketColumns+` FROM rockets`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rockets := []rocket.Rocket{}
	for rows.Next() {
		r, err := scanRocket(rows)
		if err != nil {
			return nil, err
		}
		rockets = append(rockets, r)
	}
	return rockets, rows.Err()
}

// sqliteTx is rocket.TxStore inside a WithTx transaction.
type sqliteTx struct {
	tx *sql.Tx
}

// GetRocket returns one rocket, or rocket.ErrNotFound.
func (t sqliteTx) GetRocket(ctx context.Context, channel string) (rocket.Rocket, error) {
	return getRocket(ctx, t.tx, channel)
}

// GetPendingMessages returns the messages waiting for channel's rocket, ordered by number.
func (t sqliteTx) GetPendingMessages(ctx context.Context, channel string) ([]rocket.Message, error) {
	rows, err := t.tx.QueryContext(ctx, `SELECT message FROM pending_messages WHERE channel = ? ORDER BY message_number`, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []rocket.Message
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var m rocket.Message
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			return nil, fmt.Errorf("decode pending message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// SaveRocket creates or replaces a rocket.
func (t sqliteTx) SaveRocket(ctx context.Context, r rocket.Rocket) error {
	_, err := t.tx.ExecContext(ctx, `INSERT OR REPLACE INTO rockets (`+rocketColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Channel, r.Status, r.Type, r.Mission, r.Speed, r.ExplosionReason,
		r.LastMessageNumber, r.LastMessageTime.Format(time.RFC3339Nano))
	return err
}

// SavePendingMessage stores m to wait for an earlier message; a message already pending is kept.
func (t sqliteTx) SavePendingMessage(ctx context.Context, m rocket.Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(ctx, `INSERT INTO pending_messages (channel, message_number, message) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		m.Metadata.Channel, m.Metadata.MessageNumber, string(data))
	return err
}

// DeletePendingMessages deletes channel's pending messages numbered up to throughNumber.
func (t sqliteTx) DeletePendingMessages(ctx context.Context, channel string, throughNumber int) error {
	_, err := t.tx.ExecContext(ctx, `DELETE FROM pending_messages WHERE channel = ? AND message_number <= ?`, channel, throughNumber)
	return err
}

// querier is what getRocket needs from *sql.DB and *sql.Tx.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getRocket(ctx context.Context, q querier, channel string) (rocket.Rocket, error) {
	r, err := scanRocket(q.QueryRowContext(ctx, `SELECT `+rocketColumns+` FROM rockets WHERE channel = ?`, channel))
	if errors.Is(err, sql.ErrNoRows) {
		return rocket.Rocket{}, rocket.ErrNotFound
	}
	return r, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRocket(row scanner) (rocket.Rocket, error) {
	var r rocket.Rocket
	var lastMessageTime string
	err := row.Scan(&r.Channel, &r.Status, &r.Type, &r.Mission, &r.Speed, &r.ExplosionReason,
		&r.LastMessageNumber, &lastMessageTime)
	if err != nil {
		return rocket.Rocket{}, err
	}
	r.LastMessageTime, err = time.Parse(time.RFC3339Nano, lastMessageTime)
	return r, err
}
