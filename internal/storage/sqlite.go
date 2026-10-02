// Package storage implements rocket.Store on SQLite.
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

	"lunar-backend-engineer-challenge/internal/rocket"
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

// selectRockets reads rocketColumns plus the number of pending messages for each rocket.
const selectRockets = `SELECT ` + rocketColumns + `,
	(SELECT COUNT(*) FROM pending_messages p WHERE p.channel = rockets.channel)
FROM rockets`

const selectRocket = selectRockets + ` WHERE channel = ?`

// SQLite is a rocket.Store backed by a SQLite database file. SQLite permits a single writer,
// so all transactions share one connection and are serialized; reads use a separate read-only
// connection pool and, in WAL mode, run concurrently with writes.
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

// ProcessMessage atomically applies m and any pending messages that follow it, using apply.
func (s *SQLite) ProcessMessage(ctx context.Context, m rocket.Message, apply func(r *rocket.Rocket, m rocket.Message) rocket.MessageStatus) (rocket.MessageStatus, error) {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback() // no effect after Commit; ensures rollback on error or panic

	channel := m.Metadata.Channel
	r, err := scanRocket(tx.QueryRowContext(ctx, selectRocket, channel))
	if errors.Is(err, rocket.ErrNotFound) {
		r = rocket.NewRocket(channel)
	} else if err != nil {
		return "", err
	}

	status := apply(&r, m)
	switch status {
	case rocket.MessagePending:
		if err := savePending(ctx, tx, m); err != nil {
			return "", err
		}
	case rocket.MessageApplied:
		// Apply pending messages that now follow in sequence, until a gap.
		for {
			next, ok, err := takePending(ctx, tx, channel, r.LastMessageNumber+1)
			if err != nil {
				return "", err
			}
			if !ok || apply(&r, next) != rocket.MessageApplied {
				break
			}
		}
	case rocket.MessageDuplicate:
		return status, nil // nothing to persist
	default:
		return "", fmt.Errorf("unexpected message status %q", status)
	}
	if err := saveRocket(ctx, tx, r); err != nil {
		return "", err
	}
	return status, tx.Commit()
}

// GetRocket returns one rocket, or rocket.ErrNotFound.
func (s *SQLite) GetRocket(ctx context.Context, channel string) (rocket.Rocket, error) {
	return scanRocket(s.read.QueryRowContext(ctx, selectRocket, channel))
}

// ListRockets returns all rockets in no particular order.
func (s *SQLite) ListRockets(ctx context.Context) ([]rocket.Rocket, error) {
	rows, err := s.read.QueryContext(ctx, selectRockets)
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

func saveRocket(ctx context.Context, tx *sql.Tx, r rocket.Rocket) error {
	_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO rockets (`+rocketColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Channel, r.Status, r.Type, r.Mission, r.Speed, r.ExplosionReason,
		r.LastMessageNumber, r.LastMessageTime.Format(time.RFC3339Nano))
	return err
}

// savePending stores m until the preceding messages arrive; an existing pending message with the same number is kept.
func savePending(ctx context.Context, tx *sql.Tx, m rocket.Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO pending_messages (channel, message_number, message) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		m.Metadata.Channel, m.Metadata.MessageNumber, string(data))
	return err
}

// takePending deletes and returns channel's pending message with the given number, or false if there is none.
func takePending(ctx context.Context, tx *sql.Tx, channel string, number int64) (rocket.Message, bool, error) {
	var data string
	err := tx.QueryRowContext(ctx, `DELETE FROM pending_messages WHERE channel = ? AND message_number = ? RETURNING message`,
		channel, number).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return rocket.Message{}, false, nil
	}
	if err != nil {
		return rocket.Message{}, false, err
	}
	var m rocket.Message
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return rocket.Message{}, false, fmt.Errorf("decode pending message: %w", err)
	}
	return m, true, nil
}

// scanner is the subset of *sql.Row and *sql.Rows used by scanRocket.
type scanner interface {
	Scan(dest ...any) error
}

// scanRocket reads a rockets row; sql.ErrNoRows is reported as rocket.ErrNotFound.
func scanRocket(row scanner) (rocket.Rocket, error) {
	var r rocket.Rocket
	var lastMessageTime string
	err := row.Scan(&r.Channel, &r.Status, &r.Type, &r.Mission, &r.Speed, &r.ExplosionReason,
		&r.LastMessageNumber, &lastMessageTime, &r.PendingMessages)
	if errors.Is(err, sql.ErrNoRows) {
		return rocket.Rocket{}, rocket.ErrNotFound
	}
	if err != nil {
		return rocket.Rocket{}, err
	}
	r.LastMessageTime, err = time.Parse(time.RFC3339Nano, lastMessageTime)
	return r, err
}
