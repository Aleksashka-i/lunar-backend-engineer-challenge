package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/storage"
)

func openSQLite(t *testing.T, path string) *storage.SQLite {
	t.Helper()
	db, err := storage.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func newSQLite(t *testing.T) *storage.SQLite {
	return openSQLite(t, filepath.Join(t.TempDir(), "rockets.db"))
}

func speedUp(channel string, number int) rocket.Message {
	return rocket.Message{
		Metadata: rocket.Metadata{Channel: channel, MessageNumber: number, MessageType: rocket.TypeSpeedIncreased},
		Event:    rocket.SpeedIncreased{By: number},
	}
}

// inTx runs fn in a transaction and fails the test if it returns an error.
func inTx(t *testing.T, db *storage.SQLite, fn func(tx rocket.TxStore) error) {
	t.Helper()
	require.NoError(t, db.WithTx(context.Background(), fn))
}

func pendingNumbers(t *testing.T, db *storage.SQLite, channel string) []int {
	t.Helper()
	var numbers []int
	inTx(t, db, func(tx rocket.TxStore) error {
		pending, err := tx.GetPendingMessages(context.Background(), channel)
		for _, m := range pending {
			numbers = append(numbers, m.Metadata.MessageNumber)
		}
		return err
	})
	return numbers
}

func TestGetRocketUnknown(t *testing.T) {
	db := newSQLite(t)
	_, err := db.GetRocket(context.Background(), "unknown")
	assert.ErrorIs(t, err, rocket.ErrNotFound)

	inTx(t, db, func(tx rocket.TxStore) error {
		_, err := tx.GetRocket(context.Background(), "unknown")
		assert.ErrorIs(t, err, rocket.ErrNotFound)
		return nil
	})
}

func TestSaveAndGetRocket(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)
	want := rocket.Rocket{Channel: "ch1", Status: rocket.StatusLaunched, Type: "Falcon-9", Speed: 500, LastMessageNumber: 1}
	inTx(t, db, func(tx rocket.TxStore) error { return tx.SaveRocket(ctx, want) })

	got, err := db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	rockets, err := db.ListRockets(ctx)
	require.NoError(t, err)
	assert.Equal(t, []rocket.Rocket{want}, rockets)
}

func TestWithTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)
	failure := errors.New("failure")

	err := db.WithTx(ctx, func(tx rocket.TxStore) error {
		require.NoError(t, tx.SaveRocket(ctx, rocket.NewRocket("ch1")))
		require.NoError(t, tx.SavePendingMessage(ctx, speedUp("ch1", 2)))
		return failure
	})
	assert.ErrorIs(t, err, failure)

	_, err = db.GetRocket(ctx, "ch1")
	assert.ErrorIs(t, err, rocket.ErrNotFound)
	assert.Empty(t, pendingNumbers(t, db, "ch1"))
}

func TestPendingMessages(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)
	inTx(t, db, func(tx rocket.TxStore) error {
		for _, n := range []int{4, 2, 3, 2} { // 2 twice: the repeat is ignored
			require.NoError(t, tx.SavePendingMessage(ctx, speedUp("ch1", n)))
		}
		return tx.SavePendingMessage(ctx, speedUp("other", 2))
	})
	assert.Equal(t, []int{2, 3, 4}, pendingNumbers(t, db, "ch1"), "ordered by number")

	inTx(t, db, func(tx rocket.TxStore) error { return tx.DeletePendingMessages(ctx, "ch1", 3) })
	assert.Equal(t, []int{4}, pendingNumbers(t, db, "ch1"))
	assert.Equal(t, []int{2}, pendingNumbers(t, db, "other"), "other channels are untouched")
}

func TestSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rockets.db")

	db := openSQLite(t, path)
	inTx(t, db, func(tx rocket.TxStore) error {
		require.NoError(t, tx.SaveRocket(ctx, rocket.NewRocket("ch1")))
		return tx.SavePendingMessage(ctx, speedUp("ch1", 2))
	})
	require.NoError(t, db.Close())

	db = openSQLite(t, path)
	_, err := db.GetRocket(ctx, "ch1")
	assert.NoError(t, err)
	assert.Equal(t, []int{2}, pendingNumbers(t, db, "ch1"))
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)
	inTx(t, db, func(tx rocket.TxStore) error {
		require.NoError(t, tx.SaveRocket(ctx, rocket.NewRocket("ch1")))
		return tx.SavePendingMessage(ctx, speedUp("ch1", 2))
	})

	require.NoError(t, db.Reset(ctx))
	_, err := db.GetRocket(ctx, "ch1")
	assert.ErrorIs(t, err, rocket.ErrNotFound)
	assert.Empty(t, pendingNumbers(t, db, "ch1"))
}

// TestConcurrentTransactions increments rockets from many goroutines while reading; each
// increment is a read-modify-write in one transaction, so none may be lost. Run with -race.
func TestConcurrentTransactions(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)
	channels := []string{"a", "b", "c"}

	var wg sync.WaitGroup
	for range 100 {
		for _, ch := range channels {
			wg.Go(func() {
				assert.NoError(t, db.WithTx(ctx, func(tx rocket.TxStore) error {
					r, err := tx.GetRocket(ctx, ch)
					if errors.Is(err, rocket.ErrNotFound) {
						r, err = rocket.NewRocket(ch), nil
					}
					if err != nil {
						return err
					}
					r.Speed++
					return tx.SaveRocket(ctx, r)
				}))
			})
			wg.Go(func() {
				_, err := db.ListRockets(ctx)
				assert.NoError(t, err)
			})
		}
	}
	wg.Wait()

	for _, ch := range channels {
		r, err := db.GetRocket(ctx, ch)
		require.NoError(t, err)
		assert.Equal(t, 100, r.Speed, ch)
	}
}
